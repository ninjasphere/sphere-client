package client

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/ninjasphere/go-ninja/config"
)

type Node struct {
	ID     string `json:"node_id"`
	SiteID string `json:"site_id"`
}

type Site struct {
	ID           string `json:"site_id"`
	MasterNodeID string `json:"master_node_id"`
	UserID       string `json:"user_id"`
	Updated      nTime  `json:"updated"`
}

var errorUnauthorised = errors.New("Unauthorised token")

type nTime time.Time

func (jt *nTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}

	*jt = nTime(t)
	return nil
}

type restError struct {
	Type    string `json:"type"`
	Code    int    `json:"code"`
	Message int    `json:"message"`
}

type restResponse struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func getNodes() (map[string]Node, error) {
	var data []Node
	err := req(config.MustString("cloud", "nodes"), &data)
	log.Debugf("Fetched nodes: %+v", data)

	m := make(map[string]Node)
	for _, n := range data {
		m[n.ID] = n
	}

	return m, err
}

func getSites() (map[string]Site, error) {
	var data []Site
	err := req(config.MustString("cloud", "sites"), &data)
	log.Debugf("Fetched sites: %+v", data)

	m := make(map[string]Site)
	for _, s := range data {
		m[s.ID] = s
	}

	return m, err
}

func req(url string, data interface{}) error {

	client := &http.Client{
		Timeout: time.Second * 30,
	}

	if config.Bool(false, "cloud", "allowSelfSigned") {
		log.Warningf("Allowing self-signed cerificate (should only be used to connect to development cloud)")
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Get(fmt.Sprintf(url, config.MustString("token")))
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var response restResponse

	err = json.Unmarshal(body, &response)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK || response.Type == "error" {
		var data restError
		err = json.Unmarshal(response.Data, &data)

		if data.Type == "authentication_invalid_token" {
			return errorUnauthorised
		}

		if err != nil {
			return err
		}

		return fmt.Errorf("Error from cloud: %s (%s)", data.Message, data.Type)
	}

	return json.Unmarshal(response.Data, data)
}
