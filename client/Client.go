package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/ninjasphere/go-ninja/api"
	"github.com/ninjasphere/go-ninja/config"
	"github.com/ninjasphere/go-ninja/logger"
)

var log = logger.GetLogger("Client")

type client struct {
}

func Start() {

	log.Infof("Starting client on Node: %s", config.Serial())
	client := &client{}

	if !client.isPaired() {
		log.Infof("Client is unpaired. Attempting to pair.")
		if err := client.pair(); err != nil {
			log.Infof("An error occurred while pairing. Restarting. error: %s", err)
			os.Exit(1)
		}

		log.Infof("Pairing was successful.")
		// We reload the config so the creds can be picked up
		config.MustRefresh()

		if !client.isPaired() {
			log.Infof("Pairing appeared successful, but we did not get the credentials. Restarting.")
			os.Exit(1)
		}
	}

	log.Infof("Client is paired. Site: %s User: %s", config.MustString("siteId"), config.MustString("userId"))
}

func (c *client) isPaired() bool {
	return config.HasString("siteId") && config.HasString("token") && config.HasString("userId") && config.HasString("nodeId")
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

//{"type":"node_claim","data":{"user_id":"4f983c15-c040-4de8-995d-12a656711113","site_id":"ac4da3f6-1439-4958-bfbb-6dc4f4078c5e","node_id":"elliotmac","token":"ntok_20bda9cec28b078e61e0fdad6e8197bee3d169752f230efd"}}

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
