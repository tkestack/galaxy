package gc

import (
	"flag"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/glog"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/wait"
)

var (
	flagFlannelGCInterval = flag.Duration("flannel_gc_interval", time.Minute, "Interval of executing flannel network gc")
	flagAllocatedIPDir    = flag.String("flannel_allocated_ip_dir", "/var/lib/cni/networks/flannel", "IP storage directory of flannel cni plugin")
	flagBridgeConfDir     = flag.String("flannel_bridge_conf_dir", "/var/lib/cni/flannel", "Bridge configure storage directory of flannel cni plugin")
)

type flannelGC struct {
	allocatedIPDir string
	bridgeConfDir  string
	dockerCli      *docker.DockerInterface
	quit1, quit2   chan error
}

func NewFlannelGC(dockerCli *docker.DockerInterface, quit1, quit2 chan error) GC {
	return &flannelGC{
		allocatedIPDir: *flagAllocatedIPDir,
		bridgeConfDir:  *flagBridgeConfDir,
		dockerCli:      dockerCli,
		quit1:          quit1,
		quit2:          quit2,
	}
}

func (gc *flannelGC) Run() {
	go wait.UntilQuitSignal("flannel gc cleanup ip", func() {
		if err := gc.cleanupIP(); err != nil {
			glog.Errorf("Error executing flannel gc cleanup ip %v", err)
		}
	}, *flagFlannelGCInterval, gc.quit1)
	//this is an ensurance routine
	go wait.UntilQuitSignal("flannel gc cleanup bridge conf", func() {
		if err := gc.cleanupBridgeConf(); err != nil {
			glog.Errorf("Error executing flannel gc cleanup bridge conf %v", err)
		}
	}, *flagFlannelGCInterval*3, gc.quit2)
}

func (gc *flannelGC) cleanupIP() error {
	fis, err := ioutil.ReadDir(gc.allocatedIPDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, fi := range fis {
		ip := net.ParseIP(fi.Name())
		if len(ip) == 0 {
			continue
		}
		ipFile := filepath.Join(gc.allocatedIPDir, fi.Name())
		containerIdData, err := ioutil.ReadFile(ipFile)
		if os.IsNotExist(err) || len(containerIdData) == 0 {
			continue
		}
		containerId := string(containerIdData)
		if err != nil {
			if !os.IsNotExist(err) {
				glog.Warningf("Error read file %s: %v", fi.Name(), err)
			}
			continue
		}
		if _, err := gc.dockerCli.InspectContainer(containerId); err != nil {
			if _, ok := err.(docker.ContainerNotFoundError); ok {
				if err := os.Remove(ipFile); err != nil && !os.IsNotExist(err) {
					glog.Warningf("Error deleting leaky ip file %s container: %v", ipFile, containerId, err)
				} else {
					if err == nil {
						glog.Infof("Deleted leaky ip file %s container %s", ipFile, containerId)
					}
					bridgeConfFile := filepath.Join(gc.bridgeConfDir, containerId)
					if err := os.Remove(bridgeConfFile); err != nil && !os.IsNotExist(err) {
						glog.Warningf("Error deleting bridge config file %s: %v", bridgeConfFile, err)
					} else {
						if err == nil {
							glog.Infof("Deleted bridge config file %s", bridgeConfFile)
						}
					}
				}
			} else {
				glog.Warningf("Error inspect container %s: %v", containerId, err)
			}
		}
	}
	return nil
}

func (gc *flannelGC) cleanupBridgeConf() error {
	fis, err := ioutil.ReadDir(gc.bridgeConfDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, fi := range fis {
		bridgeConfFile := filepath.Join(gc.bridgeConfDir, fi.Name())
		if _, err := gc.dockerCli.InspectContainer(fi.Name()); err != nil {
			if _, ok := err.(docker.ContainerNotFoundError); ok {
				if err := os.Remove(bridgeConfFile); err != nil && !os.IsNotExist(err) {
					glog.Warningf("Error deleting bridge config file %s: %v", bridgeConfFile, err)
				} else {
					if err == nil {
						glog.Infof("Deleted bridge config file %s", bridgeConfFile)
					}
				}
			} else {
				glog.Warningf("Error inspect container %s: %v", fi.Name(), err)
			}
		}
	}
	return nil
}
