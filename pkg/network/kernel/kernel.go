package kernel

import (
	"io/ioutil"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/wait"
	"github.com/golang/glog"
)

func BridgeNFCallIptables(quit chan error, set bool) {
	file := "/proc/sys/net/bridge/bridge-nf-call-iptables"
	go wait.UntilQuitSignal("ensure kernel args bridge-nf-call-iptables", func() {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			glog.Warningf("Error open %s: %v", file, err)
		}
		if set {
			if string(data) != "1\n" {
				glog.Warningf("%s unset, setting it", file)
				if err := ioutil.WriteFile(file, []byte("1"), 0644); err != nil {
					glog.Warningf("Error set kernel args %s: %v", file, err)
				}
			}
		} else {
			glog.Warningf("%s seted, unsetting it", file)
			if err := ioutil.WriteFile(file, []byte("0"), 0644); err != nil {
				glog.Warningf("Error set kernel args %s: %v", file, err)
			}
		}
	}, 5*time.Minute, quit)
}
