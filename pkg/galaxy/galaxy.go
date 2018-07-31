package galaxy

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"reflect"
	"strings"
	"time"

	zhiyunapi "git.code.oa.com/gaiastack/galaxy/cni/zhiyun-ipam/api"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/flags"
	"git.code.oa.com/gaiastack/galaxy/pkg/gc"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/firewall"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/kernel"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/portmapping"
	"git.code.oa.com/gaiastack/galaxy/pkg/policy"

	"github.com/golang/glog"
	"github.com/vishvananda/netlink"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Galaxy struct {
	quitChan        chan struct{}
	dockerCli       *docker.DockerInterface
	netConf         map[string]map[string]interface{}
	pmhandler       *portmapping.PortMappingHandler
	client          *kubernetes.Clientset
	zhiyunConf      *zhiyunapi.Conf
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
	g.pmhandler = portmapping.New()
	return g, nil
}

func (g *Galaxy) Start() error {
	g.initk8sClient()
	g.pm = policy.New(g.client, g.quitChan)
	gc.NewFlannelGC(g.dockerCli, g.quitChan).Run()
	if g.zhiyunConf != nil {
		gc.NewZhiyunGC(g.dockerCli, g.quitChan, g.zhiyunConf).Run()
	}
	kernel.BridgeNFCallIptables(g.quitChan, *flagBridgeNFCallIptables)
	kernel.IPForward(g.quitChan, *flagIPForward)
	if *flagEbtableRules {
		firewall.SetupEbtables(g.quitChan)
	}
	firewall.EnsureIptables(g.pmhandler, g.quitChan)
	go wait.Until(g.pm.Run, 3*time.Minute, g.quitChan)
	go wait.Until(g.updateIPInfoCM, time.Minute, g.quitChan)
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
				if typ, hasType := ipamMap["type"]; hasType {
					g.underlayCNIIPAM = true
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

func (g *Galaxy) updateIPInfoCM() {
	defer func() {
		s1 := rand.NewSource(time.Now().UnixNano())
		r1 := rand.New(s1)
		s := 1 + r1.Int31n(10)
		time.Sleep(time.Duration(s) * time.Second)
	}()
	data, err := getLocalAddr()
	if err != nil {
		glog.Errorf("can't get local addr: %v", err.Error())
		return
	}
	if len(data) == 0 {
		return
	}
	s, _ := json.Marshal(data)
	nodeIPN := flags.GetNodeIP()
	nodeIP, _, err := net.ParseCIDR(nodeIPN)
	if err != nil {
		glog.Errorf("node ip error: %v", err)
		return
	}
	cm, err := g.client.CoreV1().ConfigMaps("default").Get("ipinfo", v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			cm = &core.ConfigMap{
				TypeMeta: v1.TypeMeta{
					Kind: "ConfigMap",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:      "ipinfo",
					Namespace: "default",
				},
				Data: map[string]string{
					nodeIP.String(): string(s),
				},
			}
			_, err = g.client.CoreV1().ConfigMaps("default").Create(cm)
			if err != nil {
				glog.Errorf(err.Error())
			}
		} else {
			glog.Error(err)
		}
		return
	}
	if oldData, exist := cm.Data[nodeIP.String()]; !exist {
		s, _ := json.Marshal(data)
		cm.Data[nodeIP.String()] = string(s)
		_, err = g.client.CoreV1().ConfigMaps("default").Update(cm)
		if err != nil {
			glog.Errorf(err.Error())
		}
	} else {
		od := map[string]string{}
		err = json.Unmarshal([]byte(oldData), &od)
		if err != nil {
			glog.Error(err.Error())
			return
		}
		if reflect.DeepEqual(data, od) {
			glog.Infof("ipinfo up to date")
		} else {
			s, _ := json.Marshal(data)
			cm.Data[nodeIP.String()] = string(s)
			_, err = g.client.CoreV1().ConfigMaps("default").Update(cm)
			if err != nil {
				glog.Errorf(err.Error())
			}
		}
	}
}

func getLocalAddr() (map[string]string, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}
	data := make(map[string]string)
	for _, link := range links {
		if omitLink(link.Attrs().Name) {
			continue
		}
		if link.Type() == "ipip" || link.Type() == "gre" {
			continue
		}
		addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return nil, err
		}
		if len(addrs) >= 1 && len(addrs[0].IP) != 0 {
			data[link.Attrs().Name] = addrs[0].IP.String()
		}
	}
	return data, nil
}

func omitLink(linkName string) bool {
	if strings.HasPrefix(linkName, "veth") || strings.HasPrefix(linkName, "tunl") ||
		strings.HasPrefix(linkName, "cni") || linkName == "lo" || linkName == "flannel.ipip" {
		return true
	}
	return false
}
