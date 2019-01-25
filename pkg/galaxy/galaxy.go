package galaxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/flags"
	"git.code.oa.com/gaiastack/galaxy/pkg/gc"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/kernel"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/masq"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/portmapping"
	"git.code.oa.com/gaiastack/galaxy/pkg/policy"
	utildbus "git.code.oa.com/gaiastack/galaxy/pkg/utils/dbus"
	utiliptables "git.code.oa.com/gaiastack/galaxy/pkg/utils/iptables"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	utilexec "k8s.io/utils/exec"
)

type Galaxy struct {
	quitChan        chan struct{}
	dockerCli       *docker.DockerInterface
	netConf         map[string]map[string]interface{}
	pmhandler       *portmapping.PortMappingHandler
	client          *kubernetes.Clientset
	pm              *policy.PolicyManager
	underlayCNIIPAM bool // if set, galaxy delegates to a cni ipam to allocate ip for underlay network
}

func NewGalaxy() (*Galaxy, error) {
	dockerClient, err := docker.NewDockerInterface()
	if err != nil {
		return nil, err
	}
	g := &Galaxy{
		quitChan: make(chan struct{}),
	}
	g.dockerCli = dockerClient
	if err := g.parseConfig(); err != nil {
		return nil, err
	}
	natInterfaceName := ""
	if overlayConfMap, ok := g.netConf[private.NetworkTypeOverlay.CNIType]; ok {
		if delegateConf, ok := overlayConfMap["delegate"]; ok {
			if delegateConfMap, ok := delegateConf.(map[string]interface{}); ok {
				if typ, ok := delegateConfMap["type"]; ok && typ == private.CNIBridgePlugin {
					natInterfaceName = "cni0"
				}
			}
		}
	}
	g.pmhandler = portmapping.New(natInterfaceName)
	return g, nil
}

func (g *Galaxy) Start() error {
	g.initk8sClient()
	g.pm = policy.New(g.client, g.quitChan)
	gc.NewFlannelGC(g.dockerCli, g.quitChan, g.cleanIPtables).Run()
	kernel.BridgeNFCallIptables(g.quitChan, *flagBridgeNFCallIptables)
	kernel.IPForward(g.quitChan, *flagIPForward)
	if err := g.setupIPtables(); err != nil {
		return err
	}
	go wait.Until(g.pm.Run, 3*time.Minute, g.quitChan)
	if v1, exist := g.netConf["galaxy-flannel"]; exist {
		if v2, exist := v1["subnetFile"]; exist {
			if file, ok := v2.(string); ok && file == "/etc/kubernetes/subnet.env" {
				// assume we are in vpc(tce/qcloud) mode when subnetFiel = `/etc/kubernetes/subnet.env`
				glog.Infof("in vpc mode, setup masq")
				iptablesCli := utiliptables.New(utilexec.New(), utildbus.New(), utiliptables.ProtocolIpv4)
				go wait.Forever(masq.EnsureIPMasq(iptablesCli), time.Minute)
			}
		}
	}
	return g.startServer()
}

func (g *Galaxy) Stop() error {
	close(g.quitChan)
	g.quitChan = make(chan struct{})
	return nil
}

func (g *Galaxy) parseConfig() error {
	if strings.TrimSpace(*flagNetworkConf) == "" {
		return fmt.Errorf("No network configured")
	}
	var networkConf map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(*flagNetworkConf), &networkConf); err != nil {
		return fmt.Errorf("Error unmarshal network config %s: %v", *flagNetworkConf, err)
	}
	if len(networkConf) == 0 {
		return fmt.Errorf("No network configured")
	} else {
		g.netConf = networkConf
		glog.Infof("Network config %s", *flagNetworkConf)
	}
	for k, v := range g.netConf {
		if v == nil {
			g.netConf[k] = make(map[string]interface{})
		}
		if _, ok := g.netConf[k]["type"]; !ok {
			g.netConf[k]["type"] = k
		}
	}
	if vlanConfMap, ok := g.netConf[private.NetworkTypeUnderlay.CNIType]; ok {
		if ipamObj, exist := vlanConfMap["ipam"]; exist {
			if ipamMap, isMap := ipamObj.(map[string]interface{}); isMap {
				ipamMap["node_ip"] = flags.GetNodeIP()
				if _, hasType := ipamMap["type"]; hasType {
					g.underlayCNIIPAM = true
				}
			}
		}
	}
	glog.Infof("normalized network config %v", g.netConf)
	return nil
}

func (g *Galaxy) initk8sClient() {
	if *flagApiServer == "" && *flagKubeConf == "" {
		// galaxy currently not support running in pod, so either flagApiServer or flagKubeConf should be specified
		glog.Fatal("apiserver address unknown")
	}
	clientConfig, err := clientcmd.BuildConfigFromFlags(*flagApiServer, *flagKubeConf)
	if err != nil {
		glog.Fatalf("Invalid client config: error(%v)", err)
	}
	clientConfig.QPS = 1000.0
	clientConfig.Burst = 2000
	glog.Infof("QPS: %e, Burst: %d", clientConfig.QPS, clientConfig.Burst)

	g.client, err = kubernetes.NewForConfig(clientConfig)
	if err != nil {
		glog.Fatalf("Can not generate client from config: error(%v)", err)
	}
	glog.Infof("apiserver address %s, kubeconf %s", *flagApiServer, *flagKubeConf)
}
