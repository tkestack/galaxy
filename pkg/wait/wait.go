package wait

import (
	"time"

	"github.com/golang/glog"
)

func UntilQuitSignal(routineName string, f func(), interval time.Duration, quit chan error) {
	glog.Infof("Starting %s..", routineName)
	for {
		select {
		case <-quit:
			// Quit if asked to do so.
			quit <- nil
			glog.Infof("Exiting %s..", routineName)
			return
		case <-time.After(interval):
			f()
		}
	}
}
