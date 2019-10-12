package signal

import (
	"os"
	"os/signal"
	"syscall"

	glog "k8s.io/klog"
)

func BlockSignalHandler(f func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Block until a signal is received.
	sig := <-c
	f()
	glog.Infof("Exiting given signal: %v", sig)
	os.Exit(0)
}
