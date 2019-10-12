package main

import (
	"math/rand"
	"time"

	"git.code.oa.com/tkestack/galaxy/pkg/galaxy"
	"git.code.oa.com/tkestack/galaxy/pkg/signal"
	glog "k8s.io/klog"
	"github.com/spf13/pflag"
	"k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/kubernetes/pkg/version/verflag"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	galaxy := galaxy.NewGalaxy()
	galaxy.AddFlags(pflag.CommandLine)
	flag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	verflag.PrintAndExitIfRequested()
	if err := galaxy.Start(); err != nil {
		glog.Fatalf("Error start galaxy: %v", err)
	}
	signal.BlockSignalHandler(func() {
		if err := galaxy.Stop(); err != nil {
			glog.Errorf("Error stop galaxy: %v", err)
		}
	})
}
