package galaxy

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	galaxyapi "git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"git.code.oa.com/gaiastack/galaxy/pkg/flags"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/flannel"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/remote"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
	"k8s.io/client-go/1.4/pkg/util/wait"
)

var (
	flagMaster               = flag.String("master", "", "URL of galaxy master controller, currently apiswitch")
	flagNetworkConf          = flag.String("network-conf", `{"galaxy-flannel":{"delegate":{"type":"galaxy-bridge","isDefaultGateway":true,"forceAddress":true},"subnetFile":"/run/flannel/subnet.env"}}`, "various network configrations")
	flagBridgeNFCallIptables = flag.Bool("bridge-nf-call-iptables", true, "ensure bridge-nf-call-iptables is set/unset")
	flagIPForward            = flag.Bool("ip-forward", true, "ensure ip-forward is set/unset")
	flagEbtableRules         = flag.Bool("ebtable-rules", false, "whether galaxy should ensure ebtable-rules")
	flagApiServer            = flag.String("api-servers", "", "The address of apiserver")
	flagLabelSubnet          = flag.Bool("label-subnet", true, "whether galaxy should label the kubelet node with subnet={ip}-{onesInMask}, this label is used by rackfilter scheduler plugin")
)

func (g *Galaxy) startServer() error {
	g.installHandlers()
	if err := os.Remove(galaxyapi.GalaxySocketPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %v", galaxyapi.GalaxySocketPath, err)
		}
	}
	l, err := net.Listen("unix", galaxyapi.GalaxySocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on pod info socket: %v", err)
	}
	if err := os.Chmod(galaxyapi.GalaxySocketPath, 0600); err != nil {
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
	req, err := galaxyapi.CniRequestToPodRequest(r.Request)
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
				err = g.setupPortMapping(req, req.Ports, req.ContainerID, result)
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

func (g *Galaxy) cmdAdd(req *galaxyapi.PodRequest) (*types.Result, error) {
	if err := disableIPv6(req.Netns); err != nil {
		glog.Warningf("Error disable ipv6 %v", err)
	}
	if *flagMaster == "" {
		req.CmdArgs.StdinData = g.flannelConf
		return flannel.CmdAdd(req.CmdArgs)
	}
	return remote.CmdAdd(req, *flagMaster, flags.GetNodeIP(), g.netConf)
}

func (g *Galaxy) cmdDel(req *galaxyapi.PodRequest) error {
	if *flagMaster == "" {
		req.CmdArgs.StdinData = g.flannelConf
		return flannel.CmdDel(req.CmdArgs)
	}
	return remote.CmdDel(req, g.netConf)
}

func (g *Galaxy) setupPortMapping(req *galaxyapi.PodRequest, portStr, containerID string, result *types.Result) error {
	if g.client == nil {
		return nil
	}
	ports, err := k8s.ParsePorts(portStr)
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		return nil
	}
	pod, err := g.client.Pods(req.PodNamespace).Get(req.PodName)
	if err != nil {
		return fmt.Errorf("failed to get pod from apiserver: %v", err)
	}
	_, portMapping := pod.Annotations[k8s.PortMappingAnnotation]
	if err := g.pmhandler.OpenHostports(k8s.GetPodFullName(req.PodName, req.PodNamespace), portMapping, ports); err != nil {
		return err
	}
	data, err := json.Marshal(ports)
	if err != nil {
		return fmt.Errorf("failed to marshal ports: %v", err)
	}
	if err := k8s.SavePort(containerID, data); err != nil {
		return fmt.Errorf("failed to save ports %v", err)
	}
	// we have to fulfill ip field of the current pod
	if result.IP4 == nil {
		return fmt.Errorf("CNI plugin reported no IPv4 address")
	}
	ip4 := result.IP4.IP.IP.To4()
	if ip4 == nil {
		return fmt.Errorf("CNI plugin reported an invalid IPv4 address: %+v.", result.IP4)
	}
	for _, p := range ports {
		p.PodIP = ip4.String()
	}
	if err := g.pmhandler.SetupPortMapping("cni0", ports); err != nil {
		return fmt.Errorf("failed to setup port mapping %v: %v", ports, err)
	}
	if err := wait.Poll(10*time.Millisecond, 1*time.Minute, func() (bool, error) {
		pod, err := g.client.Pods(req.PodNamespace).Get(req.PodName)
		if err != nil {
			return false, err
		}
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[k8s.PortMappingPortsAnnotation] = string(data)
		_, err = g.client.Pods(req.PodNamespace).Update(pod)
		if err == nil {
			return true, nil
		}
		glog.Warningf("failed to update pod %s annotation: %v", k8s.GetPodFullName(pod.Name, pod.Namespace), err)
		if k8s.ShouldRetry(err) {
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
