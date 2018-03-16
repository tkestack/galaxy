package schedulerplugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	tappv1 "git.code.oa.com/gaia/tapp-controller/pkg/apis/tappcontroller/v1alpha1"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/schedulerapi"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Conf struct {
	FloatingIPs        []*floatingip.FloatingIP `json:"floatingips,omitempty"`
	DBConfig           *database.DBConfig       `json:"database"`
	ResyncInterval     uint                     `json:"resyncInterval"`
	ConfigMapName      string                   `json:"configMapName"`
	ConfigMapNamespace string                   `json:"configMapNamespace"`
}

// FloatingIPPlugin Allocates Floating IP for deployments
type FloatingIPPlugin struct {
	objectSelector, nodeSelector labels.Selector
	// whether or not the deployment wants its allocated floatingips immutable accross pod reassigning
	immutableSeletor labels.Selector
	ipam             floatingip.IPAM
	// node name to subnet cache
	nodeSubnet     map[string]*net.IPNet
	nodeSubnetLock sync.Mutex
	sync.Mutex
	*PluginFactoryArgs
	lastFIPConf string
	conf        *Conf
	unreleased  chan *corev1.Pod
}

func NewFloatingIPPlugin(conf Conf, args *PluginFactoryArgs) (*FloatingIPPlugin, error) {
	if conf.ResyncInterval < 1 {
		conf.ResyncInterval = 1
	}
	if conf.ConfigMapName == "" {
		conf.ConfigMapName = "floatingip-config"
	}
	if conf.ConfigMapNamespace == "" {
		conf.ConfigMapNamespace = "kube-system"
	}
	glog.Infof("floating ip config: %v", conf)
	db := database.NewDBRecorder(conf.DBConfig)
	if err := db.Run(); err != nil {
		return nil, err
	}
	ipam := floatingip.NewIPAM(db)
	plugin := &FloatingIPPlugin{
		ipam:              ipam,
		nodeSubnet:        make(map[string]*net.IPNet),
		PluginFactoryArgs: args,
		conf:              &conf,
		unreleased:        make(chan *corev1.Pod, 10),
	}
	plugin.initSelector()
	return plugin, nil
}

func (p *FloatingIPPlugin) Init() error {
	if len(p.conf.FloatingIPs) > 0 {
		if err := p.ipam.ConfigurePool(p.conf.FloatingIPs); err != nil {
			return err
		}
	} else {
		glog.Infof("empty floatingips from config, fetching from configmap")
		if err := wait.PollInfinite(time.Second, func() (done bool, err error) {
			updated, err := p.updateConfigMap()
			if err != nil {
				glog.Warning(err)
			}
			return updated, nil
		}); err != nil {
			return fmt.Errorf("failed to get floatingip config from configmap: %v", err)
		}
	}
	return nil
}

func (p *FloatingIPPlugin) Run(stop chan struct{}) {
	if len(p.conf.FloatingIPs) == 0 {
		go wait.Until(func() {
			if _, err := p.updateConfigMap(); err != nil {
				glog.Warning(err)
			}
		}, time.Minute, stop)
	}
	go wait.Until(func() {
		if err := p.resyncPod(); err != nil {
			glog.Warning(err)
		}
		p.syncPodIPsIntoDB()
	}, time.Duration(p.conf.ResyncInterval)*time.Minute, stop)
	go p.loop(stop)
}

func (p *FloatingIPPlugin) initSelector() error {
	objectSelectorMap := make(map[string]string)
	objectSelectorMap[private.LabelKeyNetworkType] = private.LabelValueNetworkTypeFloatingIP
	nodeSelectorMap := make(map[string]string)
	nodeSelectorMap[private.LabelKeyNetworkType] = private.NodeLabelValueNetworkTypeFloatingIP

	immutableLabelMap := make(map[string]string)
	immutableLabelMap[private.LabelKeyFloatingIP] = private.LabelValueImmutable

	labels.SelectorFromSet(labels.Set(objectSelectorMap))
	p.objectSelector = labels.SelectorFromSet(labels.Set(objectSelectorMap))
	p.nodeSelector = labels.SelectorFromSet(labels.Set(nodeSelectorMap))
	p.immutableSeletor = labels.SelectorFromSet(labels.Set(immutableLabelMap))
	return nil
}

// updateConfigMap fetches the newest floatingips configmap and syncs in memory/db config,
// returns true if updated.
func (p *FloatingIPPlugin) updateConfigMap() (bool, error) {
	cm, err := p.Client.CoreV1().ConfigMaps(p.conf.ConfigMapNamespace).Get(p.conf.ConfigMapName, v1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get floatingip configmap %s_%s: %v", p.conf.ConfigMapName, p.conf.ConfigMapNamespace, err)

	}
	val, ok := cm.Data["floatingips"]
	if !ok {
		return false, fmt.Errorf("configmap %s_%s doesn't have a key floatingips", p.conf.ConfigMapName, p.conf.ConfigMapNamespace)
	}
	if val == p.lastFIPConf {
		glog.V(4).Infof("floatingip configmap unchanged")
		return false, nil
	}
	glog.Infof("updating floatingip config %s", val)
	var conf []*floatingip.FloatingIP
	if err := json.Unmarshal([]byte(val), &conf); err != nil {
		return false, fmt.Errorf("failed to unmarshal configmap %s_%s val %s to floatingip config", p.conf.ConfigMapName, p.conf.ConfigMapNamespace, val)
	}
	p.lastFIPConf = val
	if err := p.ipam.ConfigurePool(conf); err != nil {
		glog.Warningf("failed to configure pool: %v", err)
	}
	return true, nil
}

// Filter marks nodes which haven't been labeled as supporting floating IP or have no available ips as FailedNodes
// If the given pod doesn't want floating IP, none failedNodes returns
func (p *FloatingIPPlugin) Filter(pod *corev1.Pod, nodes []corev1.Node) ([]corev1.Node, schedulerapi.FailedNodesMap, error) {
	failedNodesMap := schedulerapi.FailedNodesMap{}
	if !p.wantedObject(&pod.ObjectMeta) {
		return nodes, failedNodesMap, nil
	}
	filteredNodes := []corev1.Node{}
	var (
		subnets []string
		err     error
	)
	key := keyInDB(pod)
	if subnets, err = p.ipam.QueryRoutableSubnetByKey(key); err != nil {
		return filteredNodes, failedNodesMap, fmt.Errorf("failed to query by key %s: %v", key, err)
	}
	if len(subnets) != 0 {
		glog.V(3).Infof("%s already have an allocated floating ip in subnets %v, it may have been deleted or evicted", key, subnets)
	} else {
		if subnets, err = p.ipam.QueryRoutableSubnetByKey(""); err != nil {
			return filteredNodes, failedNodesMap, fmt.Errorf("failed to query allocatable subnet: %v", err)
		}
	}
	subsetSet := sets.NewString(subnets...)
	for i := range nodes {
		nodeName := nodes[i].Name
		if !p.nodeSelector.Matches(labels.Set(nodes[i].GetLabels())) {
			failedNodesMap[nodeName] = "FloatingIPPlugin:UnlabelNode"
			continue
		}
		subnet, err := p.getNodeSubnet(&nodes[i])
		if err != nil {
			failedNodesMap[nodes[i].Name] = err.Error()
			continue
		}
		if subsetSet.Has(subnet.String()) {
			filteredNodes = append(filteredNodes, nodes[i])
		} else {
			failedNodesMap[nodeName] = "FloatingIPPlugin:NoFIPLeft"
		}
	}
	if bool(glog.V(4)) {
		nodeNames := make([]string, len(filteredNodes))
		for i := range filteredNodes {
			nodeNames[i] = filteredNodes[i].Name
		}
		glog.V(4).Infof("filtered nodes %v failed nodes %v", nodeNames, failedNodesMap)
	}
	return filteredNodes, failedNodesMap, nil
}

func (p *FloatingIPPlugin) Prioritize(pod *corev1.Pod, nodes []corev1.Node) (*schedulerapi.HostPriorityList, error) {
	list := &schedulerapi.HostPriorityList{}
	if !p.wantedObject(&pod.ObjectMeta) {
		return list, nil
	}
	//TODO
	return list, nil
}

// allocateIP Allocates a floating IP to the pod based on the winner node name
func (p *FloatingIPPlugin) allocateIP(key, nodeName string) (map[string]string, error) {
	var how string
	ipInfo, err := p.ipam.QueryFirst(key)
	if err != nil {
		return nil, fmt.Errorf("failed to query floating ip by key %s: %v", key, err)
	}
	if ipInfo != nil {
		how = "reused"
		glog.V(3).Infof("pod %s may have been deleted or evicted, it already have an allocated floating ip %s", key, ipInfo.IP.String())
	} else {
		subnet, err := p.queryNodeSubnet(nodeName)
		_, err = p.ipam.AllocateInSubnet(key, subnet)
		if err != nil {
			// return this error directly, invokers depend on the error type if it is ErrNoEnoughIP
			return nil, err
		}
		how = "allocated"
		ipInfo, err = p.ipam.QueryFirst(key)
		if err != nil {
			return nil, fmt.Errorf("failed to query floating ip by key %s: %v", key, err)
		}
		if ipInfo == nil {
			return nil, fmt.Errorf("nil floating ip for key %s: %v", key, err)
		}
	}
	data, err := json.Marshal(ipInfo)
	if err != nil {
		return nil, err
	}
	glog.Infof("%s floating ip %s for %s", how, ipInfo.IP.String(), key)
	bind := make(map[string]string)
	bind[private.AnnotationKeyIPInfo] = string(data)
	return bind, nil
}

func (p *FloatingIPPlugin) releasePodIP(key string) error {
	ipInfo, err := p.ipam.QueryFirst(key)
	if err != nil {
		return fmt.Errorf("failed to query floating ip of %s: %v", key, err)
	}
	if ipInfo == nil {
		return nil
	}
	if err := p.ipam.Release([]string{key}); err != nil {
		return fmt.Errorf("failed to release floating ip of %s: %v", key, err)
	}
	glog.Infof("released floating ip %s from %s", ipInfo.IP.String(), key)
	return nil
}

func (p *FloatingIPPlugin) AddPod(pod *corev1.Pod) error {
	return nil
}

func (p *FloatingIPPlugin) Bind(args *schedulerapi.ExtenderBindingArgs) error {
	key := fmtKey(args.PodName, args.PodNamespace)
	pod, err := p.PluginFactoryArgs.PodLister.Pods(args.PodNamespace).Get(args.PodName)
	if err != nil {
		return fmt.Errorf("failed to find pod %s: %v", key, err)
	}
	if !p.wantedObject(&pod.ObjectMeta) {
		// we will config extender resources which ensures pod which doesn't want floatingip won't be sent to plugin
		// see https://github.com/kubernetes/kubernetes/pull/60332
		return fmt.Errorf("pod which doesn't want floatingip have been sent to plugin")
	}
	bind, err := p.allocateIP(key, args.Node)
	if err != nil {
		return err
	}
	if bind == nil {
		return nil
	}
	ret := &unstructured.Unstructured{}
	ret.SetAnnotations(bind)
	patchData, err := json.Marshal(ret)
	if err != nil {
		glog.Error(err)
	}
	if err := wait.PollImmediate(time.Millisecond*500, 20*time.Second, func() (bool, error) {
		_, err := p.Client.CoreV1().Pods(args.PodNamespace).Patch(args.PodName, types.MergePatchType, patchData)
		if err != nil {
			glog.Warningf("failed to update pod %s: %v", key, err)
			return false, nil
		}
		glog.V(3).Infof("updated annotation %s=%s for pod %s", private.AnnotationKeyIPInfo, bind[private.AnnotationKeyIPInfo], key)
		// It's the extender's response to bind pods to nodes since it is a binder
		if err := p.Client.CoreV1().Pods(args.PodNamespace).Bind(&corev1.Binding{
			ObjectMeta: v1.ObjectMeta{Namespace: args.PodNamespace, Name: args.PodName, UID: args.PodUID},
			Target: corev1.ObjectReference{
				Kind: "Node",
				Name: args.Node,
			},
		}); err != nil {
			return false, err
		}
		glog.V(3).Infof("bind pod %s to %s", key, args.Node)
		return true, nil
	}); err != nil {
		// If fails to update, depending on resync to update
		return fmt.Errorf("failed to update pod %s: %v", key, err)
	}
	return nil
}

func (p *FloatingIPPlugin) UpdatePod(oldPod, newPod *corev1.Pod) error {
	if !p.wantedObject(&newPod.ObjectMeta) {
		return nil
	}
	if !evicted(oldPod) && evicted(newPod) {
		// Deployments will leave evicted pods, while TApps don't
		// If it's a evicted one, release its ip
		p.unreleased <- newPod
	}
	if err := p.syncPodIP(newPod); err != nil {
		glog.Warningf("failed to sync pod ip: %v", err)
	}
	return nil
}

func (p *FloatingIPPlugin) DeletePod(pod *corev1.Pod) error {
	if !p.wantedObject(&pod.ObjectMeta) {
		return nil
	}
	p.unreleased <- pod
	return nil
}

func (p *FloatingIPPlugin) unbind(pod *corev1.Pod) error {
	key := keyInDB(pod)
	if !p.immutableSeletor.Matches(labels.Set(pod.GetLabels())) {
		return p.releasePodIP(key)
	} else {
		tapps, err := p.TAppLister.GetPodTApps(pod)
		if err != nil {
			return p.releasePodIP(key)
		}
		tapp := tapps[0]
		for i, status := range tapp.Spec.Statuses {
			if !tappInstanceKilled(status) || i != pod.Labels[tappv1.TAppInstanceKey] {
				continue
			}
			// build the key namespace_tappname-id
			return p.releasePodIP(key)
		}
	}
	if pod.Annotations != nil {
		glog.V(3).Infof("reserved %s for pod %s", pod.Annotations[private.AnnotationKeyIPInfo], key)
	}
	return nil
}

func (p *FloatingIPPlugin) releaseAppIPs(keyPrefix string) error {
	ipMap, err := p.ipam.QueryByPrefix(keyPrefix)
	if err != nil {
		return fmt.Errorf("failed to query allocated floating ips for app %s: %v", keyPrefix, err)
	}
	if err := p.ipam.ReleaseByPrefix(keyPrefix); err != nil {
		return fmt.Errorf("failed to release floating ip for app %s: %v", keyPrefix, err)
	} else {
		ips := []string{}
		for ip := range ipMap {
			ips = append(ips, ip)
		}
		glog.Infof("released all floating ip %v for %s", ips, keyPrefix)
	}
	return nil
}

func (p *FloatingIPPlugin) wantedObject(o *v1.ObjectMeta) bool {
	labelMap := o.GetLabels()
	if labelMap == nil {
		return false
	}
	if !p.objectSelector.Matches(labels.Set(labelMap)) {
		return false
	}
	return true
}

func getNodeIP(node *corev1.Node) net.IP {
	for i := range node.Status.Addresses {
		if node.Status.Addresses[i].Type == corev1.NodeInternalIP {
			return net.ParseIP(node.Status.Addresses[i].Address)
		}
	}
	return nil
}

func tappInstanceKilled(status tappv1.InstanceStatus) bool {
	// TODO v1 INSTANCE_KILLED = "killed" but in types INSTANCE_KILLED = "Killed"
	return strings.ToLower(string(status)) == strings.ToLower(string(tappv1.INSTANCE_KILLED))
}

func (p *FloatingIPPlugin) loop(stop chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case pod := <-p.unreleased:
			go func() {
				if err := p.unbind(pod); err != nil {
					glog.Warning(err)
					// backoff time if required
					time.Sleep(300 * time.Millisecond)
					p.unreleased <- pod
				}
			}()
		}
	}
}

func evicted(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed && pod.Status.Reason == "Evicted"
}

func (p *FloatingIPPlugin) getNodeSubnet(node *corev1.Node) (*net.IPNet, error) {
	p.nodeSubnetLock.Lock()
	defer p.nodeSubnetLock.Unlock()
	if subnet, ok := p.nodeSubnet[node.Name]; !ok {
		nodeIP := getNodeIP(node)
		if nodeIP == nil {
			return nil, errors.New("FloatingIPPlugin:UnknowNode")
		}
		if ipNet := p.ipam.RoutableSubnet(nodeIP); ipNet != nil {
			return ipNet, nil
		} else {
			return nil, errors.New("FloatingIPPlugin:NoFIPConfigNode")
		}
	} else {
		return subnet, nil
	}
}

func (p *FloatingIPPlugin) queryNodeSubnet(nodeName string) (*net.IPNet, error) {
	var (
		node *corev1.Node
	)
	p.nodeSubnetLock.Lock()
	defer p.nodeSubnetLock.Unlock()
	if subnet, ok := p.nodeSubnet[nodeName]; !ok {
		if err := wait.Poll(time.Millisecond*100, time.Minute, func() (done bool, err error) {
			node, err = p.Client.CoreV1().Nodes().Get(nodeName, v1.GetOptions{})
			if !metaErrs.IsServerTimeout(err) {
				return true, err
			}
			return false, nil
		}); err != nil {
			return nil, err
		}
		nodeIP := getNodeIP(node)
		if nodeIP == nil {
			return nil, errors.New("FloatingIPPlugin:UnknowNode")
		}
		if ipNet := p.ipam.RoutableSubnet(nodeIP); ipNet != nil {
			glog.V(4).Infof("node %s %s %s", nodeName, nodeIP.String(), ipNet.String())
			return ipNet, nil
		} else {
			return nil, errors.New("FloatingIPPlugin:NoFIPConfigNode")
		}
	} else {
		return subnet, nil
	}
}
