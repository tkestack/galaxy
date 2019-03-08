package main

import (
	"math/rand"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/galaxy"
	"git.code.oa.com/gaiastack/galaxy/pkg/signal"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/flag"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/ldflags/verflag"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/logs"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
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
