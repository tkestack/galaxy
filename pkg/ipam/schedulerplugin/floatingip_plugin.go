/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package schedulerplugin

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/galaxy/private"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	"tkestack.io/galaxy/pkg/utils/keylock"
)

// FloatingIPPlugin Allocates Floating IP for deployments
type FloatingIPPlugin struct {
	ipam, secondIPAM floatingip.IPAM
	// node name to subnet cache
	nodeSubnet     map[string]*net.IPNet
	nodeSubnetLock sync.Mutex
	resyncLock     sync.RWMutex
	*PluginFactoryArgs
	lastIPConf, lastSecondIPConf string
	conf                         *Conf
	unreleased                   chan *releaseEvent
	hasSecondIPConf              atomic.Value
	cloudProvider                cloudprovider.CloudProvider
	// protect unbind immutable deployment pod
	dpLockPool *keylock.Keylock
}

// NewFloatingIPPlugin creates FloatingIPPlugin
func NewFloatingIPPlugin(conf Conf, args *PluginFactoryArgs) (*FloatingIPPlugin, error) {
	conf.validate()
	glog.Infof("floating ip config: %v", conf)
	plugin := &FloatingIPPlugin{
		nodeSubnet:        make(map[string]*net.IPNet),
		PluginFactoryArgs: args,
		conf:              &conf,
		unreleased:        make(chan *releaseEvent, 1000),
		dpLockPool:        keylock.NewKeylock(),
	}
	if conf.StorageDriver == "k8s-crd" {
		plugin.ipam = floatingip.NewCrdIPAM(args.CrdClient, floatingip.InternalIp)
		plugin.secondIPAM = floatingip.NewCrdIPAM(args.CrdClient, floatingip.ExternalIp)
	} else {
		return nil, fmt.Errorf("unknown storage driver %s", conf.StorageDriver)
	}
	plugin.hasSecondIPConf.Store(false)
	if conf.CloudProviderGRPCAddr != "" {
		plugin.cloudProvider = cloudprovider.NewGRPCCloudProvider(conf.CloudProviderGRPCAddr)
	}
	return plugin, nil
}

// Init retrieves floatingips from json config or config map and calls ipam to update
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
	wait.PollInfinite(time.Second, func() (done bool, err error) {
		glog.Infof("waiting store ready")
		return p.storeReady(), nil
	})
	glog.Infof("store is ready, plugin init done")
	return nil
}

// Run starts resyncing pod routine
func (p *FloatingIPPlugin) Run(stop chan struct{}) {
	if len(p.conf.FloatingIPs) == 0 {
		go wait.Until(func() {
			if _, err := p.updateConfigMap(); err != nil {
				glog.Warning(err)
			}
		}, time.Minute, stop)
	}
	firstTime := true
	go wait.Until(func() {
		if firstTime {
			glog.Infof("start resyncing for the first time")
			defer glog.Infof("resyncing complete for the first time")
			firstTime = false
		}
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
	for i := 0; i < 5; i++ {
		go p.loop(stop)
	}
}

// updateConfigMap fetches the newest floatingips configmap and syncs in memory/db config,
// returns true if successfully gets floatingip config.
func (p *FloatingIPPlugin) updateConfigMap() (bool, error) {
	cm, err := p.Client.CoreV1().ConfigMaps(p.conf.ConfigMapNamespace).Get(p.conf.ConfigMapName, v1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get floatingip configmap %s_%s: %v", p.conf.ConfigMapName,
			p.conf.ConfigMapNamespace, err)
	}
	val, ok := cm.Data[p.conf.FloatingIPKey]
	if !ok {
		return false, fmt.Errorf("configmap %s_%s doesn't have a key floatingips", p.conf.ConfigMapName,
			p.conf.ConfigMapNamespace)
	}
	if err := ensureIPAMConf(p.ipam, &p.lastIPConf, val); err != nil {
		return false, fmt.Errorf("[%s] %v", p.ipam.Name(), err)
	}
	secondVal, ok := cm.Data[p.conf.SecondFloatingIPKey]
	if !ok {
		return true, nil
	}
	if err = ensureIPAMConf(p.secondIPAM, &p.lastSecondIPConf, secondVal); err != nil {
		return false, fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
	}
	p.hasSecondIPConf.Store(p.lastSecondIPConf != "")
	return true, nil
}

// Filter marks nodes which have no available ips as FailedNodes
// If the given pod doesn't want floating IP, none failedNodes returns
func (p *FloatingIPPlugin) Filter(pod *corev1.Pod, nodes []corev1.Node) ([]corev1.Node, schedulerapi.FailedNodesMap,
	error) {
	p.resyncLock.RLock()
	defer p.resyncLock.RUnlock()
	failedNodesMap := schedulerapi.FailedNodesMap{}
	if !p.hasResourceName(&pod.Spec) {
		return nodes, failedNodesMap, nil
	}
	filteredNodes := []corev1.Node{}
	subnetSet, err := p.getSubnet(pod)
	if err != nil {
		return filteredNodes, failedNodesMap, err
	}
	for i := range nodes {
		nodeName := nodes[i].Name
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
	if glog.V(5) {
		nodeNames := make([]string, len(filteredNodes))
		for i := range filteredNodes {
			nodeNames[i] = filteredNodes[i].Name
		}
		glog.V(5).Infof("filtered nodes %v failed nodes %v", nodeNames, failedNodesMap)
	}
	return filteredNodes, failedNodesMap, nil
}

// #lizard forgives
func (p *FloatingIPPlugin) getSubnet(pod *corev1.Pod) (sets.String, error) {
	keyObj := util.FormatKey(pod)
	// first check if exists an already allocated ip for this pod
	subnets, err := p.ipam.QueryRoutableSubnetByKey(keyObj.KeyInDB)
	if err != nil {
		return nil, fmt.Errorf("failed to query by key %s: %v", keyObj.KeyInDB, err)
	}
	if len(subnets) > 0 {
		// assure second IPAM gets the same subnets
		glog.V(3).Infof("%s already have an allocated ip in subnets %v", keyObj.KeyInDB, subnets)
		return sets.NewString(subnets...), nil
	}
	policy := parseReleasePolicy(&pod.ObjectMeta)
	var replicas int
	var isPoolSizeDefined bool
	if keyObj.Deployment() {
		replicas, isPoolSizeDefined, err = p.getDpReplicas(keyObj)
		if err != nil {
			return nil, err
		}
		// Lock to make checking available subnets and allocating reserved ip atomic
		lockIndex := p.dpLockPool.GetLockIndex([]byte(keyObj.PoolPrefix()))
		p.dpLockPool.RawLock(lockIndex)
		defer p.dpLockPool.RawUnlock(lockIndex)
	}
	subnets, reserve, err := getAvailableSubnet(p.ipam, keyObj, policy, replicas, isPoolSizeDefined)
	if err != nil {
		return nil, fmt.Errorf("[%s] %v", p.ipam.Name(), err)
	}
	subnetSet := sets.NewString(subnets...)
	if p.enabledSecondIP(pod) {
		secondSubnets, reserve2, err := getAvailableSubnet(p.secondIPAM, keyObj, policy, replicas, isPoolSizeDefined)
		if err != nil {
			return nil, fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
		}
		subnetSet = subnetSet.Intersection(sets.NewString(secondSubnets...))
		reserve = reserve || reserve2
	}
	if (reserve || isPoolSizeDefined) && subnetSet.Len() > 0 {
		// Since bind is in a different goroutine than filter in scheduler, we can't ensure this pod got binded
		// before the next one got filtered to ensure max size of allocated ips.
		// So we'd better do the allocate in filter for reserve situation.
		reserveSubnet := subnetSet.List()[0]
		subnetSet = sets.NewString(reserveSubnet)
		if err := p.allocateDuringFilter(keyObj, p.enabledSecondIP(pod), reserve, isPoolSizeDefined, reserveSubnet,
			policy); err != nil {
			return nil, err
		}
	}
	return subnetSet, nil
}

func (p *FloatingIPPlugin) allocateDuringFilter(keyObj *util.KeyObj, enabledSecondIP, reserve, isPoolSizeDefined bool,
	reserveSubnet string, policy constant.ReleasePolicy) error {
	// we can't get nodename during filter, update attr on bind
	attr := getAttr("")
	if reserve {
		if err := allocateInSubnetWithKey(p.ipam, keyObj.PoolPrefix(), keyObj.KeyInDB, reserveSubnet, policy, attr,
			"filter"); err != nil {
			return err
		}
		if enabledSecondIP {
			if err := allocateInSubnetWithKey(p.secondIPAM, keyObj.PoolPrefix(), keyObj.KeyInDB, reserveSubnet, policy,
				attr, "filter"); err != nil {
				return err
			}
		}
	} else if isPoolSizeDefined {
		// if pool size defined and we got no reserved IP, we need to allocate IP from empty key
		_, ipNet, err := net.ParseCIDR(reserveSubnet)
		if err != nil {
			return err
		}
		if err := allocateInSubnet(p.ipam, keyObj.KeyInDB, ipNet, policy, attr, "filter"); err != nil {
			return err
		}
		if enabledSecondIP {
			if err := allocateInSubnet(p.secondIPAM, keyObj.KeyInDB, ipNet, policy, attr, "filter"); err != nil {
				return err
			}
		}
	}
	return nil
}

// Prioritize can score each node, currently it does nothing
func (p *FloatingIPPlugin) Prioritize(pod *corev1.Pod, nodes []corev1.Node) (*schedulerapi.HostPriorityList, error) {
	list := &schedulerapi.HostPriorityList{}
	if !p.hasResourceName(&pod.Spec) {
		return list, nil
	}
	//TODO
	return list, nil
}

func (p *FloatingIPPlugin) allocateIP(ipam floatingip.IPAM, key string, nodeName string,
	pod *corev1.Pod) (*constant.IPInfo, error) {
	var how string
	ipInfo, err := ipam.First(key)
	if err != nil {
		return nil, fmt.Errorf("failed to query floating ip by key %s: %v", key, err)
	}
	started := time.Now()
	policy := parseReleasePolicy(&pod.ObjectMeta)
	attr := getAttr(nodeName)
	if ipInfo != nil {
		how = "reused"
	} else {
		subnet, err := p.queryNodeSubnet(nodeName)
		if err != nil {
			return nil, err
		}
		if err := allocateInSubnet(ipam, key, subnet, policy, attr, "bind"); err != nil {
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
	glog.Infof("AssignIP nodeName %s, ip %s, key %s", nodeName, ipInfo.IPInfo.IP.IP.String(), key)
	if err := p.cloudProviderAssignIP(&rpc.AssignIPRequest{
		NodeName:  nodeName,
		IPAddress: ipInfo.IPInfo.IP.IP.String(),
	}); err != nil {
		// do not rollback allocated ip
		return nil, fmt.Errorf("failed to assign ip %s to %s: %v", ipInfo.IPInfo.IP.IP.String(), key, err)
	}
	if how == "reused" {
		glog.Infof("pod %s reused %s, updating policy to %v attr %s", key, ipInfo.IPInfo.IP.String(), policy, attr)
		if err := ipam.UpdatePolicy(key, ipInfo.IPInfo.IP.IP, policy, attr); err != nil {
			return nil, fmt.Errorf("failed to update floating ip release policy: %v", err)
		}
	}
	glog.Infof("[%s] started at %d %s ip %s, policy %v, attr %s for %s", ipam.Name(), started.UnixNano(), how,
		ipInfo.IPInfo.IP.String(), policy, attr, key)
	return &ipInfo.IPInfo, nil
}

// Bind binds a new floatingip or reuse an old one to pod
func (p *FloatingIPPlugin) Bind(args *schedulerapi.ExtenderBindingArgs) error {
	p.resyncLock.RLock()
	defer p.resyncLock.RUnlock()
	pod, err := p.PluginFactoryArgs.PodLister.Pods(args.PodNamespace).Get(args.PodName)
	if err != nil {
		return fmt.Errorf("failed to find pod %s: %v", util.Join(args.PodName, args.PodNamespace), err)
	}
	if !p.hasResourceName(&pod.Spec) {
		// we will config extender resources which ensures pod which doesn't want floatingip won't be sent to plugin
		// see https://github.com/kubernetes/kubernetes/pull/60332
		return fmt.Errorf("pod which doesn't want floatingip have been sent to plugin")
	}
	keyObj := util.FormatKey(pod)
	ipInfo, err := p.allocateIP(p.ipam, keyObj.KeyInDB, args.Node, pod)
	if err != nil {
		return err
	}
	ipInfos := []constant.IPInfo{*ipInfo}
	if p.enabledSecondIP(pod) {
		secondIPInfo, err := p.allocateIP(p.secondIPAM, keyObj.KeyInDB, args.Node, pod)
		// TODO release ip if it's been allocated in this goroutine?
		if err != nil {
			return fmt.Errorf("[%s] %v", p.secondIPAM.Name(), err)
		}
		ipInfos = append(ipInfos, *secondIPInfo)
	}
	bindAnnotation := make(map[string]string)
	data, err := constant.FormatIPInfo(ipInfos)
	if err != nil {
		return fmt.Errorf("failed to format ipinfo %v: %v", ipInfos, err)
	}
	bindAnnotation[constant.ExtendedCNIArgsAnnotation] = data //TODO don't overlap this annotation
	var err1 error
	if err := wait.PollImmediate(time.Millisecond*500, 3*time.Second, func() (bool, error) {
		// It's the extender's response to bind pods to nodes since it is a binder
		if err := p.Client.CoreV1().Pods(args.PodNamespace).Bind(&corev1.Binding{
			ObjectMeta: v1.ObjectMeta{Namespace: args.PodNamespace, Name: args.PodName, UID: args.PodUID,
				Annotations: bindAnnotation},
			Target: corev1.ObjectReference{
				Kind: "Node",
				Name: args.Node,
			},
		}); err != nil {
			err1 = err
			if isPodNotFoundError(err) {
				// break retry if pod no longer exists
				return false, err
			}
			return false, nil
		}
		glog.Infof("bind pod %s to %s with ip %v", keyObj.KeyInDB, args.Node,
			bindAnnotation[constant.ExtendedCNIArgsAnnotation])
		return true, nil
	}); err != nil {
		if isPodNotFoundError(err1) {
			glog.Infof("binding returns not found for pod %s, putting it into unreleased chan", keyObj.KeyInDB)
			// attach ip annotation
			p.unreleased <- &releaseEvent{pod: pod}
		}
		// If fails to update, depending on resync to update
		return fmt.Errorf("update pod %s: %w", keyObj.KeyInDB, err1)
	}
	return nil
}

func isPodNotFoundError(err error) bool {
	return metaErrs.IsNotFound(err)
}

// unbind release ip from pod
func (p *FloatingIPPlugin) unbind(pod *corev1.Pod) error {
	p.resyncLock.RLock()
	defer p.resyncLock.RUnlock()
	glog.V(3).Infof("handle unbind pod %s", pod.Name)
	keyObj := util.FormatKey(pod)
	key := keyObj.KeyInDB
	if p.cloudProvider != nil {
		ipInfo, err := p.ipam.First(key)
		if err != nil {
			return fmt.Errorf("failed to query floating ip of %s: %v", key, err)
		}
		if ipInfo == nil {
			glog.Infof("pod %s hasn't an allocated ip", key)
			return nil
		}
		ipStr := ipInfo.IPInfo.IP.IP.String()
		glog.Infof("UnAssignIP nodeName %s, ip %s, key %s", pod.Spec.NodeName, ipStr, key)
		if err = p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
			NodeName:  pod.Spec.NodeName,
			IPAddress: ipStr,
		}); err != nil {
			return fmt.Errorf("failed to unassign ip %s from %s: %v", ipStr, key, err)
		}
	}
	policy := parseReleasePolicy(&pod.ObjectMeta)
	if keyObj.Deployment() {
		return p.unbindDpPod(pod, keyObj, policy)
	}
	return p.unbindStsOrTappPod(pod, keyObj, policy)
}

// hasResourceName checks if the podspec has floatingip resource name
func (p *FloatingIPPlugin) hasResourceName(spec *corev1.PodSpec) bool {
	for i := range spec.Containers {
		reqResource := spec.Containers[i].Resources.Requests
		for name := range reqResource {
			if name == constant.ResourceName {
				return true
			}
		}
	}
	return false
}

func getNodeIP(node *corev1.Node) net.IP {
	for i := range node.Status.Addresses {
		if node.Status.Addresses[i].Type == corev1.NodeInternalIP {
			return net.ParseIP(node.Status.Addresses[i].Address)
		}
	}
	return nil
}

func evicted(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed && pod.Status.Reason == "Evicted"
}

// getNodeSubnet gets node subnet from ipam
func (p *FloatingIPPlugin) getNodeSubnet(node *corev1.Node) (*net.IPNet, error) {
	p.nodeSubnetLock.Lock()
	defer p.nodeSubnetLock.Unlock()
	if subnet, ok := p.nodeSubnet[node.Name]; !ok {
		return p.getNodeSubnetfromIPAM(node)
	} else {
		return subnet, nil
	}
}

// queryNodeSubnet gets node subnet from ipam
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
		return p.getNodeSubnetfromIPAM(node)
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

func parseReleasePolicy(meta *v1.ObjectMeta) constant.ReleasePolicy {
	if meta == nil || meta.Annotations == nil {
		return constant.ReleasePolicyPodDelete
	}
	// if there is a pool annotations, we consider it as never release policy
	pool := constant.GetPool(meta.Annotations)
	if pool != "" {
		return constant.ReleasePolicyNever
	}
	return constant.ConvertReleasePolicy(meta.Annotations[constant.ReleasePolicyAnnotation])
}

// Attr stores attrs about this pod
type Attr struct {
	NodeName string // need this attr to send unassign request to cloud provider on resync
}

func getAttr(nodeName string) string {
	obj := Attr{NodeName: nodeName}
	attr, err := json.Marshal(obj)
	if err != nil {
		glog.Warningf("failed to marshal attr %+v: %v", obj, err)
	}
	return string(attr)
}

func (p *FloatingIPPlugin) GetLockPool() *keylock.Keylock {
	return p.dpLockPool
}

func (p *FloatingIPPlugin) GetIpam() floatingip.IPAM {
	return p.ipam
}

func (p *FloatingIPPlugin) GetSecondIpam() floatingip.IPAM {
	return p.secondIPAM
}
