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
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	glog "k8s.io/klog"
	"k8s.io/utils/keymutex"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/galaxy/constant/utils"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	"tkestack.io/galaxy/pkg/ipam/cloudprovider"
	"tkestack.io/galaxy/pkg/ipam/context"
	"tkestack.io/galaxy/pkg/ipam/crd"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

// FloatingIPPlugin Allocates Floating IP for deployments
type FloatingIPPlugin struct {
	ipam floatingip.IPAM
	// node name to subnet cache
	nodeSubnet     map[string]*net.IPNet
	nodeSubnetLock sync.Mutex
	*context.IPAMContext
	lastIPConf    string
	conf          *Conf
	unreleased    chan *releaseEvent
	cloudProvider cloudprovider.CloudProvider
	// protect unbind immutable deployment pod
	dpLockPool keymutex.KeyMutex
	// protect bind/unbind for each pod
	podLockPool keymutex.KeyMutex
	crdCache    crd.CrdCache
	crdKey      CrdKey
}

// NewFloatingIPPlugin creates FloatingIPPlugin
func NewFloatingIPPlugin(conf Conf, ctx *context.IPAMContext) (*FloatingIPPlugin, error) {
	conf.validate()
	glog.Infof("floating ip config: %v", conf)
	plugin := &FloatingIPPlugin{
		nodeSubnet:  make(map[string]*net.IPNet),
		IPAMContext: ctx,
		conf:        &conf,
		unreleased:  make(chan *releaseEvent, 50000),
		dpLockPool:  keymutex.NewHashed(500000),
		podLockPool: keymutex.NewHashed(500000),
		crdKey:      NewCrdKey(ctx.ExtensionLister),
		crdCache:    crd.NewCrdCache(ctx.DynamicClient, ctx.ExtensionLister, 0),
	}
	plugin.ipam = floatingip.NewCrdIPAM(ctx.GalaxyClient, floatingip.InternalIp, plugin.FIPInformer)
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

// Prioritize can score each node, currently it does nothing
func (p *FloatingIPPlugin) Prioritize(pod *corev1.Pod, nodes []corev1.Node) (*schedulerapi.HostPriorityList, error) {
	list := &schedulerapi.HostPriorityList{}
	if !p.hasResourceName(&pod.Spec) {
		return list, nil
	}
	//TODO
	return list, nil
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

// finished returns true if pod completes and won't be restarted again
func finished(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded
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

func (p *FloatingIPPlugin) GetIpam() floatingip.IPAM {
	return p.ipam
}

func (p *FloatingIPPlugin) lockPod(name, namespace string) func() {
	key := fmt.Sprintf("%s_%s", namespace, name)
	start := time.Now()
	p.podLockPool.LockKey(key)
	elapsed := (time.Now().UnixNano() - start.UnixNano()) / 1e6
	if elapsed > 500 {
		glog.Infof("acquire lock for %s took %d ms, started at %s, %s", key, elapsed,
			start.Format("15:04:05.000"), getCaller())
	}
	return func() {
		_ = p.podLockPool.UnlockKey(key)
	}
}

func getPodCniArgs(pod *corev1.Pod) (constant.CniArgs, error) {
	m := pod.GetAnnotations()
	if len(m) == 0 {
		return constant.CniArgs{}, nil
	}
	str, ok := m[constant.ExtendedCNIArgsAnnotation]
	if !ok {
		return constant.CniArgs{}, nil
	}
	args, err := constant.UnmarshalCniArgs(str)
	if args == nil {
		args = &constant.CniArgs{}
	}
	return *args, err
}

// supportReserveIPPolicy checks if reserveIP release policy is supported for a given keyObj
func (p *FloatingIPPlugin) supportReserveIPPolicy(obj *util.KeyObj, policy constant.ReleasePolicy) error {
	if obj.Deployment() || obj.StatefulSet() {
		return nil
	}
	_, err := parsePodIndex(obj.PodName)
	if err != nil {
		return NotStatefulWorkload
	}
	if policy == constant.ReleasePolicyNever {
		return nil
	}
	gvr := p.crdKey.GetGroupVersionResource(obj.AppTypePrefix)
	if gvr == nil {
		return NoReplicas
	}
	return nil
}

// getCaller returns the func packageName.funcName of the caller
func getCaller() string {
	pc, _, no, ok := runtime.Caller(2)
	details := runtime.FuncForPC(pc)
	if ok && details != nil {
		return fmt.Sprintf("called from %s:%d\n", details.Name(), no)
	}
	return ""
}
