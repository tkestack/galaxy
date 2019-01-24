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

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/schedulerapi"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	"github.com/golang/glog"
	appv1 "k8s.io/api/apps/v1"
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
	deletedAndScaledDownSSPod      = "deletedAndScaledDownSSPod"
	deletedAndScaledDownDpPod      = "deletedAndScaledDownDpPod"
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
	getDeployment                func(name, namespace string) (*appv1.Deployment, error)
	dpLock                       sync.Mutex
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
	plugin.getDeployment = func(name, namespace string) (*appv1.Deployment, error) {
		return plugin.DeploymentLister.Deployments(namespace).Get(name)
	}
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
	dp := p.podBelongToDeployment(pod)
	if dp != "" {
		key = keyForDeploymentPod(pod, dp)
	}
	policy := parseReleasePolicy(pod.GetLabels())
	var err error
	var deployment *appv1.Deployment
	if dp != "" {
		deployment, err = p.getDeployment(dp, pod.Namespace)
		if err != nil {
			return filteredNodes, failedNodesMap, err
		}
	}
	subnets, reserve, err := getAvailableSubnet(p.ipam, key, deployment)
	if err != nil {
		return filteredNodes, failedNodesMap, fmt.Errorf("[%s] %v", p.ipam.Name(), err)
	}
	subnetSet := sets.NewString(subnets...)
	if p.enabledSecondIP(pod) {
		secondSubnets, reserve2, err := getAvailableSubnet(p.secondIPAM, key, deployment)
		if err != nil {
			return filteredNodes, failedNodesMap, fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
		}
		subnetSet = subnetSet.Intersection(sets.NewString(secondSubnets...))
		reserve = reserve || reserve2
	}
	if reserve && subnetSet.Len() > 0 {
		reserveSubnet := subnetSet.List()[0]
		subnetSet = sets.NewString(reserveSubnet)
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
		if subnetSet.Has(subnet.String()) {
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
	if reserve && subnetSet.Len() > 0 {
		reserveSubnet := subnetSet.List()[0]
		prefixKey := deploymentPrefix(dp, pod.Namespace)
		attr := getAttr(pod)
		if err := p.ipam.AllocateInSubnetWithKey(prefixKey, key, reserveSubnet, policy, attr); err != nil {
			return filteredNodes, failedNodesMap, err
		}
		if p.enabledSecondIP(pod) {
			if err := p.secondIPAM.AllocateInSubnetWithKey(prefixKey, key, reserveSubnet, policy, attr); err != nil {
				return filteredNodes, failedNodesMap, err
			}
		}
	}

	return filteredNodes, failedNodesMap, nil
}

func getAvailableSubnet(ipam floatingip.IPAM, key string, dp *appv1.Deployment) (subnets []string, reserve bool, err error) {
	if subnets, err = ipam.QueryRoutableSubnetByKey(key); err != nil {
		err = fmt.Errorf("failed to query by key %s: %v", key, err)
		return
	}
	if len(subnets) != 0 {
		glog.V(3).Infof("[%s] %s already have an allocated floating ip in subnets %v, it may have been deleted or evicted", ipam.Name(), key, subnets)
	} else {
		if dp != nil && parseReleasePolicy(dp.Spec.Template.GetLabels()) != database.PodDelete { // get label from pod?
			prefixKey := deploymentPrefix(dp.Name, dp.Namespace)
			var ips []database.FloatingIP
			ips, err = ipam.ByPrefix(prefixKey)
			if err != nil {
				err = fmt.Errorf("failed query prefix %s: %s", prefixKey, err)
				return
			}
			replicas := int(*dp.Spec.Replicas)
			usedCount := 0
			unusedSubnetSet := sets.NewString()
			for _, ip := range ips {
				if ip.Key != prefixKey {
					usedCount++
				} else {
					unusedSubnetSet.Insert(ip.Subnet)
				}
			}
			if usedCount >= replicas {
				glog.Warningf("deployment %s has allocate %d ips, but we only want %d, wait to release", dp.Name, usedCount, replicas)
				return nil, false, nil
			}
			if unusedSubnetSet.Len() > 0 {
				return unusedSubnetSet.List(), true, nil
			}
		}
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

// podBelongToDeployment return deployment name if pod is generated by deployment
func (p *FloatingIPPlugin) podBelongToDeployment(pod *corev1.Pod) string {
	if len(pod.OwnerReferences) == 1 && pod.OwnerReferences[0].Kind == "ReplicaSet" {
		// assume pod belong to deployment for convenient, name format: dpname-rsid-podid
		parts := strings.Split(pod.Name, "-")
		if len(parts) < 3 {
			return ""
		}
		return strings.Join(parts[:len(parts)-2], "-")
	}
	return ""
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
	ipInfo, err := ipam.First(key)
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
	glog.Infof("[%s] released floating ip %s from %s because of %s", ipam.Name(), ipInfo.IPInfo.IP.String(), key, reason)
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
	if dp := p.podBelongToDeployment(pod); dp != "" {
		key = keyForDeploymentPod(pod, dp)
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
		// Deployments will leave evicted pods
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
	glog.Warningf("pod %s deleted, handle", pod.Name)
	p.unreleased <- pod
	return nil
}

func (p *FloatingIPPlugin) unbind(pod *corev1.Pod) error {
	glog.Infof("handle unbind pod %s", pod.Name)
	key := keyInDB(pod)
	dp := p.podBelongToDeployment(pod)
	if dp != "" {
		key = keyForDeploymentPod(pod, dp)
		p.dpLock.Lock()
		defer p.dpLock.Unlock()
	}

	switch pod.GetLabels()[private.LabelKeyFloatingIP] {
	case private.LabelValueNeverRelease:
		if dp := p.podBelongToDeployment(pod); dp != "" {
			key := keyForDeploymentPod(pod, dp)
			keyPrefix := deploymentPrefix(dp, pod.Namespace)
			err := p.ipam.UpdateKey(key, keyPrefix)
			if err != nil {
				glog.Errorf("failed put back pod(%s) ip to deployment %s: %v", pod.Name, dp, err)
			}
			if p.enabledSecondIP(pod) {
				err = p.secondIPAM.UpdateKey(key, keyPrefix)
				if err != nil {
					glog.Errorf("failed put back pod(%s) ip to deployment %s: %v", pod.Name, dp, err)
				}
			}
		}
		glog.V(3).Infof("reserved %s for pod %s (never release)", pod.Annotations[private.AnnotationKeyIPInfo], key)
		return nil
	case private.LabelValueImmutable:
		break
	default:
		return p.releaseIP(key, deletedAndIPMutablePod, pod)
	}
	var statefulSet []*appv1.StatefulSet
	var err error
	// for test
	if p.StatefulSetLister != nil {
		statefulSet, err = p.StatefulSetLister.GetPodStatefulSets(pod)
	} else {
		err = fmt.Errorf("StatefulSetLister not support")
	}
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
	} else if dp != "" {
		// release if scale down, or reserve if should be
		deploy, err := p.getDeployment(dp, pod.Namespace)
		replicas := 0
		if err != nil {
			if !metaErrs.IsNotFound(err) {
				return err
			}
		} else {
			replicas = int(*deploy.Spec.Replicas)
		}
		prefixKey := deploymentPrefix(dp, pod.Namespace)
		fips, err := p.ipam.ByPrefix(prefixKey)
		if err != nil {
			return err
		}
		if len(fips) > replicas {
			err = p.ipam.Release([]string{key})
			if err != nil {
				return err
			}
		} else {
			err = p.ipam.UpdateKey(key, prefixKey)
			if err != nil {
				glog.Errorf("failed reserve ip from pod %s to deploy %s: %v", pod.Name, deploy.Name, err)
				return err
			}
		}
		if p.enabledSecondIP(pod) {
			fips, err := p.secondIPAM.ByPrefix(prefixKey)
			if err != nil {
				return err
			}
			if len(fips) > replicas {
				err = p.secondIPAM.Release([]string{key})
				if err != nil {
					return err
				}
			} else {
				err = p.secondIPAM.UpdateKey(key, prefixKey)
				if err != nil {
					glog.Errorf("failed reserve ip from pod %s to deploy %s: %v", pod.Name, deploy.Name, err)
					return err
				}
			}
		}
		return nil
	} else {
		return p.releaseIP(key, deletedAndParentAppNotExistPod, pod)
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
	t := time.Now().Unix()
	obj := struct {
		Time int64
	}{Time: t}
	attr, err := json.Marshal(obj)
	if err != nil {
		glog.Warningf("failed to marshal attr %+v: %v", obj, err)
	}
	return string(attr)
}
