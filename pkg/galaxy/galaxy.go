package galaxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/galaxy/options"
	"git.code.oa.com/gaiastack/galaxy/pkg/gc"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/kernel"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/portmapping"
	"git.code.oa.com/gaiastack/galaxy/pkg/policy"
	"git.code.oa.com/gaiastack/galaxy/pkg/tke/eni"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Galaxy struct {
	*options.ServerRunOptions
	quitChan  chan struct{}
	dockerCli *docker.DockerInterface
	netConf   map[string]map[string]interface{}
	pmhandler *portmapping.PortMappingHandler
	client    *kubernetes.Clientset
	pm        *policy.PolicyManager
}

func NewGalaxy() (*Galaxy, error) {
	dockerClient, err := docker.NewDockerInterface()
	if err != nil {
		return nil, err
	}
	g := &Galaxy{
		ServerRunOptions: options.NewServerRunOptions(),
		quitChan:         make(chan struct{}),
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
	kernel.BridgeNFCallIptables(g.quitChan, g.BridgeNFCallIptables)
	kernel.IPForward(g.quitChan, g.IPForward)
	if err := g.setupIPtables(); err != nil {
		return err
	}
	go wait.Until(g.pm.Run, 3*time.Minute, g.quitChan)

	if g.RouteENI {
		kernel.DisableRPFilter(g.quitChan)
		go eni.SetupENIs(g.quitChan)
	}
	return g.startServer()
}

func (g *Galaxy) Stop() error {
	close(g.quitChan)
	g.quitChan = make(chan struct{})
	return nil
}

func (g *Galaxy) parseConfig() error {
	if strings.TrimSpace(g.NetworkConf) == "" {
		return fmt.Errorf("No network configured")
	}
	var networkConf map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(g.NetworkConf), &networkConf); err != nil {
		return fmt.Errorf("Error unmarshal network config %s: %v", g.NetworkConf, err)
	}
	if len(networkConf) == 0 {
		return fmt.Errorf("No network configured")
	} else {
		g.netConf = networkConf
		glog.Infof("Network config %s", g.NetworkConf)
	}
	for k, v := range g.netConf {
		if v == nil {
			g.netConf[k] = make(map[string]interface{})
		}
		if _, ok := g.netConf[k]["type"]; !ok {
			g.netConf[k]["type"] = k
		}
	}
	glog.Infof("normalized network config %v", g.netConf)
	return nil
}

func (g *Galaxy) initk8sClient() {
	if g.Master == "" && g.KubeConf == "" {
		// galaxy currently not support running in pod, so either flagApiServer or flagKubeConf should be specified
		glog.Fatal("apiserver address unknown")
	}
	clientConfig, err := clientcmd.BuildConfigFromFlags(g.Master, g.KubeConf)
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
	glog.Infof("apiserver address %s, kubeconf %s", g.Master, g.KubeConf)
}
