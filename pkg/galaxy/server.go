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
package galaxy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/emicklei/go-restful"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/cniutil"
	galaxyapi "tkestack.io/galaxy/pkg/api/galaxy"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/galaxy/constant/utils"
	"tkestack.io/galaxy/pkg/api/galaxy/private"
	"tkestack.io/galaxy/pkg/api/k8s"
	k8sutil "tkestack.io/galaxy/pkg/api/k8s/utils"
)

// StartServer will start galaxy server.
func (g *Galaxy) StartServer() error {
	if g.PProf {
		go func() {
			http.ListenAndServe("127.0.0.1:0", nil)
		}()
	}
	g.installHandlers()
	if err := os.MkdirAll(private.GalaxySocketDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %v", private.GalaxySocketDir, err)
	}
	if err := os.Remove(private.GalaxySocketPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %v", private.GalaxySocketPath, err)
		}
	}
	l, err := net.Listen("unix", private.GalaxySocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on pod info socket: %v", err)
	}
	if err := os.Chmod(private.GalaxySocketPath, 0600); err != nil {
		_ = l.Close()
		return fmt.Errorf("failed to set pod info socket mode: %v", err)
	}

	glog.Fatal(http.Serve(l, nil))
	return nil
}

func (g *Galaxy) installHandlers() {
	ws := new(restful.WebService)
	ws.Route(ws.GET("/cni").To(g.cni))
	ws.Route(ws.POST("/cni").To(g.cni))
	restful.Add(ws)
}

func (g *Galaxy) cni(r *restful.Request, w *restful.Response) {
	data, err := ioutil.ReadAll(r.Request.Body)
	if err != nil {
		glog.Warningf("bad request %v", err)
		http.Error(w, fmt.Sprintf("err read body %v", err), http.StatusBadRequest)
		return
	}
	defer r.Request.Body.Close() // nolint: errcheck
	req, err := galaxyapi.CniRequestToPodRequest(data)
	if err != nil {
		glog.Warningf("bad request %v", err)
		http.Error(w, fmt.Sprintf("%v", err), http.StatusBadRequest)
		return
	}
	req.Path = strings.TrimRight(fmt.Sprintf("%s:%s", req.Path, strings.Join(g.CNIPaths, ":")), ":")
	result, err := g.requestFunc(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
	} else {
		// Empty response JSON means success with no body
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(result); err != nil {
			glog.Warningf("Error writing %s HTTP response: %v", req.Command, err)
		}
	}
}

// #lizard forgives
func (g *Galaxy) requestFunc(req *galaxyapi.PodRequest) (data []byte, err error) {
	start := time.Now()
	glog.Infof("%v, %s+", req, start.Format(time.StampMicro))
	if req.Command == cniutil.COMMAND_ADD {
		defer func() {
			glog.Infof("%v, data %s, err %v, %s-", req, string(data), err, start.Format(time.StampMicro))
		}()
		var pod *corev1.Pod
		pod, err = g.getPod(req.PodName, req.PodNamespace)
		if err != nil {
			return
		}
		result, err1 := g.cmdAdd(req, pod)
		if err1 != nil {
			err = err1
			return
		} else {
			result020, err2 := convertResult(result)
			if err2 != nil {
				err = err2
			} else {
				data, err = json.Marshal(result)
				if err != nil {
					return
				}
				err = g.setupPortMapping(req, req.ContainerID, result020, pod)
				if err != nil {
					g.cleanupPortMapping(req)
					return
				}
				pod.Status.PodIP = result020.IP4.IP.IP.String()
				if g.pm != nil {
					if err := g.pm.SyncPodChains(pod); err != nil {
						glog.Warning(err)
					}
					g.pm.SyncPodIPInIPSet(pod, true)
				}
			}
		}
	} else if req.Command == cniutil.COMMAND_DEL {
		defer glog.Infof("%v err %v, %s-", req, err, start.Format(time.StampMicro))
		err = cniutil.CmdDel(req.CmdArgs, -1)
		if err == nil {
			err = g.cleanupPortMapping(req)
		}
	} else {
		err = fmt.Errorf("unknown command %s", req.Command)
	}
	return
}

func parsePorts(pod *corev1.Pod) []k8s.Port {
	_, portMappingOn := pod.Annotations[k8s.PortMappingPortsAnnotation]
	var ports []k8s.Port
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if (port.HostPort == 0 && portMappingOn) || port.HostPort > 0 {
				tmp := k8s.Port{
					HostPort:      port.HostPort,
					ContainerPort: port.ContainerPort,
					Protocol:      string(port.Protocol),
					PodName:       pod.Name,
					HostIP:        port.HostIP,
					PodIP:         pod.Status.PodIP,
				}
				ports = append(ports, tmp)
			}
		}
	}
	return ports
}

// #lizard forgives
func (g *Galaxy) resolveNetworks(req *galaxyapi.PodRequest, pod *corev1.Pod) ([]*cniutil.NetworkInfo, error) {
	var networkInfos []*cniutil.NetworkInfo
	if pod.Annotations == nil || pod.Annotations[constant.MultusCNIAnnotation] == "" {
		if utils.WantENIIP(&pod.Spec) && g.ENIIPNetwork != "" {
			networkInfos = append(networkInfos, cniutil.NewNetworkInfo(g.ENIIPNetwork, g.getNetworkConf(g.ENIIPNetwork),
				req.IfName))
		} else {
			for i, netName := range g.DefaultNetworks {
				networkInfos = append(networkInfos, cniutil.NewNetworkInfo(netName, g.getNetworkConf(netName),
					setNetInterface("", i, req.IfName)))
			}
		}
	} else {
		v := pod.Annotations[constant.MultusCNIAnnotation]
		glog.V(4).Infof("pod %s_%s network annotation is %s", pod.Name, pod.Namespace, v)
		networks, err := k8s.ParsePodNetworkAnnotation(v)
		if err != nil {
			return nil, err
		}
		//init networkInfo
		for idx, network := range networks {
			netConf := g.getNetworkConf(network.Name)
			if netConf == nil {
				return nil, fmt.Errorf("pod %s_%s requires network %s which is not configured", pod.Name,
					pod.Namespace, network.Name)
			}
			networkInfo := cniutil.NewNetworkInfo(network.Name, netConf,
				setNetInterface(network.InterfaceRequest, idx, req.CmdArgs.IfName))
			networkInfos = append(networkInfos, networkInfo)
		}
	}
	extendedCNIArgs, err := parseExtendedCNIArgs(pod)
	if err != nil {
		return nil, err
	}
	if commonArgs, exist := extendedCNIArgs[constant.CommonCNIArgsKey]; exist {
		for i := range networkInfos {
			for k, v := range commonArgs {
				networkInfos[i].Args[k] = string([]byte(v))
			}
		}
	}
	glog.V(4).Infof("pod %s_%s networkInfo %v", pod.Name, pod.Namespace, networkInfos)
	return networkInfos, nil
}

func (g *Galaxy) getNetworkConf(networkName string) map[string]interface{} {
	if netConf, ok := g.netConf[networkName]; ok {
		return netConf
	}
	// In the absence of existing network config from json
	// config, load and execute a CNI .configlist
	// or .config (in that order) file on-disk whose JSON
	// “name” key matches this Network object’s name.
	data, err := cniutil.GetNetworkConfig(networkName, g.NetworkConfDir)
	if err != nil {
		glog.Warningf("failed to load network config %s from confdir %s with error %v", networkName, g.NetworkConfDir, err)
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		glog.Warningf("failed to unmarshal networkinfo %s: %v", string(data), err)
		return nil
	}
	// kubeconfig of host filesystem won't be reachable for galaxy.
	// Since galaxy is running in pod, it can talk to apiserver via secret token
	if m["kubeconfig"] != "" {
		delete(m, "kubeconfig")
	}
	return m
}

func (g *Galaxy) cmdAdd(req *galaxyapi.PodRequest, pod *corev1.Pod) (types.Result, error) {
	if err := disableIPv6(req.Netns); err != nil {
		glog.Warningf("Error disable ipv6 %v", err)
	}
	networkInfos, err := g.resolveNetworks(req, pod)
	if err != nil {
		return nil, err
	}
	return cniutil.CmdAdd(req.CmdArgs, networkInfos)
}

// parseExtendedCNIArgs parses extended cni args from pod's annotation
func parseExtendedCNIArgs(pod *corev1.Pod) (map[string]map[string]json.RawMessage, error) {
	if pod.Annotations == nil {
		return nil, nil
	}
	annotation := pod.Annotations[constant.ExtendedCNIArgsAnnotation]
	if annotation == "" {
		return nil, nil
	}
	argsMap, err := constant.ParseExtendedCNIArgs(annotation)
	if err != nil {
		return nil, err
	}
	return argsMap, nil
}

func (g *Galaxy) setupIPtables() error {
	// filter all running pods on node
	pods, err := g.client.CoreV1().Pods(v1.NamespaceAll).List(v1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", k8s.GetHostname()).String()})
	if err != nil {
		return fmt.Errorf("failed to get pods on node: %v", err)
	}
	var allPorts []k8s.Port
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.HostNetwork {
			continue
		}
		var ports []k8s.Port
		if pod.Annotations != nil && pod.Annotations[k8s.PortMappingPortsAnnotation] != "" {
			if err := json.Unmarshal([]byte(pod.Annotations[k8s.PortMappingPortsAnnotation]), &ports); err != nil {
				glog.Warningf("failed to unmarshal %s_%s annotation %s: %v", pod.Name, pod.Namespace,
					k8s.PortMappingPortsAnnotation, err)
				continue
			}
		} else {
			ports = parsePorts(pod)
		}
		// open ports on start
		if err := g.pmhandler.OpenHostports(k8s.GetPodFullName(pod.Name, pod.Namespace), false, ports); err != nil {
			// port maybe taken by other process during restart, but we can do nothing about that
			// we should still setting up iptables for it.
			glog.Warning(err)
		}
		allPorts = append(allPorts, ports...)
	}
	// sync all iptables on start
	if err := g.pmhandler.SetupPortMappingForAllPods(allPorts); err != nil {
		return fmt.Errorf("failed to setup portmappings for all pods, ports %+v: %v", allPorts, err)
	}
	go wait.Until(func() {
		glog.V(4).Infof("starting to ensure iptables rules")
		defer glog.V(4).Infof("ensure iptables rules complete")
		if err := g.pmhandler.EnsureBasicRule(); err != nil {
			glog.Warningf("failed to ensure iptables rules")
		}
	}, 1*time.Minute, make(chan struct{}))
	return nil
}

func (g *Galaxy) setupPortMapping(req *galaxyapi.PodRequest, containerID string, result *t020.Result,
	pod *corev1.Pod) error {
	_, portMappingOn := pod.Annotations[k8s.PortMappingPortsAnnotation]
	req.Ports = parsePorts(pod)
	if len(req.Ports) == 0 {
		return nil
	}
	for i := range req.Ports {
		req.Ports[i].PodIP = result.IP4.IP.IP.To4().String()
		req.Ports[i].PodName = req.PodName
	}
	if err := g.pmhandler.OpenHostports(k8s.GetPodFullName(req.PodName, req.PodNamespace), portMappingOn,
		req.Ports); err != nil {
		return err
	}
	data, err := json.Marshal(req.Ports)
	if err != nil {
		return fmt.Errorf("failed to marshal ports: %v", err)
	}
	if err := k8s.SavePort(containerID, data); err != nil {
		return fmt.Errorf("failed to save ports %v", err)
	}
	if err := g.pmhandler.SetupPortMapping(req.Ports); err != nil {
		return fmt.Errorf("failed to setup port mapping %v: %v", req.Ports, err)
	}
	if portMappingOn {
		if err := g.updatePortMappingAnnotation(req, data); err != nil {
			return fmt.Errorf("failed to update pod %s annotation: %v", k8s.GetPodFullName(req.PodName,
				req.PodNamespace), err)
		}
	}
	return nil
}

func (g *Galaxy) updatePortMappingAnnotation(req *galaxyapi.PodRequest, data []byte) error {
	return wait.Poll(10*time.Millisecond, 1*time.Minute, func() (bool, error) {
		pod, err := g.client.CoreV1().Pods(req.PodNamespace).Get(req.PodName, v1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[k8s.PortMappingPortsAnnotation] = string(data)
		_, err = g.client.CoreV1().Pods(req.PodNamespace).Update(pod)
		if err == nil {
			return true, nil
		}
		glog.Warningf("failed to update pod %s annotation: %v", k8s.GetPodFullName(pod.Name, pod.Namespace), err)
		if k8sutil.ShouldRetry(err) {
			return false, nil
		}
		return false, err
	})
}

func (g *Galaxy) cleanupPortMapping(req *galaxyapi.PodRequest) error {
	g.pmhandler.CloseHostports(k8s.GetPodFullName(req.PodName, req.PodNamespace))
	return g.cleanIPtables(req.ContainerID)
}

func (g *Galaxy) cleanIPtables(containerID string) error {
	ports, err := k8s.ConsumePort(containerID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read ports %v", err)
	}
	if len(ports) != 0 {
		if err := g.pmhandler.CleanPortMapping(ports); err != nil {
			return err
		}
		if err := k8s.RemovePortFile(containerID); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete port file for %s: %v", containerID, err)
		}
	}
	return nil
}

func disableIPv6(path string) error {
	cmd := &exec.Cmd{
		Path:   "/opt/cni/bin/disable-ipv6",
		Args:   append([]string{"set-ipv6"}, path),
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("reexec to set IPv6 failed: %v", err)
	}
	return nil
}

func (g *Galaxy) getPod(name, namespace string) (*corev1.Pod, error) {
	var pod *corev1.Pod
	printOnce := false
	if err := wait.PollImmediate(time.Millisecond*500, 5*time.Second, func() (done bool, err error) {
		pod, err = g.client.CoreV1().Pods(namespace).Get(name, v1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				if printOnce == false {
					printOnce = true
					glog.Warningf("can't find pod %s_%s, retring", name, namespace)
				}
				return false, nil
			}
			return false, err
		}
		return true, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to get pod %s_%s: %v", name, namespace, err)
	}
	return pod, nil
}

func convertResult(result types.Result) (*t020.Result, error) {
	if result == nil {
		return nil, fmt.Errorf("result is nil")
	}
	result020, ok := result.(*t020.Result)
	if !ok {
		return nil, fmt.Errorf("faild to convert result to 020 result")
	}
	if result020.IP4 == nil {
		return nil, fmt.Errorf("CNI plugin reported no IPv4 address")
	}
	ip4 := result020.IP4.IP.IP.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("CNI plugin reported an invalid IPv4 address: %+v.", result020.IP4)
	}
	return result020, nil
}

func setNetInterface(netIf string, idx int, argIf string) string {
	if idx == 0 {
		return argIf
	}
	if netIf != "" {
		return netIf
	}
	return fmt.Sprintf("eth%d", idx)
}
