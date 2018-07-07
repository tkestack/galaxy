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
	"k8s.io/apimachinery/pkg/util/wait"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
)

var (
	flagFlannelGCInterval = flag.Duration("flannel_gc_interval", time.Second*10, "Interval of executing flannel network gc")
	flagAllocatedIPDir    = flag.String("flannel_allocated_ip_dir", "/var/lib/cni/networks", "IP storage directory of flannel cni plugin")
	flagGCDirs            = flag.String("gc_dirs", "/var/lib/cni/flannel,/var/lib/cni/galaxy,/var/lib/cni/galaxy/port", "Comma separated configure storage directory of cni plugin, the file names in this directory are container ids")
)

type flannelGC struct {
	allocatedIPDir string
	gcDirs         []string
	dockerCli      *docker.DockerInterface
	quit           <-chan struct{}
}

func NewFlannelGC(dockerCli *docker.DockerInterface, quit <-chan struct{}) GC {
	dirs := strings.Split(*flagGCDirs, ",")
	return &flannelGC{
		allocatedIPDir: *flagAllocatedIPDir,
		gcDirs:         dirs,
		dockerCli:      dockerCli,
		quit:           quit,
	}
}

func (gc *flannelGC) Run() {
	go wait.Until(func() {
		glog.Infof("starting flannel gc cleanup ip")
		defer glog.Infof("flannel gc cleanup ip complete")
		if err := gc.cleanupIP(); err != nil {
			glog.Warningf("Error executing flannel gc cleanup ip %v", err)
		}
	}, *flagFlannelGCInterval, gc.quit)
	//this is an ensurance routine
	go wait.Until(func() {
		glog.Infof("starting cleanup container id file dirs")
		defer glog.Infof("cleanup container id file dirs complete")
		if err := gc.cleanupGCDirs(); err != nil {
			glog.Errorf("Error executing cleanup gc_dirs %v", err)
		}
	}, *flagFlannelGCInterval, gc.quit)
}

func (gc *flannelGC) cleanupIP() error {
	glog.Infof("cleanup ip...")
	fis, err := ioutil.ReadDir(gc.allocatedIPDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
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
		if c, err := gc.dockerCli.InspectContainer(containerId); err != nil {
			if _, ok := err.(docker.ContainerNotFoundError); ok {
				glog.Infof("container %s not found", containerId)
				removeLeakyIPFile(ipFile, containerId)
			} else {
				glog.Warningf("Error inspect container %s: %v", containerId, err)
			}
		} else {
			if c.State != nil && (c.State.Status == "exited" || c.State.Status == "dead") {
				glog.Infof("container %s(%s) exited %s", c.ID, c.Name, c.State.Status)
				removeLeakyIPFile(ipFile, containerId)
			}
		}
	}
	return nil
}

func (gc *flannelGC) cleanupGCDirs() error {
	glog.Infof("cleanup gc_dirs...")
	for _, dir := range gc.gcDirs {
		glog.Infof("start cleanup %s", dir)
		fis, err := ioutil.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		for _, fi := range fis {
			if fi.IsDir() {
				continue
			}
			file := filepath.Join(dir, fi.Name())
			if c, err := gc.dockerCli.InspectContainer(fi.Name()); err != nil {
				if _, ok := err.(docker.ContainerNotFoundError); ok {
					glog.Infof("container %s not found", fi.Name())
					removeLeakyStateFile(file)
				} else {
					glog.Warningf("Error inspect container %s: %v", fi.Name(), err)
				}
			} else {
				if c.State != nil && (c.State.Status == "exited" || c.State.Status == "dead") {
					glog.Infof("container %s(%s) exited %s", c.ID, c.Name, c.State.Status)
					removeLeakyStateFile(file)
				}
			}
		}
	}
	return nil
}

func removeLeakyIPFile(ipFile, containerId string) {
	if err := os.Remove(ipFile); err != nil && !os.IsNotExist(err) {
		glog.Warningf("Error deleting leaky ip file %s container %s: %v", ipFile, containerId, err)
	} else {
		if err == nil {
			glog.Infof("Deleted leaky ip file %s container %s", ipFile, containerId)
		}
	}
}

func removeLeakyStateFile(file string) {
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		glog.Warningf("Error deleting file %s: %v", file, err)
	} else {
		if err == nil {
			glog.Infof("Deleted file %s", file)
		}
	}
}
