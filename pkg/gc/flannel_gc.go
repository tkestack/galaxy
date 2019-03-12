package gc

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"github.com/golang/glog"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	ContainerExited = "exited"
	ContainerDead   = "dead"
)

var (
	flagFlannelGCInterval = flag.Duration("flannel_gc_interval", time.Second*10, "Interval of executing flannel network gc")
	flagAllocatedIPDir    = flag.String("flannel_allocated_ip_dir", "/var/lib/cni/networks", "IP storage directory of flannel cni plugin")
	// /var/lib/cni/galaxy/$containerid stores network type, it's like {"galaxy-flannel":{}}
	// /var/lib/cni/flannel/$containerid stores flannel cni plugin chain, it's like {"forceAddress":true,"ipMasq":false,"ipam":{"routes":[{"dst":"172.16.0.0/13"}],"subnet":"172.16.24.0/24","type":"host-local"},"isDefaultGateway":true,"mtu":1480,"name":"","routeSrc":"172.16.24.0","type":"galaxy-veth"}
	// /var/lib/cni/galaxy/port/$containerid stores port infos, it's like [{"hostPort":52701,"containerPort":19998,"protocol":"tcp","podName":"loader-server-seanyulei-1","podIP":"172.16.24.119"}]
	flagGCDirs = flag.String("gc_dirs", "/var/lib/cni/flannel,/var/lib/cni/galaxy/port", "Comma separated configure storage directory of cni plugin, the file names in this directory are container ids")
)

type flannelGC struct {
	allocatedIPDir string
	gcDirs         []string
	dockerCli      *docker.DockerInterface
	quit           <-chan struct{}
	cleanPortFunc  func(containerID string) error
}

func NewFlannelGC(dockerCli *docker.DockerInterface, quit <-chan struct{}, cleanPortFunc func(containerID string) error) GC {
	dirs := strings.Split(*flagGCDirs, ",")
	return &flannelGC{
		allocatedIPDir: *flagAllocatedIPDir,
		gcDirs:         dirs,
		dockerCli:      dockerCli,
		quit:           quit,
		cleanPortFunc:  cleanPortFunc,
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

	go wait.Until(func() {
		if err := gc.cleanupVeth(); err != nil {
			glog.Errorf("failed cleanup links: %v", err)
		}
	}, *flagFlannelGCInterval*3, gc.quit)
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
			if c.State != nil && (c.State.Status == ContainerExited || c.State.Status == ContainerDead) {
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
					gc.removeLeakyStateFile(file)
				} else {
					glog.Warningf("Error inspect container %s: %v", fi.Name(), err)
				}
			} else {
				if c.State != nil && (c.State.Status == ContainerExited || c.State.Status == ContainerDead) {
					glog.Infof("container %s(%s) exited %s", c.ID, c.Name, c.State.Status)
					gc.removeLeakyStateFile(file)
				}
			}
		}
	}
	return nil
}

func (gc *flannelGC) cleanupVeth() error {
	links, err := netlink.LinkList()
	if err != nil {
		err = fmt.Errorf("failed list links: %v", err)
		return err
	}
	for _, link := range links {
		if !strings.HasPrefix(link.Attrs().Name, "v-h") {
			continue
		}
		if link.Type() != "veth" {
			continue
		}
		parts := strings.Split(link.Attrs().Name[3:], "-")
		cid := ""
		if len(parts) == 1 || len(parts) == 2 {
			cid = parts[0]
		} else {
			continue
		}
		if c, err := gc.dockerCli.InspectContainer(cid); err != nil {
			if _, ok := err.(docker.ContainerNotFoundError); ok {
				glog.Infof("container %s not found, should remove link %s", cid, link.Attrs().Name)
				if err = netlink.LinkDel(link); err != nil {
					glog.Warningf("failed remove link %s: %v; try next time", link.Attrs().Name, err)
				}
			}
		} else {
			if c.State != nil && (c.State.Status == ContainerExited || c.State.Status == ContainerDead) {
				glog.Infof("container %s(%s) exited %s, should remove link %s", c.ID, c.Name, c.State.Status, link.Attrs().Name)
				if err = netlink.LinkDel(link); err != nil {
					glog.Warningf("failed remove link %s: %v; try next time", link.Attrs().Name, err)
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

func (gc *flannelGC) removeLeakyStateFile(file string) {
	if err := gc.cleanPortFunc(filepath.Base(file)); err != nil {
		glog.Warningf("failed to clean port of file %s: %v", file, err)
	}
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		glog.Warningf("Error deleting file %s: %v", file, err)
	} else {
		if err == nil {
			glog.Infof("Deleted file %s", file)
		}
	}
}
