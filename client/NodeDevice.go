package client

import (
	"github.com/ninjasphere/go-ninja/api"
	"github.com/ninjasphere/go-ninja/config"
	"github.com/ninjasphere/go-ninja/model"
)

type NodeDevice struct {
	info *model.Module
}

func (d *NodeDevice) GetDeviceInfo() *model.Device {
	name := "Spheramid " + config.Serial()
	return &model.Device{
		NaturalID:     config.Serial(),
		NaturalIDType: "node",
		Name:          &name,
		Signatures: &map[string]string{
			"ninja:manufacturer": "Ninja Blocks Inc.",
			"ninja:productName":  "Spheramid",
			"ninja:thingType":    "node",
		},
	}
}

func (d *NodeDevice) GetModuleInfo() *model.Module {
	return d.info
}

func (d *NodeDevice) GetDriver() ninja.Driver {
	return d
}

func (d *NodeDevice) SetEventHandler(handler func(event string, payload interface{}) error) {
}
