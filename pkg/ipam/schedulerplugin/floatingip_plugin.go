package schedulerplugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	deletedAndIPMutablePod         = "deletedAndIPMutablePod"
	deletedAndParentAppNotExistPod = "deletedAndParentAppNotExistPod"
	deletedAndKilledTappPod        = "deletedAndKilledTappPod"
	deletedAndScaledDownSSPod      = "deletedAndScaledDownSSPod"
	evictedPod                     = "evictedPod"
	deletedAndLabelMissMatchPod    = "deletedAndLabelMissMatchPod"
)

type Conf struct {
	FloatingIPs         []*floatingip.FloatingIP `json:"floatingips,omitempty"`
	DBConfig            *database.DBConfig       `json:"database"`
	ResyncInterval      uint                     `json:"resyncInterval"`
	ConfigMapName       string                   `json:"configMapName"`
	ConfigMapNamespace  string                   `json:"configMapNamespace"`
	DisableLabelNode    bool                     `json:"disableLabelNode"`
	FloatingIPKey       string                   `json:"floatingipKey"`       // configmap floatingip data key
	SecondFloatingIPKey string                   `json:"secondFloatingipKey"` // configmap second floatingip data key
}

// FloatingIPPlugin Allocates Floating IP for deployments
type FloatingIPPlugin struct {
	objectSelector, nodeSelector labels.Selector
	ipam, secondIPAM             floatingip.IPAM
	// node name to subnet cache
	nodeSubnet     map[string]*net.IPNet
	nodeSubnetLock sync.Mutex
	sync.Mutex
	*PluginFactoryArgs
	lastIPConf, lastSecondIPConf string
	conf                         *Conf
	unreleased                   chan *corev1.Pod
	hasSecondIPConf              atomic.Value
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
	if conf.FloatingIPKey == "" {
		conf.FloatingIPKey = "floatingips"
	}
	if conf.SecondFloatingIPKey == "" {
		conf.SecondFloatingIPKey = "second_floatingips"
	}
	glog.Infof("floating ip config: %v", conf)
	db := database.NewDBRecorder(conf.DBConfig)
	if err := db.Run(); err != nil {
		return nil, err
	}
	plugin := &FloatingIPPlugin{
		ipam:              floatingip.NewIPAM(db),
		secondIPAM:        floatingip.NewIPAMWithTableName(db, "second_fips"),
		nodeSubnet:        make(map[string]*net.IPNet),
		PluginFactoryArgs: args,
		conf:              &conf,
		unreleased:        make(chan *corev1.Pod, 10),
	}
	plugin.hasSecondIPConf.Store(false)
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
			p.labelNodes()
		}, time.Minute, stop)
	}
	go wait.Until(func() {
		if err := p.resyncPod(p.ipam); err != nil {
			glog.Warningf("[%s] %v", p.ipam.Name(), err)
		}
		if p.hasSecondIPConf.Load().(bool) {
			if err := p.resyncPod(p.secondIPAM); err != nil {
				glog.Warningf("[%s] %v", p.secondIPAM.Name(), err)
			}
		}
		p.syncPodIPsIntoDB()
		p.syncTAppRequestResource()
	}, time.Duration(p.conf.ResyncInterval)*time.Minute, stop)
	go p.loop(stop)
}

func (p *FloatingIPPlugin) initSelector() error {
	objectSelectorMap := make(map[string]string)
	objectSelectorMap[private.LabelKeyNetworkType] = private.LabelValueNetworkTypeFloatingIP
	nodeSelectorMap := make(map[string]string)
	nodeSelectorMap[private.LabelKeyNetworkType] = private.NodeLabelValueNetworkTypeFloatingIP

	labels.SelectorFromSet(labels.Set(objectSelectorMap))
	p.objectSelector = labels.SelectorFromSet(labels.Set(objectSelectorMap))
	p.nodeSelector = labels.SelectorFromSet(labels.Set(nodeSelectorMap))
	return nil
}

// updateConfigMap fetches the newest floatingips configmap and syncs in memory/db config,
// returns true if successfully gets floatingip config.
func (p *FloatingIPPlugin) updateConfigMap() (bool, error) {
	cm, err := p.Client.CoreV1().ConfigMaps(p.conf.ConfigMapNamespace).Get(p.conf.ConfigMapName, v1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get floatingip configmap %s_%s: %v", p.conf.ConfigMapName, p.conf.ConfigMapNamespace, err)

	}
	val, ok := cm.Data[p.conf.FloatingIPKey]
	if !ok {
		return false, fmt.Errorf("configmap %s_%s doesn't have a key floatingips", p.conf.ConfigMapName, p.conf.ConfigMapNamespace)
	}
	if err := ensureIPAMConf(p.ipam, &p.lastIPConf, val); err != nil {
		return false, fmt.Errorf("[%s] %v", p.ipam.Name(), err)
	}
	secondVal, ok := cm.Data[p.conf.SecondFloatingIPKey]
	if err = ensureIPAMConf(p.secondIPAM, &p.lastSecondIPConf, secondVal); err != nil {
		return false, fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
	}
	p.hasSecondIPConf.Store(p.lastSecondIPConf != "")
	return true, nil
}

func ensureIPAMConf(ipam floatingip.IPAM, lastConf *string, newConf string) error {
	if newConf == *lastConf {
		glog.V(4).Infof("[%s] floatingip configmap unchanged", ipam.Name())
		return nil
	}
	var conf []*floatingip.FloatingIP
	if err := json.Unmarshal([]byte(newConf), &conf); err != nil {
		return fmt.Errorf("failed to unmarshal configmap val %s to floatingip config", newConf)
	}
	glog.Infof("[%s] updated floatingip conf from (%s) to (%s)", ipam.Name(), *lastConf, newConf)
	*lastConf = newConf
	if err := ipam.ConfigurePool(conf); err != nil {
		return fmt.Errorf("failed to configure pool: %v", err)
	}
	return nil
}

// Filter marks nodes which haven't been labeled as supporting floating IP or have no available ips as FailedNodes
// If the given pod doesn't want floating IP, none failedNodes returns
func (p *FloatingIPPlugin) Filter(pod *corev1.Pod, nodes []corev1.Node) ([]corev1.Node, schedulerapi.FailedNodesMap, error) {
	failedNodesMap := schedulerapi.FailedNodesMap{}
	if !p.wantedObject(&pod.ObjectMeta) {
		return nodes, failedNodesMap, nil
	}
	filteredNodes := []corev1.Node{}
	key := keyInDB(pod)
	subnets, err := getAvailableSubnet(p.ipam, key)
	if err != nil {
		return filteredNodes, failedNodesMap, fmt.Errorf("[%s] %v", p.ipam.Name(), err)
	}
	subsetSet := sets.NewString(subnets...)
	if p.enabledSecondIP(pod) {
		secondSubnets, err := getAvailableSubnet(p.secondIPAM, key)
		if err != nil {
			return filteredNodes, failedNodesMap, fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
		}
		subsetSet = subsetSet.Intersection(sets.NewString(secondSubnets...))
	}
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

func getAvailableSubnet(ipam floatingip.IPAM, key string) (subnets []string, err error) {
	if subnets, err = ipam.QueryRoutableSubnetByKey(key); err != nil {
		err = fmt.Errorf("failed to query by key %s: %v", key, err)
		return
	}
	if len(subnets) != 0 {
		glog.V(3).Infof("[%s] %s already have an allocated floating ip in subnets %v, it may have been deleted or evicted", ipam.Name(), key, subnets)
	} else {
		if subnets, err = ipam.QueryRoutableSubnetByKey(""); err != nil {
			err = fmt.Errorf("failed to query allocatable subnet: %v", err)
			return
		}
	}
	return
}

func (p *FloatingIPPlugin) Prioritize(pod *corev1.Pod, nodes []corev1.Node) (*schedulerapi.HostPriorityList, error) {
	list := &schedulerapi.HostPriorityList{}
	if !p.wantedObject(&pod.ObjectMeta) {
		return list, nil
	}
	//TODO
	return list, nil
}

func (p *FloatingIPPlugin) allocateIP(ipam floatingip.IPAM, key, nodeName string, pod *corev1.Pod) (*floatingip.IPInfo, error) {
	var how string
	ipInfo, err := ipam.First(key)
	if err != nil {
		return nil, fmt.Errorf("failed to query floating ip by key %s: %v", key, err)
	}
	policy := parseReleasePolicy(pod.GetLabels())
	attr := getAttr(pod)
	if ipInfo != nil {
		how = "reused"
		glog.Infof("pod %s have an allocated floating ip %s, updating policy to %v attr %s", key, ipInfo.IPInfo.IP.String(), policy, attr)
		if err := ipam.UpdatePolicy(key, ipInfo.IPInfo.IP.IP, policy, attr); err != nil {
			return nil, fmt.Errorf("failed to update floating ip release policy: %v", err)
		}
	} else {
		subnet, err := p.queryNodeSubnet(nodeName)
		_, err = ipam.AllocateInSubnet(key, subnet, policy, attr)
		if err != nil {
			// return this error directly, invokers depend on the error type if it is ErrNoEnoughIP
			return nil, err
		}
		how = "allocated"
		ipInfo, err = ipam.First(key)
		if err != nil {
			return nil, fmt.Errorf("failed to query floating ip by key %s: %v", key, err)
		}
		if ipInfo == nil {
			return nil, fmt.Errorf("nil floating ip for key %s: %v", key, err)
		}
	}
	glog.Infof("[%s] %s floating ip %s, policy %v, attr %s for %s", ipam.Name(), how, ipInfo.IPInfo.IP.String(), policy, attr, key)
	return &ipInfo.IPInfo, nil
}

func (p *FloatingIPPlugin) releaseIP(key string, reason string, pod *corev1.Pod) error {
	if err := releaseIP(p.ipam, key, reason); err != nil {
		return fmt.Errorf("[%s] %v", p.ipam.Name(), err)
	}
	// skip release second ip if not enabled
	if !(p.hasSecondIPConf.Load().(bool)) {
		return nil
	}
	if pod != nil && !wantSecondIP(pod) {
		return nil
	}
	if err := releaseIP(p.secondIPAM, key, reason); err != nil {
		return fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
	}
	return nil
}

func releaseIP(ipam floatingip.IPAM, key string, reason string) error {
	ipInfo, err := ipam.QueryFirst(key)
	if err != nil {
		return fmt.Errorf("failed to query floating ip of %s: %v", key, err)
	}
	if ipInfo == nil {
		glog.Infof("[%s] release floating ip from %s because of %s, but already been released", ipam.Name(), key, reason)
		return nil
	}
	if err := ipam.Release([]string{key}); err != nil {
		return fmt.Errorf("failed to release floating ip of %s because of %s: %v", key, reason, err)
	}
	glog.Infof("[%s] released floating ip %s from %s because of %s", ipam.Name(), ipInfo.IP.String(), key, reason)
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
	ipInfo, err := p.allocateIP(p.ipam, key, args.Node, pod)
	if err != nil {
		return err
	}
	data, err := json.Marshal(ipInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal ipinfo %v: %v", ipInfo, err)
	}
	bindAnnotation := make(map[string]string)
	bindAnnotation[private.AnnotationKeyIPInfo] = string(data)
	if p.enabledSecondIP(pod) {
		secondIPInfo, err := p.allocateIP(p.secondIPAM, key, args.Node, pod)
		// TODO release ip if it's been allocated in this goroutine?
		if err != nil {
			return fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
		}
		data, err := json.Marshal(secondIPInfo)
		if err != nil {
			return fmt.Errorf("failed to marshal ipinfo %v: %v", secondIPInfo, err)
		}
		bindAnnotation[private.AnnotationKeySecondIPInfo] = string(data)
	}
	if err := wait.PollImmediate(time.Millisecond*500, 20*time.Second, func() (bool, error) {
		// It's the extender's response to bind pods to nodes since it is a binder
		if err := p.Client.CoreV1().Pods(args.PodNamespace).Bind(&corev1.Binding{
			ObjectMeta: v1.ObjectMeta{Namespace: args.PodNamespace, Name: args.PodName, UID: args.PodUID, Annotations: bindAnnotation},
			Target: corev1.ObjectReference{
				Kind: "Node",
				Name: args.Node,
			},
		}); err != nil {
			glog.Warningf("failed to bind pod %s: %v", key, err)
			return false, err
		}
		glog.V(3).Infof("bind pod %s to %s with ip %v", key, args.Node, bindAnnotation[private.AnnotationKeyIPInfo])
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

	switch pod.GetLabels()[private.LabelKeyFloatingIP] {
	case private.LabelValueNeverRelease:
		glog.V(3).Infof("reserved %s for pod %s (never release)", pod.Annotations[private.AnnotationKeyIPInfo], key)
		return nil
	case private.LabelValueImmutable:
		break
	default:
		return p.releaseIP(key, deletedAndIPMutablePod, pod)
	}
	statefulSet, err := p.StatefulSetLister.GetPodStatefulSets(pod)
	if err == nil {
		// it's a statefulset pod
		if len(statefulSet) > 1 {
			glog.Warningf("multiple ss found for pod %s", key)
		}
		ss := statefulSet[0]
		index, err := parsePodIndex(pod.Name)
		if err != nil {
			return fmt.Errorf("invalid pod name %s of ss %s: %v", key, statefulsetName(ss), err)
		}
		if ss.Spec.Replicas != nil && *ss.Spec.Replicas < int32(index)+1 {
			return p.releaseIP(key, deletedAndScaledDownSSPod, pod)
		}
	} else {
		tapps, err := p.TAppLister.GetPodTApps(pod)
		if err != nil {
			return p.releaseIP(key, deletedAndParentAppNotExistPod, pod)
		}
		if len(tapps) > 1 {
			glog.Warningf("multiple tapp found for pod %s", key)
		}
		tapp := tapps[0]
		for i, status := range tapp.Spec.Statuses {
			if !tappInstanceKilled(status) || i != pod.Labels[tappv1.TAppInstanceKey] {
				continue
			}
			// build the key namespace_tappname-id
			return p.releaseIP(key, deletedAndKilledTappPod, pod)
		}
	}
	if pod.Annotations != nil {
		glog.V(3).Infof("reserved %s for pod %s", pod.Annotations[private.AnnotationKeyIPInfo], key)
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
			glog.V(4).Infof("node %s %s %s", node.Name, nodeIP.String(), ipNet.String())
			p.nodeSubnet[node.Name] = ipNet
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
			p.nodeSubnet[nodeName] = ipNet
			return ipNet, nil
		} else {
			return nil, errors.New("FloatingIPPlugin:NoFIPConfigNode")
		}
	} else {
		return subnet, nil
	}
}

func (p *FloatingIPPlugin) enabledSecondIP(pod *corev1.Pod) bool {
	return p.hasSecondIPConf.Load().(bool) && wantSecondIP(pod)
}

func wantSecondIP(pod *corev1.Pod) bool {
	labelMap := pod.GetLabels()
	if labelMap == nil {
		return false
	}
	return labelMap[private.LabelKeyEnableSecondIP] == private.LabelValueEnabled
}

func parseReleasePolicy(labels map[string]string) database.ReleasePolicy {
	switch labels[private.LabelKeyFloatingIP] {
	case private.LabelValueNeverRelease:
		return database.Never
	case private.LabelValueImmutable:
		return database.AppDeleteOrScaleDown
	default:
		return database.PodDelete
	}
}

func getAttr(pod *corev1.Pod) string {
	var submitter string
	for i := range pod.Spec.Containers {
		for _, env := range pod.Spec.Containers[i].Env {
			if env.Name == private.EnvKeySubmitter {
				submitter = env.Value
				break
			}
		}
		if submitter != "" {
			break
		}
	}
	var appID string
	if pod.Annotations != nil {
		appID = pod.Annotations[private.AnnotationKeyAppID]
	}
	t := time.Now().Unix()
	obj := struct {
		Submitter string
		Time      int64
		AppID     string
	}{
		Submitter: submitter,
		Time:      t,
		AppID:     appID,
	}
	attr, err := json.Marshal(obj)
	if err != nil {
		glog.Warningf("failed to marshal attr %+v: %v", obj, err)
	}
	return string(attr)
}
