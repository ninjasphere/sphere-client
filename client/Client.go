package client

import (
	"github.com/ninjasphere/go-ninja/config"
	"github.com/ninjasphere/go-ninja/logger"
)

var log = logger.GetLogger("Client")

type Client struct {
}

func NewClient() *Client {
	client := &Client{}

	if client.isPaired() {
		log.Infof("Client is paired. Starting HomeCloud")
	} else {
		log.Infof("Client is unpaired. Attempting to pair.")
	}

	return client
}

func (c *Client) isPaired() bool {
	return config.HasString("siteId") && config.HasString("token") && config.HasString("userId") && config.HasString("nodeId")
}
