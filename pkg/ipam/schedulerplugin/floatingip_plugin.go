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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	glog "k8s.io/klog"
	"k8s.io/utils/keymutex"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/galaxy/constant/utils"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider/rpc"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

// FloatingIPPlugin Allocates Floating IP for deployments
type FloatingIPPlugin struct {
	ipam floatingip.IPAM
	// node name to subnet cache
	nodeSubnet     map[string]*net.IPNet
	nodeSubnetLock sync.Mutex
	*PluginFactoryArgs
	lastIPConf    string
	conf          *Conf
	unreleased    chan *releaseEvent
	cloudProvider cloudprovider.CloudProvider
	// protect unbind immutable deployment pod
	dpLockPool keymutex.KeyMutex
	// protect bind/unbind for each pod
	podLockPool keymutex.KeyMutex
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
		dpLockPool:        keymutex.NewHashed(500000),
		podLockPool:       keymutex.NewHashed(500000),
	}
	plugin.ipam = floatingip.NewCrdIPAM(args.CrdClient, floatingip.InternalIp, plugin.FIPInformer)
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
	glog.Infof("plugin init done")
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
	go wait.Until(func() {
		if err := p.resyncPod(); err != nil {
			glog.Warningf("resync pod: %v", err)
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
	var updated bool
	if updated, err = p.ensureIPAMConf(&p.lastIPConf, val); err != nil {
		return false, err
	}
	defer func() {
		if !updated {
			return
		}
		// If floatingip configuration changes, node's subnet may change either
		p.nodeSubnetLock.Lock()
		defer p.nodeSubnetLock.Unlock()
		p.nodeSubnet = map[string]*net.IPNet{}
	}()
	return true, nil
}

// Filter marks nodes which have no available ips as FailedNodes
// If the given pod doesn't want floating IP, none failedNodes returns
func (p *FloatingIPPlugin) Filter(pod *corev1.Pod, nodes []corev1.Node) ([]corev1.Node, schedulerapi.FailedNodesMap,
	error) {
	failedNodesMap := schedulerapi.FailedNodesMap{}
	if !p.hasResourceName(&pod.Spec) {
		return nodes, failedNodesMap, nil
	}
	filteredNodes := []corev1.Node{}
	defer p.lockPod(pod.Name, pod.Namespace)()
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
	keyObj, err := util.FormatKey(pod)
	if err != nil {
		return nil, err
	}
	// first check if exists an already allocated ip for this pod
	subnets, err := p.ipam.NodeSubnetsByKey(keyObj.KeyInDB)
	if err != nil {
		return nil, fmt.Errorf("failed to query by key %s: %v", keyObj.KeyInDB, err)
	}
	if len(subnets) > 0 {
		glog.V(3).Infof("%s already have an allocated ip in subnets %v", keyObj.KeyInDB, subnets)
		return subnets, nil
	}
	policy := parseReleasePolicy(&pod.ObjectMeta)
	if !keyObj.Deployment() && !keyObj.StatefulSet() && !keyObj.TApp() && policy != constant.ReleasePolicyPodDelete {
		return nil, fmt.Errorf("policy %s not supported for non deployment/tapp/sts app", constant.PolicyStr(policy))
	}
	var replicas int
	var isPoolSizeDefined bool
	if keyObj.Deployment() {
		replicas, isPoolSizeDefined, err = p.getDpReplicas(keyObj)
		if err != nil {
			return nil, err
		}
		// Lock to make checking available subnets and allocating reserved ip atomic
		defer p.LockDpPool(keyObj.PoolPrefix())()
	}
	subnetSet, reserve, err := p.getAvailableSubnet(keyObj, policy, replicas, isPoolSizeDefined)
	if err != nil {
		return nil, err
	}
	if (reserve || isPoolSizeDefined) && subnetSet.Len() > 0 {
		// Since bind is in a different goroutine than filter in scheduler, we can't ensure this pod got binded
		// before the next one got filtered to ensure max size of allocated ips.
		// So we'd better do the allocate in filter for reserve situation.
		reserveSubnet := subnetSet.List()[0]
		subnetSet = sets.NewString(reserveSubnet)
		if err := p.allocateDuringFilter(keyObj, reserve, isPoolSizeDefined, reserveSubnet, policy,
			string(pod.UID)); err != nil {
			return nil, err
		}
	}
	return subnetSet, nil
}

func (p *FloatingIPPlugin) allocateDuringFilter(keyObj *util.KeyObj, reserve, isPoolSizeDefined bool,
	reserveSubnet string, policy constant.ReleasePolicy, uid string) error {
	// we can't get nodename during filter, update attr on bind
	attr := getAttr("", uid)
	if reserve {
		if err := p.allocateInSubnetWithKey(keyObj.PoolPrefix(), keyObj.KeyInDB, reserveSubnet, policy, attr,
			"filter"); err != nil {
			return err
		}
	} else if isPoolSizeDefined {
		// if pool size defined and we got no reserved IP, we need to allocate IP from empty key
		_, ipNet, err := net.ParseCIDR(reserveSubnet)
		if err != nil {
			return err
		}
		if err := p.allocateInSubnet(keyObj.KeyInDB, ipNet, policy, attr, "filter"); err != nil {
			return err
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

func (p *FloatingIPPlugin) allocateIP(key string, nodeName string, pod *corev1.Pod) (*constant.IPInfo, error) {
	var how string
	ipInfo, err := p.ipam.First(key)
	if err != nil {
		return nil, fmt.Errorf("failed to query floating ip by key %s: %v", key, err)
	}
	started := time.Now()
	policy := parseReleasePolicy(&pod.ObjectMeta)
	attr := getAttr(nodeName, string(pod.UID))
	if ipInfo != nil {
		how = "reused"
		// check if uid missmatch, if we delete a statfulset/tapp and creates a same name statfulset/tapp immediately,
		// galaxy-ipam may receive bind event for new pod early than deleting event for old pod
		var oldAttr Attr
		if ipInfo.FIP.Attr != "" {
			if err := json.Unmarshal([]byte(ipInfo.FIP.Attr), &oldAttr); err != nil {
				return nil, fmt.Errorf("failed to unmarshal attr %s", ipInfo.FIP.Attr)
			}
			if oldAttr.Uid != "" && oldAttr.Uid != string(pod.UID) {
				return nil, fmt.Errorf("waiting for delete event of %s before reuse this ip", key)
			}
		}
	} else {
		subnet, err := p.queryNodeSubnet(nodeName)
		if err != nil {
			return nil, err
		}
		if err := p.allocateInSubnet(key, subnet, policy, attr, "bind"); err != nil {
			return nil, err
		}
		how = "allocated"
		ipInfo, err = p.ipam.First(key)
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
		if err := p.ipam.UpdatePolicy(key, ipInfo.IPInfo.IP.IP, policy, attr); err != nil {
			return nil, fmt.Errorf("failed to update floating ip release policy: %v", err)
		}
	}
	glog.Infof("started at %d %s ip %s, policy %v, attr %s for %s", started.UnixNano(), how,
		ipInfo.IPInfo.IP.String(), policy, attr, key)
	return &ipInfo.IPInfo, nil
}

// Bind binds a new floatingip or reuse an old one to pod
func (p *FloatingIPPlugin) Bind(args *schedulerapi.ExtenderBindingArgs) error {
	pod, err := p.PluginFactoryArgs.PodLister.Pods(args.PodNamespace).Get(args.PodName)
	if err != nil {
		return fmt.Errorf("failed to find pod %s: %w", util.Join(args.PodName, args.PodNamespace), err)
	}
	if !p.hasResourceName(&pod.Spec) {
		// we will config extender resources which ensures pod which doesn't want floatingip won't be sent to plugin
		// see https://github.com/kubernetes/kubernetes/pull/60332
		return fmt.Errorf("pod which doesn't want floatingip have been sent to plugin")
	}
	defer p.lockPod(pod.Name, pod.Namespace)()
	keyObj, err := util.FormatKey(pod)
	if err != nil {
		return err
	}
	ipInfo, err := p.allocateIP(keyObj.KeyInDB, args.Node, pod)
	if err != nil {
		return err
	}
	ipInfos := []constant.IPInfo{*ipInfo}
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
	return apierrors.IsNotFound(err)
}

// unbind release ip from pod
func (p *FloatingIPPlugin) unbind(pod *corev1.Pod) error {
	defer p.lockPod(pod.Name, pod.Namespace)()
	glog.V(3).Infof("handle unbind pod %s", pod.Name)
	keyObj, err := util.FormatKey(pod)
	if err != nil {
		return err
	}
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
		var attr Attr
		if err := json.Unmarshal([]byte(ipInfo.FIP.Attr), &attr); err != nil {
			return fmt.Errorf("failed to unmarshal attr %s for pod %s: %v", ipInfo.FIP.Attr, key, err)
		}

		glog.Infof("UnAssignIP nodeName %s, ip %s, key %s", attr.NodeName, ipStr, key)
		if err = p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
			NodeName:  attr.NodeName,
			IPAddress: ipStr,
		}); err != nil {
			return fmt.Errorf("failed to unassign ip %s from %s: %v", ipStr, key, err)
		}
	}
	policy := parseReleasePolicy(&pod.ObjectMeta)
	if keyObj.Deployment() {
		return p.unbindDpPod(keyObj, policy, "during unbinding pod")
	}
	return p.unbindStsOrTappPod(pod, keyObj, policy)
}

// hasResourceName checks if the podspec has floatingip resource name
func (p *FloatingIPPlugin) hasResourceName(spec *corev1.PodSpec) bool {
	return utils.WantENIIP(spec)
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
			if !apierrors.IsServerTimeout(err) {
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
	// NodeName is needed to send unassign request to cloud provider on resync
	NodeName string
	// uid is used to differentiate a deleting pod and a newly created pod with the same name such as statefulsets
	// or tapp pod
	Uid string
}

func getAttr(nodeName, uid string) string {
	obj := Attr{NodeName: nodeName, Uid: uid}
	attr, err := json.Marshal(obj)
	if err != nil {
		glog.Warningf("failed to marshal attr %+v: %v", obj, err)
	}
	return string(attr)
}

func (p *FloatingIPPlugin) GetIpam() floatingip.IPAM {
	return p.ipam
}

func (p *FloatingIPPlugin) lockPod(name, namespace string) func() {
	key := fmt.Sprintf("%s_%s", namespace, name)
	p.podLockPool.LockKey(key)
	return func() {
		_ = p.podLockPool.UnlockKey(key)
	}
}
