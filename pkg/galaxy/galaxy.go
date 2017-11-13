package galaxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"git.code.oa.com/gaiastack/galaxy/pkg/flags"
	"git.code.oa.com/gaiastack/galaxy/pkg/gc"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/firewall"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/kernel"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/portmapping"

	"k8s.io/client-go/1.4/kubernetes"
	"k8s.io/client-go/1.4/pkg/util/wait"
	"k8s.io/client-go/1.4/tools/clientcmd"
)

type Galaxy struct {
	quitChannels []chan error
	cleaner      gc.GC
	netConf      map[string]map[string]interface{}
	flannelConf  []byte
	pmhandler    *portmapping.PortMappingHandler
	client       *kubernetes.Clientset
}

func NewGalaxy() (*Galaxy, error) {
	dockerClient, err := docker.NewDockerInterface()
	if err != nil {
		return nil, err
	}
	g := &Galaxy{}
	if err := g.parseConfig(); err != nil {
		return nil, err
	}
	g.cleaner = gc.NewFlannelGC(dockerClient, g.newQuitChannel(), g.newQuitChannel())
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
	g.cleaner.Run()
	kernel.BridgeNFCallIptables(g.newQuitChannel(), *flagBridgeNFCallIptables)
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
	if _, ok := g.netConf["galaxy-k8s-vlan"]; ok {
		g.netConf["galaxy-k8s-vlan"]["url"] = *flagMaster
		g.netConf["galaxy-k8s-vlan"]["node_ip"] = flags.GetNodeIP()
	}
	if _, ok := g.netConf["galaxy-flannel"]; ok {
		var err error
		if g.flannelConf, err = json.Marshal(g.netConf["galaxy-flannel"]); err != nil {
			return err
		}
	}
	glog.Infof("normalized network config %v", g.netConf)
	return nil
}

func (g *Galaxy) initk8sClient() {
	if *flagApiServer == "" {
		glog.Infof("apiserver address unknown")
		return
	}
	clientConfig, err := clientcmd.BuildConfigFromFlags(*flagApiServer, "")
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
	glog.Infof("apiserver address %s", *flagApiServer)
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
				node, err := g.client.Nodes().Get(nodeName)
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
				node.Labels["subnet"] = strings.Replace(flags.GetNodeIP(), "/", "-", 1) //subnet=10.235.7.146-26
				_, err = g.client.Nodes().Update(node)
				if err == nil {
					glog.Infof("created kubelet label subnet=%s", node.Labels["subnet"])
					return
				}
				glog.Warningf("failed to update node label: %v", err)
				if !k8s.ShouldRetry(err) {
					return
				}
			}
		}, 3*time.Minute)
	}
}
