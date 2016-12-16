package kernel

import (
	"io/ioutil"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/wait"
	"github.com/golang/glog"
)

func BridgeNFCallIptables(quit chan error) {
	file := "/proc/sys/net/bridge/bridge-nf-call-iptables"
	go wait.UntilQuitSignal("ensure kernel args bridge-nf-call-iptables", func() {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			glog.Warningf("Error open %s: %v", file, err)
		}
		if string(data) != "1" {
			glog.Warningf("%s unset", file)
			if err := ioutil.WriteFile(file, []byte("1"), 0644); err != nil {
				glog.Warningf("Error set kernel args %s: %v", file, err)
			}
		}
	}, 5*time.Minute, quit)
}
