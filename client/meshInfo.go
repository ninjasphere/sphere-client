package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"time"

	"github.com/ninjasphere/go-ninja/config"
)

var meshFile = config.String("/data/etc/opt/ninja/mesh.json", "meshFile")

type meshInfo struct {
	SiteID       string `json:"siteId"`
	MasterNodeID string `json:"masterNodeId"`
	SiteUpdated  int    `json:"siteUpdated"`
}

func refreshMeshInfo() (*meshInfo, error) {

	nodes, err := getNodes()
	if err != nil {
		return nil, err
	}

	sites, err := getSites()
	if err != nil {
		return nil, err
	}

	node, ok := nodes[config.Serial()]
	if !ok {
		return nil, errors.New("Could not find our node in the cloud. (race condition?)")
	}

	site, ok := sites[node.SiteID]
	if !ok {
		return nil, errors.New("Could not find our node in the cloud. (race condition?)")
	}

	if config.Bool(false, "forceMaster") {
		site.MasterNodeID = config.Serial()
	}

	meshInfo := &meshInfo{
		SiteID:       site.ID,
		MasterNodeID: site.MasterNodeID,
		SiteUpdated:  int(time.Time(site.Updated).UnixNano() / int64(time.Second)),
	}

	return meshInfo, saveMeshInfo(meshInfo)
}

func saveMeshInfo(mesh *meshInfo) error {

	log.Infof("Saving mesh info to %s", meshFile)

	meshJSON, err := json.Marshal(mesh)
	if err != nil {
		return fmt.Errorf("Failed to marshal mesh info: %s", err)
	}

	// XXX: HACK: TODO: CHANGE ME BACK TO 600
	err = ioutil.WriteFile(meshFile, meshJSON, 0644)

	if err != nil {
		return fmt.Errorf("Failed to write mesh info file: %s", err)
	}

	cmd := exec.Command("sync")
	out, err := cmd.Output()

	if err != nil {
		return fmt.Errorf("Failed to call sync after saving mesh info: %s - %s", err, out)
	}

	return nil
}
