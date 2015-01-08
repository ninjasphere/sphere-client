package client

import (
	"os/exec"
	"time"
)

func reboot() {
	exec.Command("reboot").Output()
	time.Sleep(time.Second * 5)
	//syscall.Reboot(0)
}
