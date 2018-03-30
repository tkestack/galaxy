package galaxy

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/golang/glog"

	zhiyunapi "git.code.oa.com/gaiastack/galaxy/cni/zhiyun-ipam/api"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	k8sutil "git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/utils"
	"git.code.oa.com/gaiastack/galaxy/pkg/flags"
	"git.code.oa.com/gaiastack/galaxy/pkg/gc"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/firewall"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/kernel"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/portmapping"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type Galaxy struct {
	quitChannels []chan error
	dockerCli    *docker.DockerInterface
	netConf      map[string]map[string]interface{}
	pmhandler    *portmapping.PortMappingHandler
	client       *kubernetes.Clientset
	podStore     corev1lister.PodLister
	zhiyunConf   *zhiyunapi.Conf
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
	g.labelSubnet()
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
	lw := cache.NewListWatchFromClient(g.client.CoreV1().RESTClient(), "pods", v1.NamespaceAll, fields.OneTermEqualSelector("spec.nodeName", k8s.GetHostname("")))
	indexer, r := cache.NewNamespaceKeyedIndexerAndReflector(lw, &corev1.Pod{}, 0)
	go r.Run(make(chan struct{}))
	g.podStore = corev1lister.NewPodLister(indexer)
}

// labelSubnet labels kubelet on this node with subnet=IP-OnesInMask to provide rack information for kube-scheduler
// rackfilter plugin to work
func (g *Galaxy) labelSubnet() {
	if g.client != nil {
		if !(*flagLabelSubnet) {
			return
		}
		go wait.Forever(func() {
			nodeName := k8s.GetHostname("") //TODO get hostnameOverride from kubelet config
			for i := 0; i < 5; i++ {
				node, err := g.client.CoreV1().Nodes().Get(nodeName, v1.GetOptions{})
				if err != nil {
					glog.Warningf("failed to get node %s from apiserver", nodeName)
					return
				}
				if node.Labels == nil {
					node.Labels = make(map[string]string)
				}
				if subnet, ok := node.Labels["subnet"]; ok {
					if subnet != strings.Replace(flags.GetNodeIP(), "/", "-", 1) {
						glog.Infof("kubernete label subnet=%s is not created by galaxy. galaxy won't change this label", subnet, strings.Replace(flags.GetNodeIP(), "/", "-", 1))
					}
					return
				}
				_, ipNet, err := net.ParseCIDR(flags.GetNodeIP())
				if err != nil {
					glog.Errorf("invalid node ip %s", flags.GetNodeIP())
					return
				}
				node.Labels["subnet"] = strings.Replace(ipNet.String(), "/", "-", 1) //cidr=10.235.7.146/24 -> subnet=10.235.7.0-24
				_, err = g.client.CoreV1().Nodes().Update(node)
				if err == nil {
					glog.Infof("created kubelet label subnet=%s", node.Labels["subnet"])
					return
				}
				glog.Warningf("failed to update node label: %v", err)
				if !k8sutil.ShouldRetry(err) {
					return
				}
			}
		}, 3*time.Minute)
	}
}
