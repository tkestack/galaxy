package galaxy

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	galaxyapi "git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	k8sutil "git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/utils"

	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	flagMaster               = flag.String("master", "", "URL of galaxy master controller, currently apiswitch")
	flagNetworkConf          = flag.String("network-conf", `{"galaxy-flannel":{"delegate":{"type":"galaxy-bridge","isDefaultGateway":true,"forceAddress":true},"subnetFile":"/run/flannel/subnet.env"}}`, "various network configrations")
	flagBridgeNFCallIptables = flag.Bool("bridge-nf-call-iptables", true, "ensure bridge-nf-call-iptables is set/unset")
	flagIPForward            = flag.Bool("ip-forward", true, "ensure ip-forward is set/unset")
	flagEbtableRules         = flag.Bool("ebtable-rules", false, "whether galaxy should ensure ebtable-rules")
	flagApiServer            = flag.String("api-servers", "", "The address of apiserver")
	flagKubeConf             = flag.String("kubeconf", "", "kube configure file")
	flagLabelSubnet          = flag.Bool("label-subnet", true, "whether galaxy should label the kubelet node with subnet={maskedIP}-{onesInMask}, this label is used by rackfilter scheduler plugin")
	Note                     = `If ipam type is from third party, e.g. zhiyun, galaxy invokes third party ipam binaries to allocate ip.`
)

func (g *Galaxy) startServer() error {
	g.installHandlers()
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
		l.Close()
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
	defer r.Request.Body.Close()
	req, err := galaxyapi.CniRequestToPodRequest(data)
	if err != nil {
		glog.Warningf("bad request %v", err)
		http.Error(w, fmt.Sprintf("%v", err), http.StatusBadRequest)
		return
	}
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

func (g *Galaxy) requestFunc(req *galaxyapi.PodRequest) (data []byte, err error) {
	start := time.Now()
	glog.Infof("%v, %s+", req, start.Format(time.StampMicro))
	if req.Command == cniutil.COMMAND_ADD {
		defer func() {
			glog.Infof("%v, data %s, err %v, %s-", req, string(data), err, start.Format(time.StampMicro))
		}()
		result, err1 := g.cmdAdd(req)
		if err1 != nil {
			err = err1
		} else {
			if result != nil {
				data, err = json.Marshal(result)
				if result020, ok := result.(*t020.Result); !ok {
					err = fmt.Errorf("faild to convert result to 020 result")
				} else {
					err = g.setupPortMapping(req, req.ContainerID, result020)
				}
			}
		}
	} else if req.Command == cniutil.COMMAND_DEL {
		defer glog.Infof("%v err %v, %s-", req, err, start.Format(time.StampMicro))
		err = g.cmdDel(req)
		if err == nil {
			err = g.cleanupPortMapping(req)
		}
	} else {
		err = fmt.Errorf("unkown command %s", req.Command)
	}
	return
}

func (g *Galaxy) cmdAdd(req *galaxyapi.PodRequest) (types.Result, error) {
	if err := disableIPv6(req.Netns); err != nil {
		glog.Warningf("Error disable ipv6 %v", err)
	}
	// get network type from pods' labels
	var (
		networkInfo cniutil.NetworkInfo
		pod         *corev1.Pod
	)
	if err := wait.PollImmediate(time.Millisecond*500, 5*time.Second, func() (done bool, err error) {
		pod, err = g.podStore.Pods(req.PodNamespace).Get(req.PodName)
		if err != nil {
			if errors.IsNotFound(err) {
				glog.Warningf("can't find pod %s_%s, retring", req.PodName, req.PodNamespace)
				return false, nil
			}
			return false, err
		}
		return true, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to get pod %s_%s: %v", req.PodName, req.PodNamespace, err)
	}
	if pod.Labels == nil {
		networkInfo = cniutil.NetworkInfo{private.NetworkTypeOverlay.CNIType: {}}
	} else {
		networkType := pod.Labels[private.LabelKeyNetworkType]
		if private.NetworkTypeOverlay.Has(networkType) {
			networkInfo = cniutil.NetworkInfo{private.NetworkTypeOverlay.CNIType: {}}
		} else if private.NetworkTypeUnderlay.Has(networkType) {
			if pod.Annotations != nil && pod.Annotations[private.AnnotationKeyIPInfo] != "" {
				req.CmdArgs.Args = fmt.Sprintf("%s;%s=%s", req.CmdArgs.Args, cniutil.IPInfoInArgs, pod.Annotations[private.AnnotationKeyIPInfo])
			}
			networkInfo = cniutil.NetworkInfo{private.NetworkTypeUnderlay.CNIType: {}}
		} else {
			return nil, fmt.Errorf("unsupported network type: %s", networkType)
		}
	}
	return cniutil.CmdAdd(req.ContainerID, req.CmdArgs, g.netConf, networkInfo)
}

func (g *Galaxy) cmdDel(req *galaxyapi.PodRequest) error {
	return cniutil.CmdDel(req.ContainerID, req.CmdArgs, g.netConf)
}

func (g *Galaxy) setupPortMapping(req *galaxyapi.PodRequest, containerID string, result *t020.Result) error {
	if g.client == nil {
		return nil
	}
	if len(req.Ports) == 0 {
		return nil
	}
	// we have to fulfill ip field of the current pod
	if result.IP4 == nil {
		return fmt.Errorf("CNI plugin reported no IPv4 address")
	}
	ip4 := result.IP4.IP.IP.To4()
	if ip4 == nil {
		return fmt.Errorf("CNI plugin reported an invalid IPv4 address: %+v.", result.IP4)
	}
	for i := range req.Ports {
		req.Ports[i].PodIP = ip4.String()
		req.Ports[i].PodName = req.PodName
	}
	pod, err := g.client.CoreV1().Pods(req.PodNamespace).Get(req.PodName, v1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod from apiserver: %v", err)
	}
	var randomPortMapping bool
	if pod.Labels != nil && pod.Labels[private.LabelKeyNetworkType] == private.LabelValueNetworkTypeNAT {
		randomPortMapping = true
	} else {
		var newPorts []k8s.Port
		for i := range req.Ports {
			if req.Ports[i].HostPort != 0 {
				newPorts = append(newPorts, req.Ports[i])
			}
		}
		if len(newPorts) == 0 {
			return nil
		}
		req.Ports = newPorts
	}
	if err := g.pmhandler.OpenHostports(k8s.GetPodFullName(req.PodName, req.PodNamespace), randomPortMapping, req.Ports); err != nil {
		return err
	}
	data, err := json.Marshal(req.Ports)
	if err != nil {
		return fmt.Errorf("failed to marshal ports: %v", err)
	}
	if err := k8s.SavePort(containerID, data); err != nil {
		return fmt.Errorf("failed to save ports %v", err)
	}
	if err := g.pmhandler.SetupPortMapping("cni0", req.Ports); err != nil {
		return fmt.Errorf("failed to setup port mapping %v: %v", req.Ports, err)
	}
	if err := wait.Poll(10*time.Millisecond, 1*time.Minute, func() (bool, error) {
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
	}); err != nil {
		return fmt.Errorf("failed to update pod %s annotation: %v", k8s.GetPodFullName(req.PodName, req.PodNamespace), err)
	}
	return nil
}

func (g *Galaxy) cleanupPortMapping(req *galaxyapi.PodRequest) error {
	g.pmhandler.CloseHostports(k8s.GetPodFullName(req.PodName, req.PodNamespace))
	ports, err := k8s.ConsumePort(req.ContainerID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read ports %v", err)
	}
	if len(ports) != 0 {
		g.pmhandler.CleanPortMapping("cni0", ports)
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
