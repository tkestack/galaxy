package main

import (
	"flag"

	"github.com/golang/glog"

	"git.code.oa.com/gaiastack/galaxy/pkg/galaxy"
	"git.code.oa.com/gaiastack/galaxy/pkg/signal"
)

func main() {
	defer glog.Flush()
	flag.Parse()
	galaxy, err := galaxy.NewGalaxy()
	if err != nil {
		glog.Fatalf("Error create galaxy: %v", err)
	}
	galaxy.Start()
	signal.BlockSignalHandler(func() {
		if err := galaxy.Stop(); err != nil {
			glog.Errorf("Error stop galaxy: %v", err)
		}
	})
}
