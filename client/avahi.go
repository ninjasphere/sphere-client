package client

import (
	"bytes"
	"io/ioutil"
	"runtime"
	"text/template"

	"github.com/ninjasphere/go-ninja/config"
)

var src = `<?xml version="1.0" standalone="no"?>
<!DOCTYPE service-group SYSTEM "avahi-service.dtd">
<service-group>
	<name replace-wildcards="yes">{{.Serial}}</name>
	{{if .Paired}}
	<service>
		<type>_ninja-homecloud-rest._tcp</type>
		<port>80</port>
		<txt-record>ninja.sphere.user_id={{.User}}</txt-record>
		<txt-record>ninja.sphere.node_id={{.Serial}}</txt-record>
		<txt-record>ninja.sphere.master={{.Master}}</txt-record>
	</service>
	<service>
		<type>_ninja-homecloud-mqtt._tcp</type>
		<port>1883</port>
		<txt-record>ninja.sphere.user_id={{.User}}</txt-record>
		<txt-record>ninja.sphere.node_id={{.Serial}}</txt-record>
		<txt-record>ninja.sphere.master={{.Master}}</txt-record>
	</service>
	{{else}}
	<service>
		<type>_ninja-setup-assistant-rest._tcp</type>
		<port>8888</port>
		<txt-record>ninja.sphere.node_id={{.Serial}}</txt-record>
	</service>
	{{end}}
</service-group>`

func updateSphereAvahiService(isPaired, isMaster bool) error {

	tmpl, err := template.New("avahi").Parse(src)

	if err != nil {
		return err
	}

	serviceDefinition := new(bytes.Buffer)

	err = tmpl.Execute(serviceDefinition, map[string]interface{}{
		"Serial": config.Serial(),
		"Master": isMaster,
		"Paired": isPaired,
		"User":   config.String("", "userId"),
	})

	if err != nil {
		return err
	}

	log.Debugf("Saving service definition", serviceDefinition.String())

	if runtime.GOOS != "linux" {
		log.Warningf("Avahi service definition is not being saved, as platform != linux")
		return nil
	}

	return ioutil.WriteFile("/data/etc/avahi/services/ninjasphere.service", []byte(serviceDefinition.String()), 0644)
}
