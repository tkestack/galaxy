package galaxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	zhiyunapi "git.code.oa.com/gaiastack/galaxy/cni/zhiyun-ipam/api"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/eventhandler"
	"git.code.oa.com/gaiastack/galaxy/pkg/flags"
	"git.code.oa.com/gaiastack/galaxy/pkg/gc"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/firewall"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/kernel"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/portmapping"
	"git.code.oa.com/gaiastack/galaxy/pkg/policy"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	corev1informer "k8s.io/client-go/informers/core/v1"
	networkingv1informer "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type Galaxy struct {
	quitChannels []chan error
	dockerCli    *docker.DockerInterface
	netConf      map[string]map[string]interface{}
	pmhandler    *portmapping.PortMappingHandler
	client       *kubernetes.Clientset
	zhiyunConf   *zhiyunapi.Conf
	pm           *policy.PolicyManager
	*eventhandler.PodEventHandler
	plcyHandler  *eventhandler.NetworkPolicyEventHandler
	podInformer  corev1informer.PodInformer
	plcyInformer networkingv1informer.NetworkPolicyInformer
}

func NewGalaxy() (*Galaxy, error) {
	dockerClient, err := docker.NewDockerInterface()
	if err != nil {
		return nil, err
	}
	g := &Galaxy{}
	g.dockerCli = dockerClient
	if err := g.parseConfig(); err != nil {
		return nil, err
	}
	g.pmhandler = portmapping.New()
	return g, nil
}

func (g *Galaxy) newQuitChannel() chan error {
	quitChannel := make(chan error)
	g.quitChannels = append(g.quitChannels, quitChannel)
	return quitChannel
}

func (g *Galaxy) Start() error {
	g.initk8sClient()
	g.pm = policy.New(g.client)
	g.initInformers()
	gc.NewFlannelGC(g.dockerCli, g.newQuitChannel(), g.newQuitChannel()).Run()
	if g.zhiyunConf != nil {
		gc.NewZhiyunGC(g.dockerCli, g.newQuitChannel(), g.zhiyunConf).Run()
	}
	kernel.BridgeNFCallIptables(g.newQuitChannel(), *flagBridgeNFCallIptables)
	kernel.IPForward(g.newQuitChannel(), *flagIPForward)
	if *flagEbtableRules {
		firewall.SetupEbtables(g.newQuitChannel())
	}
	firewall.EnsureIptables(g.pmhandler, g.newQuitChannel())
	go wait.Forever(g.pm.Run, time.Minute)
	return g.startServer()
}

func (g *Galaxy) Stop() error {
	// Stop and wait on all quit channels.
	for i, c := range g.quitChannels {
		// Send the exit signal and wait on the thread to exit (by closing the channel).
		c <- nil
		err := <-c
		if err != nil {
			// Remove the channels that quit successfully.
			g.quitChannels = g.quitChannels[i:]
			return err
		}
	}
	g.quitChannels = make([]chan error, 0)
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
		vlanConfMap["url"] = *flagMaster
		vlanConfMap["node_ip"] = flags.GetNodeIP()
		if ipamObj, exist := vlanConfMap["ipam"]; exist {
			if ipamMap, isMap := ipamObj.(map[string]interface{}); isMap {
				ipamMap["node_ip"] = flags.GetNodeIP()
				if typ, hasType := ipamMap["type"]; hasType {
					if typeStr, isStr := typ.(string); isStr {
						if typeStr == private.IPAMTypeZhiyun {
							var zhiyunConf zhiyunapi.Conf
							data, _ := json.Marshal(ipamMap)
							if err := json.Unmarshal(data, &zhiyunConf); err != nil {
								return fmt.Errorf("failed to unmarshal zhiyun conf %s: %v", string(data), err)
							}
							g.zhiyunConf = &zhiyunConf
						}
					}
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

func (g *Galaxy) initInformers() {
	//podInformerFactory := informers.NewFilteredSharedInformerFactory(g.client, time.Minute, v1.NamespaceAll, func(listOptions *v1.ListOptions) {
	//	listOptions.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", k8s.GetHostname("")).String()
	//})
	podInformerFactory := informers.NewSharedInformerFactory(g.client, 0)
	networkingInformerFactory := informers.NewSharedInformerFactory(g.client, 0)
	g.podInformer = podInformerFactory.Core().V1().Pods()
	g.plcyInformer = networkingInformerFactory.Networking().V1().NetworkPolicies()
	g.PodEventHandler = eventhandler.NewPodEventHandler(g.pm)
	g.plcyHandler = eventhandler.NewNetworkPolicyEventHandler(g.pm)
	g.podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    g.OnAdd,
		UpdateFunc: g.OnUpdate,
		DeleteFunc: g.OnDelete,
	})
	g.plcyInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    g.plcyHandler.OnAdd,
		UpdateFunc: g.plcyHandler.OnUpdate,
		DeleteFunc: g.plcyHandler.OnDelete,
	})
	go podInformerFactory.Start(make(chan struct{}))
	go networkingInformerFactory.Start(make(chan struct{}))
}
