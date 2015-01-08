package client

import "os"

func reboot() {
	log.Warningf("Not rebooting. As you're on a mac and probably don't want to actually boot your dev machine. You're welcome.")
	os.Exit(0)
}
