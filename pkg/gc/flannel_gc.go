package gc

import (
	"flag"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/glog"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/wait"
)

var (
	flagFlannelGCInterval = flag.Duration("flannel_gc_interval", time.Minute, "Interval of executing flannel network gc")
	flagAllocatedIPDir    = flag.String("flannel_allocated_ip_dir", "/var/lib/cni/networks", "IP storage directory of flannel cni plugin")
	flagGCDirs            = flag.String("gc_dirs", "/var/lib/cni/flannel,/var/lib/cni/galaxy,/var/lib/cni/galaxy/port", "Comma separated configure storage directory of cni plugin, the file names in this directory are container ids")
)

type flannelGC struct {
	allocatedIPDir string
	gcDirs         []string
	dockerCli      *docker.DockerInterface
	quit1, quit2   chan error
}

func NewFlannelGC(dockerCli *docker.DockerInterface, quit1, quit2 chan error) GC {
	dirs := strings.Split(*flagGCDirs, ",")
	return &flannelGC{
		allocatedIPDir: *flagAllocatedIPDir,
		gcDirs:         dirs,
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
	go wait.UntilQuitSignal("cleanup gc_dirs", func() {
		if err := gc.cleanupGCDirs(); err != nil {
			glog.Errorf("Error executing cleanup gc_dirs %v", err)
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
				}
			} else {
				glog.Warningf("Error inspect container %s: %v", containerId, err)
			}
		}
	}
	return nil
}

func (gc *flannelGC) cleanupGCDirs() error {
	for _, dir := range gc.gcDirs {
		fis, err := ioutil.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		for _, fi := range fis {
			file := filepath.Join(dir, fi.Name())
			if _, err := gc.dockerCli.InspectContainer(fi.Name()); err != nil {
				if _, ok := err.(docker.ContainerNotFoundError); ok {
					if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
						glog.Warningf("Error deleting file %s: %v", file, err)
					} else {
						if err == nil {
							glog.Infof("Deleted file %s", file)
						}
					}
				} else {
					glog.Warningf("Error inspect container %s: %v", fi.Name(), err)
				}
			}
		}
	}
	return nil
}
