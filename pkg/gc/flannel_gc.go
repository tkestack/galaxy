/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package gc

import (
	"context"
	"flag"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vishvananda/netlink"

	"k8s.io/apimachinery/pkg/util/wait"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/docker"
)

const (
	ContainerExited  = "exited"
	ContainerDead    = "dead"
	SandboxName      = "io.kubernetes.cri.sandbox-name"
	SandboxNamespace = "io.kubernetes.cri.sandbox-namespace"
)

var (
	flagFlannelGCInterval = flag.Duration("flannel_gc_interval", time.Second*10, "Interval of executing flannel "+
		"network gc")
	flagAllocatedIPDir = flag.String("flannel_allocated_ip_dir", "/var/lib/cni/networks,/var/lib/cni/networks/galaxy-flannel",
		"IP storage directory of flannel cni plugin")
	// /var/lib/cni/galaxy/$containerid stores network type, it's like {"galaxy-flannel":{}}
	// /var/lib/cni/flannel/$containerid stores flannel cni plugin chain,
	// it's like {"forceAddress":true,"ipMasq":false,"ipam":{"routes":[{"dst":"172.16.0.0/13"}],"subnet":
	// "172.16.24.0/24","type":"host-local"},"isDefaultGateway":true,"mtu":1480,"name":"","routeSrc":"172.16.24.0",
	// "type":"galaxy-veth"}
	// /var/lib/cni/galaxy/port/$containerid stores port infos, it's like [{"hostPort":52701,"containerPort":19998,
	// "protocol":"tcp","podName":"loader-server-seanyulei-1","podIP":"172.16.24.119"}]
	flagGCDirs = flag.String("gc_dirs", "/var/lib/cni/flannel,/var/lib/cni/galaxy,/var/lib/cni/galaxy/port", "Comma "+
		"separated configure storage directory of cni plugin, the file names in this directory are container ids")
)

type flannelGC struct {
	allocatedIPDir []string
	gcDirs         []string
	dockerCli      *docker.DockerInterface
	kubeCli        kubernetes.Interface
	quit           <-chan struct{}
	cleanPortFunc  func(containerID string) error
}

func NewFlannelGC(kubeCli kubernetes.Interface, dockerCli *docker.DockerInterface, quit <-chan struct{},
	cleanPortFunc func(containerID string) error) GC {
	dirs := strings.Split(*flagGCDirs, ",")
	return &flannelGC{
		allocatedIPDir: strings.Split(*flagAllocatedIPDir, ","),
		gcDirs:         dirs,
		kubeCli:        kubeCli,
		dockerCli:      dockerCli,
		quit:           quit,
		cleanPortFunc:  cleanPortFunc,
	}
}

func (gc *flannelGC) Run() {
	go wait.Until(func() {
		glog.V(4).Infof("starting flannel gc cleanup ip")
		defer glog.V(4).Infof("flannel gc cleanup ip complete")
		if err := gc.cleanupIP(); err != nil {
			glog.Warningf("Error executing flannel gc cleanup ip %v", err)
		}
	}, *flagFlannelGCInterval, gc.quit)
	//this is an ensurance routine
	go wait.Until(func() {
		glog.V(4).Infof("starting cleanup container id file dirs")
		defer glog.V(4).Infof("cleanup container id file dirs complete")
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
	glog.V(4).Infof("cleanup ip...")
	for _, dir := range gc.allocatedIPDir {
		fis, err := ioutil.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			glog.Errorf("failed to read dir %s", dir)
			continue
		}
		for _, fi := range fis {
			if fi.IsDir() || len(net.ParseIP(fi.Name())) == 0 {
				continue
			}
			ipFile := filepath.Join(dir, fi.Name())
			containerIdData, err := ioutil.ReadFile(ipFile)
			if err != nil || len(containerIdData) == 0 {
				continue
			}
			// host-local plugin stores "containerid\neth0" or "containerid\r\neth0" in ip file, we should get the first line as container id
			parts := strings.Split(string(containerIdData), "\n")
			containerId := strings.TrimSpace(parts[0])
			if gc.shouldCleanup(containerId) {
				removeLeakyIPFile(ipFile, containerId)
			}
		}
	}
	return nil
}

func (gc *flannelGC) cleanupGCDirs() error {
	glog.V(4).Infof("cleanup gc_dirs...")
	for _, dir := range gc.gcDirs {
		glog.V(4).Infof("reading gcdir %s", dir)
		fis, err := ioutil.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			glog.Errorf("failed to read dir %s", dir)
			continue
		}
		for _, fi := range fis {
			if fi.IsDir() {
				continue
			}
			if gc.shouldCleanup(fi.Name()) {
				gc.removeLeakyStateFile(filepath.Join(dir, fi.Name()))
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
		if gc.shouldCleanup(cid) {
			if err = netlink.LinkDel(link); err != nil {
				glog.Warningf("failed remove link %s: %v; try next time", link.Attrs().Name, err)
			}
			glog.Infof("removed link %s for container %s", link.Attrs().Name, cid)
		}
	}
	return nil
}

func (gc *flannelGC) shouldCleanup(cid string) bool {
	if os.Getenv("CONTAINERD_HOST") != "" {
		if c, err := gc.dockerCli.ContainedInspectContainer(cid); err != nil {
			if stausErr, ok := status.FromError(err); ok {
				if stausErr.Code() == codes.NotFound {
					glog.Infof("container %s not found", cid)
					return true
				}
				glog.Warningf("Error inspect container %s: %v", cid, err)
			} else {
				glog.Warningf("Error inspect container %s: %v", cid, err)
			}
		} else {
			if c != nil && (c.State == criapi.PodSandboxState_SANDBOX_NOTREADY) {
				pod, err := gc.kubeCli.CoreV1().Pods(c.Annotations[SandboxNamespace]).Get(context.Background(), c.Annotations[SandboxName], metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						return true
					}
					glog.Errorf("failed to get pod %s", fmt.Sprintf("%s/%s", c.Annotations[SandboxNamespace], c.Annotations[SandboxName]))
					return false
				}
				for _, status := range pod.Status.ContainerStatuses {
					if status.State.Waiting != nil || status.State.Running != nil {
						return false
					}
				}
				glog.Infof("container %s exited %s", c.Id, c.State.String())
				return true
			}
		}
		return false
	}
	if c, err := gc.dockerCli.DockerInspectContainer(cid); err != nil {
		if _, ok := err.(docker.ContainerNotFoundError); ok {
			glog.Infof("container %s not found", cid)
			return true
		} else {
			glog.Warningf("Error inspect container %s: %v", cid, err)
		}
	} else {
		if c.State != nil && (c.State.Status == ContainerExited || c.State.Status == ContainerDead) {
			glog.Infof("container %s(%s) exited %s", c.ID, c.Name, c.State.Status)
			return true
		}
	}
	return false
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
