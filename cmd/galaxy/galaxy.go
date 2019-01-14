package main

import (
	"flag"
	"fmt"
	"github.com/golang/glog"

	"git.code.oa.com/gaiastack/galaxy/pkg/galaxy"
	"git.code.oa.com/gaiastack/galaxy/pkg/signal"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/ldflags"
)

var (
	flagVersion = flag.Bool("version", false, "print version")
)

func main() {
	defer glog.Flush()
	flag.Parse()
	if *flagVersion {
		fmt.Println(ldflags.Footprint())
		return
	}
	galaxy, err := galaxy.NewGalaxy()
	if err != nil {
		glog.Fatalf("Error create galaxy: %v", err)
	}
	if err := galaxy.Start(); err != nil {
		glog.Fatalf("Error start galaxy: %v", err)
	}
	signal.BlockSignalHandler(func() {
		if err := galaxy.Stop(); err != nil {
			glog.Errorf("Error stop galaxy: %v", err)
		}
	})
}
