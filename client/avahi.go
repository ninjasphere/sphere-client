package client

import (
	"fmt"
	"io/ioutil"
	"runtime"

	"github.com/ninjasphere/go-ninja/config"
)

func updateSphereAvahiService(isMaster bool) error {

	serviceDefinition := `<?xml version="1.0" standalone="no"?>
<!DOCTYPE service-group SYSTEM "avahi-service.dtd">
<service-group>
  <name replace-wildcards="yes">%s</name>
  <service>
    <type>_ninja-homecloud-rest._tcp</type>
    <port>80</port>
    <txt-record>ninja.sphere.user_id=%s</txt-record>
    <txt-record>ninja.sphere.node_id=%s</txt-record>
    <txt-record>ninja.sphere.master=%t</txt-record>
  </service>
  <service>
    <type>_ninja-homecloud-mqtt._tcp</type>
    <port>1883</port>
    <txt-record>ninja.sphere.user_id=%s</txt-record>
    <txt-record>ninja.sphere.node_id=%s</txt-record>
    <txt-record>ninja.sphere.master=%t</txt-record>
  </service>
</service-group>`

	serviceDefinition = fmt.Sprintf(serviceDefinition,
		config.Serial(),
		config.MustString("userId"), config.Serial(), isMaster,
		config.MustString("userId"), config.Serial(), isMaster)

	log.Debugf("Saving service definition", serviceDefinition)

	if runtime.GOOS != "linux" {
		log.Warningf("Avahi service definition is not being saved, as platform != linux")
		return nil
	}

	return ioutil.WriteFile("/etc/avahi/services/ninjasphere.service", []byte(serviceDefinition), 0644)
}
