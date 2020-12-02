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
	"tkestack.io/galaxy/pkg/ipam/floatingip"
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
		unreleased:        make(chan *releaseEvent, 50000),
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
		var caller string
		pc, _, no, ok := runtime.Caller(1)
		details := runtime.FuncForPC(pc)
		if ok && details != nil {
			caller = fmt.Sprintf("called from %s:%d\n", details.Name(), no)
		}
		glog.Infof("acquire lock for %s took %d ms, started at %s, %s", key, elapsed,
			start.Format("15:04:05.000"), caller)
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
