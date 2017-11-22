package kernel

import (
	"io/ioutil"
	"testing"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/network/netns"
	"k8s.io/client-go/1.4/pkg/util/wait"
)

func TestIPForward(t *testing.T) {
	teardown := netns.NewNetnsForTest()
	defer teardown()
	// remount sysfs in the new netns
	if err := remountSysfs(); err != nil {
		t.Fatal(err)
	}
	quit := make(chan error)
	// make loop runs quickly to avoid race condition
	interval = time.Millisecond * 10
	IPForward(quit, true)
	if err := wait.Poll(time.Millisecond*50, time.Minute, func() (done bool, err error) {
		data, err := ioutil.ReadFile("/proc/sys/net/ipv4/ip_forward")
		if err != nil {
			return false, err
		}
		if string(data) == "1\n" {
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	quit<-nil
	<-quit
}
