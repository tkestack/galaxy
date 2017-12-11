package kernel

import (
	"fmt"
	"io/ioutil"
	"syscall"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/wait"
	"github.com/golang/glog"
)

var interval = 5 * time.Minute

func BridgeNFCallIptables(quit chan error, set bool) {
	expect := "1"
	if !set {
		expect = "0"
	}
	setArg(expect, "/proc/sys/net/bridge/bridge-nf-call-iptables", quit)
}

func IPForward(quit chan error, set bool) {
	expect := "1"
	if !set {
		expect = "0"
	}
	setArg(expect, "/proc/sys/net/ipv4/ip_forward", quit)
}

func setArg(expect string, file string, quit chan error) {
	go wait.UntilQuitSignal(fmt.Sprintf("ensure kernel args %s", file), func() {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			glog.Warningf("Error open %s: %v", file, err)
		}
		if string(data) != expect+"\n" {
			glog.Warningf("%s unset, setting it", file)
			if err := ioutil.WriteFile(file, []byte(expect), 0644); err != nil {
				glog.Warningf("Error set kernel args %s: %v", file, err)
			}
		}
	}, interval, quit)
}

func remountSysfs() error {
	if err := syscall.Mount("", "/", "none", syscall.MS_SLAVE|syscall.MS_REC, ""); err != nil {
		return err
	}
	if err := syscall.Unmount("/sys", syscall.MNT_DETACH); err != nil {
		return err
	}
	return syscall.Mount("sysfs", "/sys", "sysfs", 0, "")
}
