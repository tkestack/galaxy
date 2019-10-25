package main

import (
	"math/rand"
	"time"

	"tkestack.io/galaxy/pkg/galaxy"
	"tkestack.io/galaxy/pkg/signal"
	glog "k8s.io/klog"
	"github.com/spf13/pflag"
	"k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/kubernetes/pkg/version/verflag"
)

func main() {
	// initialize rand seed
	rand.Seed(time.Now().UTC().UnixNano())
	galaxy := galaxy.NewGalaxy()
	// add command line args
	galaxy.AddFlags(pflag.CommandLine)
	flag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	// if checking version, print it and exit
	verflag.PrintAndExitIfRequested()
	if err := galaxy.Start(); err != nil {
		glog.Fatalf("Error start galaxy: %v", err)
	}
	// handle signals
	signal.BlockSignalHandler(func() {
		if err := galaxy.Stop(); err != nil {
			glog.Errorf("Error stop galaxy: %v", err)
		}
	})
}
