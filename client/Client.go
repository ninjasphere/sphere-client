package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/ninjasphere/go-ninja/api"
	"github.com/ninjasphere/go-ninja/bus"
	"github.com/ninjasphere/go-ninja/config"
	"github.com/ninjasphere/go-ninja/logger"
	"github.com/ninjasphere/go-ninja/model"
)

var log = logger.GetLogger("Client")

type client struct {
	conn    *ninja.Connection
	master  bool
	bridged bool
}

type bridgeStatus struct {
	Connected  bool `json:"connected"`
	Configured bool `json:"configured"`
}

func Start() {

	log.Infof("Starting client on Node: %s", config.Serial())

	conn, err := ninja.Connect("client")
	if err != nil {
		log.Fatalf("Failed to connect to sphere: %s", err)
	}

	client := &client{
		conn: conn,
	}

	if !config.IsPaired() {
		err = UpdateSphereAvahiService(false, false)
		if err != nil {
			log.Fatalf("Failed to update avahi service: %s", err)
		}
	}

	client.updatePairingLight("black", false)

	conn.SubscribeRaw("$sphere/bridge/status", client.onBridgeStatus)

	client.start()

	err = UpdateSphereAvahiService(true, client.master)
	if err != nil {
		log.Fatalf("Failed to update avahi service: %s", err)
	}

	go func() {
		err := client.ensureTimezoneIsSet()
		if err != nil {
			log.Warningf("Could not save timezone: %s", err)
		}
	}()

}

func (c *client) start() {

	if !config.IsPaired() {
		log.Infof("Client is unpaired. Attempting to pair.")
		if err := c.pair(); err != nil {
			log.Infof("An error occurred while pairing. Restarting. error: %s", err)
			os.Exit(1)
		}

		log.Infof("Pairing was successful.")
		// We reload the config so the creds can be picked up
		config.MustRefresh()

		if !config.IsPaired() {
			log.Infof("Pairing appeared successful, but I did not get the credentials. Restarting.")
			os.Exit(1)
		}

		log.Infof("Restarting sphere-client.") // XXX: Shouldn't need to do this. Just testing.
		os.Exit(0)

	}

	log.Infof("Client is paired. User: %s", config.MustString("userId"))

	mesh, err := refreshMeshInfo()

	if err == errorUnauthorised {
		log.Warningf("UNAUTHORISED! Unpairing.")
		c.unpair()
		return
	}

	if err != nil {
		log.Warningf("Failed to refresh mesh info: %s", err)
	} else {
		log.Debugf("Got mesh info: %+v", mesh)
	}

	config.MustRefresh()

	if !config.HasString("masterNodeId") {
		log.Warningf("We don't have any mesh information. Which is unlikely. But we can't do anything without it, so restarting client.")
		time.Sleep(time.Second * 10)
		os.Exit(0)
	}

	if config.MustString("masterNodeId") == config.Serial() {
		log.Infof("I am the master, starting HomeCloud.")

		go func() {
			for {
				c.conn.SendNotification("$node/"+config.Serial()+"/module/start", "sphere-go-homecloud")

				time.Sleep(time.Second * 5)
			}
		}()

		c.master = true

		c.exportNodeDevice()
	} else {
		log.Infof("I am a slave. The master is %s", config.MustString("masterNodeId"))

		cmd := exec.Command("stop", "sphere-director")
		cmd.Output()
	}

	go func() {
		for {
			c.findPeers()
			time.Sleep(time.Second * 30)
		}
	}()
}

func (c *client) exportNodeDevice() {

	device := &NodeDevice{ninja.LoadModuleInfo("./package.json")}

	// TODO: Make some generic way to see if homecloud is running.
	// XXX: Fix this. It's ugly.
	for {
		siteModel := c.conn.GetServiceClient("$home/services/SiteModel")
		err := siteModel.Call("fetch", config.MustString("siteId"), nil, time.Second*5)

		if err == nil {
			break
		}
		time.Sleep(time.Second * 5)
	}

	for {
		err := c.conn.ExportDevice(device)
		if err == nil {
			break
		}

		log.Warningf("Failed to export node device. Retrying in 5 sec: %s", err)
		time.Sleep(time.Second * 5)
	}
}

func (c *client) findPeers() {

	// Make a channel for results and start listening
	entriesCh := make(chan *mdns.ServiceEntry, 4)
	go func() {
		for entry := range entriesCh {
			nodeInfo := parseMdnsInfo(entry.Info)

			id, ok := nodeInfo["ninja.sphere.node_id"]

			if !ok {
				log.Warningf("Found a node, but couldn't get it's node id. %v", entry)
				continue
			}

			if id == config.Serial() {
				// It's me.
				continue
			}

			user, ok := nodeInfo["ninja.sphere.user_id"]
			if !ok {
				log.Warningf("Found a node, but couldn't get it's user id. %v", entry)
				continue
			}

			site, ok := nodeInfo["ninja.sphere.site_id"]
			siteUpdated, ok := nodeInfo["ninja.sphere.site_updated"]
			masterNodeID, ok := nodeInfo["ninja.sphere.master_node_id"]

			if user == config.MustString("userId") {

				if site == config.MustString("siteId") {
					log.Infof("Found a sibling node (%s) - %s", id, entry.Addr)

					siteUpdatedInt, err := strconv.ParseInt(siteUpdated, 10, 64)

					if err != nil {
						log.Warningf("Failed to read the site_updated field (%s) on node %s - %s", siteUpdated, id, entry.Addr)
					} else {
						if int(siteUpdatedInt) > config.MustInt("siteUpdated") {

							log.Infof("Found node (%s - %s) with a newer site update time (%s).", id, entry.Addr, siteUpdated)

							info := &meshInfo{
								MasterNodeID: masterNodeID,
								SiteID:       config.MustString("siteId"),
								SiteUpdated:  int(siteUpdatedInt),
							}

							err := saveMeshInfo(info)
							if err != nil {
								log.Warningf("Failed to save updated mesh info from node: %s - %+v", err, info)
							}

							if masterNodeID == config.MustString("masterNodeId") {
								log.Infof("Updated master id is the same (%s). Moving on with our lives.", masterNodeID)
							} else {
								log.Infof("Master id has changed (was %s now %s). Rebooting", config.MustString("masterNodeId"), masterNodeID)

								reboot()
								return
							}
						}
					}

				} else {
					log.Warningf("Found a node owned by the same user (%s) but from a different site (%s) - ID:%s - %s", user, site, id, entry.Addr)
				}

			} else {
				log.Infof("Found a node owned by another user (%s) (%s) - %s", user, id, entry.Addr)
			}

			if id == config.MustString("masterNodeId") {
				log.Infof("Found the master node (%s) - %s", id, entry.Addr)

				if !c.bridged {
					c.bridgeToMaster(entry.Addr, entry.Port)
					c.bridged = true
					c.exportNodeDevice()
				}
			}

		}
	}()

	// Start the lookup
	mdns.Lookup("_ninja-homecloud-mqtt._tcp", entriesCh)
	close(entriesCh)
}

func (c *client) bridgeToMaster(host net.IP, port int) {

	log.Debugf("Bridging to the master: %s:%d", host, port)

	mqttURL := fmt.Sprintf("%s:%d", host, port)

	clientID := "slave-" + config.Serial()

	log.Infof("Connecting to master %s using cid:%s", mqttURL, clientID)

	master := bus.MustConnect(mqttURL, clientID)
	local := bus.MustConnect(fmt.Sprintf("%s:%d", config.MustString("mqtt.host"), config.MustInt("mqtt.port")), "meshing")

	bridgeTopics := []string{"$node/#", "$device/#", "$home/#"}

	c.bridgeMqtt(master, local, true, bridgeTopics)
	c.bridgeMqtt(local, master, false, bridgeTopics)
}

type meshMessage struct {
	Source *string `json:"$mesh-source"`
}

// bridgeMqtt connects one mqtt broker to another. Shouldn't probably be doing this. But whatever.
func (c *client) bridgeMqtt(from, to bus.Bus, masterToSlave bool, topics []string) {

	onMessage := func(topic string, payload []byte) {

		if payload[0] != '{' {
			log.Warningf("Invalid payload (should be a json-rpc object): %s", payload)
			return
		}

		var msg meshMessage
		json.Unmarshal(payload, &msg)

		interesting := false

		if masterToSlave {
			// Interesting if it's from the master or one of the other slaves
			interesting = msg.Source == nil || (*msg.Source != config.Serial())
		} else {
			// Interesting if it's from me
			interesting = msg.Source == nil
		}

		log.Infof("Mesh master2slave:%t topic:%s interesting:%t", masterToSlave, topic, interesting)

		if interesting {

			if msg.Source == nil {
				if masterToSlave {
					payload = addMeshSource(config.MustString("masterNodeId"), payload)
				} else {
					payload = addMeshSource(config.Serial(), payload)
				}
			}

			to.Publish(topic, payload)
		}

	}

	for _, topic := range topics {
		_, err := from.Subscribe(topic, onMessage)
		if err != nil {
			log.Fatalf("Failed to subscribe to topic %s when bridging to master: %s", topic, err)
		}
	}

}

func addMeshSource(source string, payload []byte) []byte {
	return bytes.Replace(payload, []byte("{"), []byte(`{"$mesh-source":"`+source+`", `), 1)
}

func (c *client) onBridgeStatus(status *bridgeStatus) bool {
	log.Debugf("Got bridge status. connected:%t configured:%t", status.Connected, status.Configured)

	if status.Connected {
		c.updatePairingLight("green", false)
	} else {
		c.updatePairingLight("red", true)
	}

	if !status.Configured && c.master {
		log.Infof("Configuring bridge")

		c.conn.PublishRawSingleValue("$sphere/bridge/connect", map[string]string{
			"url":   config.MustString("cloud", "url"),
			"token": config.MustString("token"),
		})
	}

	return true
}

func (c *client) updatePairingLight(color string, flash bool) {
	c.conn.PublishRaw("$hardware/status/pairing", map[string]interface{}{
		"color": color,
		"flash": flash,
	})
}

func getLocalIP() string {

	var localIP string
	var err error

	for {
		localIP, err = ninja.GetNetAddress()
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	return localIP
}

func (c *client) pair() error {

	var boardType string
	if config.HasString("boardType") {
		boardType = config.MustString("boardType")
	} else {
		boardType = fmt.Sprintf("custom-%s-%s", runtime.GOOS, runtime.GOARCH)
	}

	log.Debugf("Board type: %s", boardType)

	client := &http.Client{
		Timeout: time.Second * 60, // It's 20sec on the server so this *should* be ok
	}

	if config.Bool(false, "cloud", "allowSelfSigned") {
		log.Warningf("Allowing self-signed cerificate (should only be used to connect to development cloud)")
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	var creds *credentials

	for {
		url := fmt.Sprintf(config.MustString("cloud", "activation"), config.Serial(), getLocalIP(), boardType)

		log.Debugf("Activating at URL: %s", url)

		var err error
		creds, err = activate(client, url)

		if err != nil {
			log.Warningf("Activation error : %s", err)
			log.Warningf("Sleeping for 3sec")
			time.Sleep(time.Second * 3)
		} else if creds != nil {
			break
		}
	}

	log.Infof("Got credentials. User: %s", creds.UserID)

	return saveCreds(creds)
}

var credsFile = config.String("/data/etc/opt/ninja/credentials.json", "credentialFile")

func saveCreds(creds *credentials) error {

	log.Infof("Saving credentials to %s", credsFile)

	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("Failed to marshal credentials: %s", err)
	}

	// XXX: HACK: TODO: CHANGE ME BACK TO 600
	err = ioutil.WriteFile(credsFile, credsJSON, 0644)

	if err != nil {
		return fmt.Errorf("Failed to write credentials file: %s", err)
	}

	cmd := exec.Command("sync")
	out, err := cmd.Output()

	if err != nil {
		return fmt.Errorf("Failed to call sync after saving credentials: %s - %s", err, out)
	}

	return nil
}

func (c *client) unpair() {
	log.Infof("Unpairing")
	c.conn.SendNotification(fmt.Sprintf("$node/%s/unpair", config.Serial()), nil)
}

func (c *client) ensureTimezoneIsSet() error {

	siteModel := c.conn.GetServiceClient("$home/services/SiteModel")
	var site *model.Site

	for {
		err := siteModel.Call("fetch", config.MustString("siteId"), site, time.Second*5)
		if err == nil && site.TimeZoneID != nil {

			cmd := exec.Command("with-rw", "ln", "-s", "-f", "/usr/share/zoneinfo/"+*site.TimeZoneID, "/etc/localtime")
			_, err := cmd.Output()

			if err != nil {
				return err
			}

			break
		}
		time.Sleep(time.Second * 2)
	}

	return nil

}

type nodeClaimResponse struct {
	Type string `json:"type"`
	Data struct {
		UserID           string `json:"user_id"`
		NodeID           string `json:"node_id"`
		Token            string `json:"token"`
		SphereNetworkKey string `json:"sphere_network_key"`
	} `json:"data"`
}

type credentials struct {
	UserID           string `json:"userId"`
	Token            string `json:"token"`
	SphereNetworkKey string `json:"sphereNetworkKey"`
}

func activate(client *http.Client, url string) (*credentials, error) {

	log.Debugf("Requesting url: %s", url)

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusRequestTimeout {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Failed to activate: %s - %s", resp.Status, body)
	}

	log.Debugf("Got response: %s", body)

	var response nodeClaimResponse
	err = json.Unmarshal(body, &response)

	if response.Data.NodeID != config.Serial() {
		log.Fatalf("Incorrect node id returned from pairing! Expected %s got %s", config.Serial(), response.Data.NodeID)
	}

	return &credentials{
		UserID:           response.Data.UserID,
		Token:            response.Data.Token,
		SphereNetworkKey: response.Data.SphereNetworkKey,
	}, err
}

func parseMdnsInfo(field string) map[string]string {
	vals := make(map[string]string)

	for _, part := range strings.Split(field, "|") {
		chunks := strings.Split(part, "=")
		if len(chunks) == 2 {
			vals[chunks[0]] = chunks[1]
		}
	}
	return vals
}
