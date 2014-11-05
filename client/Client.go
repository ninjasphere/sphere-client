package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"
	"github.com/hashicorp/mdns"
	"github.com/ninjasphere/go-ninja/api"
	"github.com/ninjasphere/go-ninja/config"
	"github.com/ninjasphere/go-ninja/logger"
)

var log = logger.GetLogger("Client")

type client struct {
	conn   *ninja.Connection
	master bool
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

	client := &client{conn, false}

	client.updatePairingLight("black", false)

	conn.SubscribeRaw("$sphere/bridge/status", client.onBridgeStatus)

	client.start()

	log.Infof("Starting mdns server")

	// Setup our service export
	host, _ := os.Hostname()
	service := &mdns.MDNSService{
		Instance: host,
		Service:  "_foobar._tcp",
		Port:     8000,
		TXT:      []string{"My awesome service"},
	}

	// Create the mDNS server, defer shutdown
	_, _ = mdns.NewServer(&mdns.Config{Zone: service})
	//defer server.Shutdown()
}

func (c *client) start() {

	if !c.isPaired() {
		log.Infof("Client is unpaired. Attempting to pair.")
		if err := c.pair(); err != nil {
			log.Infof("An error occurred while pairing. Restarting. error: %s", err)
			os.Exit(1)
		}

		log.Infof("Pairing was successful.")
		// We reload the config so the creds can be picked up
		config.MustRefresh()

		if !c.isPaired() {
			log.Infof("Pairing appeared successful, but I did not get the credentials. Restarting.")
			os.Exit(1)
		}
	}

	log.Infof("Client is paired. Site: %s User: %s", config.MustString("siteId"), config.MustString("userId"))

	masterID := config.String(config.Serial(), "masterNodeId")

	if masterID == config.Serial() {
		log.Infof("I am the master.")
		c.master = true
	} else {
		log.Infof("I am a slave. The master is %s", masterID)
	}

	// Make a channel for results and start listening
	entriesCh := make(chan *mdns.ServiceEntry, 4)
	go func() {
		for entry := range entriesCh {

			info := parseMdnsInfo(entry.Info)

			id, ok := info["ninja.sphere.node_id"]

			if !ok {
				log.Warningf("Found a node, but couldn't get it's node id. %v", entry)
				continue
			}

			if id == config.Serial() {
				// It's me.
				continue
			}

			if id == masterID {
				log.Infof("Found the master node (%s) - %s", id, entry.Addr)

				if err := c.bridgeToMaster(entry.Addr, entry.Port); err != nil {
					log.Errorf("Failed to bridge to master %s:%d (%s) : %s", entry.Addr, entry.Port, id, err)
				}
			} else {

				user, ok := info["ninja.sphere.user_id"]

				if !ok {
					log.Warningf("Found a node, but couldn't get it's user id. %v", entry)
					continue
				}

				if user == config.MustString("userId") {
					log.Infof("Found a sibling node (%s) - %s", id, entry.Addr)
				} else {
					log.Infof("Found a node owned by another user (%s) (%s) - %s", user, id, entry.Addr)
				}

			}
		}
	}()

	// Start the lookup
	mdns.Lookup("_ninja-homecloud-mqtt._tcp", entriesCh)
	close(entriesCh)

}

func (c *client) isPaired() bool {
	return config.HasString("siteId") && config.HasString("token") && config.HasString("userId") && config.HasString("nodeId")
}

func (c *client) bridgeToMaster(host net.IP, port int) error {

	log.Debugf("Bridging to the master: %s:%d", host, port)

	mqttURL := fmt.Sprintf("tcp://%s:%d", host, port)

	clientID := "slave-" + config.Serial()

	log.Infof("Connecting to %s using cid:%s", mqttURL, clientID)

	local := c.conn.GetMqttClient()

	opts := mqtt.NewClientOptions().AddBroker(mqttURL).SetClientId(clientID).SetCleanSession(true)
	master := mqtt.NewClient(opts)

	if _, err := master.Start(); err != nil {
		return err
	}

	bridgeTopics := []string{"$node/#", "$device/#"}

	c.bridgeMqtt(master, local, true, bridgeTopics)
	c.bridgeMqtt(local, master, false, bridgeTopics)

	return nil
}

// bridgeMqtt connects one mqtt broker to another. Shouldn't probably be doing this. But whatever.
func (c *client) bridgeMqtt(from, to *mqtt.MqttClient, masterToSlave bool, topics []string) {

	var payload map[string]interface{}
	onMessage := func(_ *mqtt.MqttClient, message mqtt.Message) {
		payload = map[string]interface{}{}
		json.Unmarshal(message.Payload(), &payload)

		source, hasSource := payload["$mesh-source"]

		interesting := false

		if masterToSlave {
			// Interesting if it's from the master or one of the other slaves
			interesting = !hasSource || (source.(string) != config.Serial())
		} else {
			// Interesting if it's from me
			interesting = !hasSource
		}

		if interesting {

			if !hasSource {
				if masterToSlave {
					payload["$mesh-source"] = "master"
				} else {
					payload["$mesh-source"] = config.Serial()
				}
			}

			jsonPayload, _ := json.Marshal(payload)
			to.Publish(mqtt.QoS(0), message.Topic(), jsonPayload)
		}

	}

	for _, topic := range topics {
		filter, _ := mqtt.NewTopicFilter(topic, 0)
		_, err := from.StartSubscription(onMessage, filter)
		if err != nil {
			log.Fatalf("Failed to subscribe to topic %s when bridging to master: %s", topic, err)
		}
	}

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

		c.conn.PublishRaw("$sphere/bridge/connect", map[string]string{
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

func (c *client) pair() error {

	localIP, err := ninja.GetNetAddress()

	if err != nil {
		log.Fatalf("Could not find local IP: %s", err)
	}

	var boardType string
	if config.HasString("boardType") {
		boardType = config.MustString("boardType")
	} else {
		boardType = fmt.Sprintf("custom-%s-%s", runtime.GOOS, runtime.GOARCH)
	}

	log.Debugf("Board type: %s", boardType)
	log.Debugf("Local IP: %s", localIP)

	url := fmt.Sprintf(config.MustString("cloud", "activation"), config.Serial(), localIP, boardType)

	log.Infof("Activating at URL: %s", url)

	client := &http.Client{}

	if config.Bool(false, "cloud", "allowSelfSigned") {
		log.Warningf("Allowing self-signed cerificate (should only be used to connect to development cloud)")
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	var creds *credentials

	for {
		creds, err = activate(client, url)

		if err != nil {
			log.Warningf("Activation error : %s", err)
			log.Warningf("Sleeping for 10sec")
			time.Sleep(time.Second * 10)
		} else if creds != nil {
			break
		}
	}

	log.Infof("Got credentials. Joining site: %s user: %s", creds.SiteID, creds.UserID)

	if creds.MasterNodeID == "" {
		log.Warningf("Cloud did not give us the master node id. So setting it to ourself (%s)", config.Serial())
		creds.MasterNodeID = config.Serial()
	}

	credsFile := config.String("/etc/opt/ninja/credentials.json", "credentialFile")

	log.Infof("Saving credentials to %s", credsFile)

	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("Failed to marshal credentials: %s", err)
	}

	return ioutil.WriteFile(credsFile, credsJSON, 0700)
}

type nodeClaimResponse struct {
	Type string              `json:"type"`
	Data responseCredentials `json:"data"`
}

type responseCredentials struct {
	UserID       string `json:"user_id"`
	SiteID       string `json:"site_id"`
	NodeID       string `json:"node_id"`
	Token        string `json:"token"`
	MasterNodeID string `json:"master_node_id"`
}

type credentials struct {
	UserID       string `json:"userId"`
	SiteID       string `json:"siteId"`
	NodeID       string `json:"nodeId"`
	Token        string `json:"token"`
	MasterNodeID string `json:"masterNodeId"`
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

	return &credentials{
		UserID:       response.Data.UserID,
		SiteID:       response.Data.SiteID,
		NodeID:       response.Data.NodeID,
		Token:        response.Data.Token,
		MasterNodeID: response.Data.MasterNodeID,
	}, err
}

func parseMdnsInfo(field string) map[string]string {
	vals := make(map[string]string)

	for _, part := range strings.Split(field, "|") {
		chunks := strings.Split(part, "=")
		vals[chunks[0]] = chunks[1]
	}
	return vals
}
