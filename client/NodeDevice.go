package client

import (
	"github.com/ninjasphere/go-ninja/api"
	"github.com/ninjasphere/go-ninja/config"
	"github.com/ninjasphere/go-ninja/model"
)

type NodeDevice struct {
	info        *model.Module
	modelDevice *model.Device
}

func (d *NodeDevice) GetDeviceInfo() *model.Device {
	if d.modelDevice == nil {
		name := "Spheramid " + config.Serial()
		sphereVersion := config.SphereVersion()
		d.modelDevice = &model.Device{
			NaturalID:     config.Serial(),
			NaturalIDType: "node",
			Name:          &name,
			Signatures: &map[string]string{
				"ninja:manufacturer":  "Ninja Blocks Inc.",
				"ninja:productName":   "Spheramid",
				"ninja:thingType":     "node",
				"ninja:sphereVersion": sphereVersion,
			},
		}
	}
	return d.modelDevice
}

func (d *NodeDevice) GetModuleInfo() *model.Module {
	return d.info
}

func (d *NodeDevice) GetDriver() ninja.Driver {
	return d
}

func (d *NodeDevice) SetEventHandler(handler func(event string, payload interface{}) error) {
}
