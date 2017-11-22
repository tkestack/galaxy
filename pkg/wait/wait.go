package wait

import (
	"time"

	"github.com/golang/glog"
)

// UntilQuitSignal runs `f` at `interval` rate until receiving a event from `quit` chan.
// The first run of `f` begins immediately after goroutine starts, i.e., it won't sleep for a interval time.
func UntilQuitSignal(routineName string, f func(), interval time.Duration, quit chan error) {
	glog.Infof("Starting %s..", routineName)
	for {
		f()
		select {
		case <-quit:
			// Quit if asked to do so.
			quit <- nil
			glog.Infof("Exiting %s..", routineName)
			return
		case <-time.After(interval):
		}
	}
}
