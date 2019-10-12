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

	"git.code.oa.com/tkestack/galaxy/pkg/api/galaxy/constant"
	"git.code.oa.com/tkestack/galaxy/pkg/api/galaxy/private"
	"git.code.oa.com/tkestack/galaxy/pkg/api/k8s/schedulerapi"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/cloudprovider"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/cloudprovider/rpc"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/schedulerplugin/util"
	"git.code.oa.com/tkestack/galaxy/pkg/utils/database"
	"git.code.oa.com/tkestack/galaxy/pkg/utils/keylock"
	glog "k8s.io/klog"
	corev1 "k8s.io/api/core/v1"
	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	deletedAndIPMutablePod         = "deletedAndIPMutablePod"
	deletedAndParentAppNotExistPod = "deletedAndParentAppNotExistPod"
	deletedAndScaledDownAppPod     = "deletedAndScaledDownAppPod"
	deletedAndScaledDownDpPod      = "deletedAndScaledDownDpPod"
)

type Conf struct {
	FloatingIPs           []*floatingip.FloatingIP `json:"floatingips,omitempty"`
	DBConfig              *database.DBConfig       `json:"database"`
	ResyncInterval        uint                     `json:"resyncInterval"`
	ConfigMapName         string                   `json:"configMapName"`
	ConfigMapNamespace    string                   `json:"configMapNamespace"`
	FloatingIPKey         string                   `json:"floatingipKey"`       // configmap floatingip data key
	SecondFloatingIPKey   string                   `json:"secondFloatingipKey"` // configmap second floatingip data key
	CloudProviderGRPCAddr string                   `json:"cloudProviderGrpcAddr"`
	StorageDriver         string                   `json:"storageDriver"`
}

// FloatingIPPlugin Allocates Floating IP for deployments
type FloatingIPPlugin struct {
	ipam, secondIPAM floatingip.IPAM
	// node name to subnet cache
	nodeSubnet     map[string]*net.IPNet
	nodeSubnetLock sync.Mutex
	sync.Mutex
	*PluginFactoryArgs
	lastIPConf, lastSecondIPConf string
	conf                         *Conf
	unreleased                   chan *releaseEvent
	hasSecondIPConf              atomic.Value
	db                           *database.DBRecorder
	cloudProvider                cloudprovider.CloudProvider
	// protect unbind immutable deployment pod
	dpLockPool *keylock.Keylock
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
	if conf.StorageDriver == "" {
		conf.StorageDriver = "mysql"
	}
	glog.Infof("floating ip config: %v", conf)
	plugin := &FloatingIPPlugin{
		nodeSubnet:        make(map[string]*net.IPNet),
		PluginFactoryArgs: args,
		conf:              &conf,
		unreleased:        make(chan *releaseEvent, 100),
		dpLockPool:        keylock.NewKeylock(),
	}
	if conf.StorageDriver == "mysql" {
		db := database.NewDBRecorder(conf.DBConfig)
		if err := db.Run(); err != nil {
			return nil, err
		}
		plugin.db = db
		plugin.ipam = floatingip.NewIPAM(db)
		plugin.secondIPAM = floatingip.NewIPAMWithTableName(db, database.SecondFloatingipTableName)
	} else if conf.StorageDriver == "k8s-crd" {
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

func (p *FloatingIPPlugin) Run(stop chan struct{}) {
	if len(p.conf.FloatingIPs) == 0 {
		go wait.Until(func() {
			if _, err := p.updateConfigMap(); err != nil {
				glog.Warning(err)
			}
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
	for i := 0; i < 5; i++ {
		go p.loop(stop)
	}
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
	if !ok {
		return true, nil
	}
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

// Filter marks nodes which have no available ips as FailedNodes
// If the given pod doesn't want floating IP, none failedNodes returns
func (p *FloatingIPPlugin) Filter(pod *corev1.Pod, nodes []corev1.Node) ([]corev1.Node, schedulerapi.FailedNodesMap, error) {
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
	if bool(glog.V(4)) {
		nodeNames := make([]string, len(filteredNodes))
		for i := range filteredNodes {
			nodeNames[i] = filteredNodes[i].Name
		}
		glog.V(5).Infof("filtered nodes %v failed nodes %v", nodeNames, failedNodesMap)
	}
	return filteredNodes, failedNodesMap, nil
}

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
		replicas, isPoolSizeDefined, err = p.getReplicas(keyObj)
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
		// Since bind is in a different goroutine than filter in scheduler, we can't ensure the first pod got binded before the next one got filtered.
		// We'd better do the allocate in filter for reserve situation.
		reserveSubnet := subnetSet.List()[0]
		subnetSet = sets.NewString(reserveSubnet)
		// we can't get nodename on filter, update attr on bind
		attr := getAttr("")
		if reserve {
			if err := allocateInSubnetWithKey(p.ipam, keyObj.PoolPrefix(), keyObj.KeyInDB, reserveSubnet, policy, attr); err != nil {
				return nil, err
			}
			if p.enabledSecondIP(pod) {
				if err := allocateInSubnetWithKey(p.secondIPAM, keyObj.PoolPrefix(), keyObj.KeyInDB, reserveSubnet, policy, attr); err != nil {
					return nil, err
				}
			}
		} else if isPoolSizeDefined {
			// if pool size defined and we got no reserved IP, we need to allocate IP from empty key
			_, ipNet, err := net.ParseCIDR(reserveSubnet)
			if err != nil {
				return nil, err
			}
			if err := allocateInSubnet(p.ipam, keyObj.KeyInDB, ipNet, policy, attr, "filter"); err != nil {
				return nil, err
			}
			if p.enabledSecondIP(pod) {
				if err := allocateInSubnet(p.secondIPAM, keyObj.KeyInDB, ipNet, policy, attr, "filter"); err != nil {
					return nil, err
				}
			}
		}
	}
	return subnetSet, nil
}

func allocateInSubnet(ipam floatingip.IPAM, key string, subnet *net.IPNet, policy constant.ReleasePolicy, attr, when string) error {
	ip, err := ipam.AllocateInSubnet(key, subnet, policy, attr)
	if err != nil {
		return err
	}
	glog.Infof("[%s] allocated ip %s to pod %s during %s", ipam.Name(), ip.String(), key, when)
	return nil
}

func allocateInSubnetWithKey(ipam floatingip.IPAM, oldK, newK, subnet string, policy constant.ReleasePolicy, attr string) error {
	if err := ipam.AllocateInSubnetWithKey(oldK, newK, subnet, policy, attr); err != nil {
		return err
	}
	fip, err := ipam.First(newK)
	if err != nil {
		return err
	}
	glog.Infof("[%s] allocated ip %s to %s from %s", ipam.Name(), fip.IPInfo.IP.String(), newK, oldK)
	return nil
}

// getReplicas returns replicas, isPoolSizeDefined, error
func (p *FloatingIPPlugin) getReplicas(keyObj *util.KeyObj) (int, bool, error) {
	if keyObj.PoolName != "" {
		pool, err := p.PoolLister.Pools("kube-system").Get(keyObj.PoolName)
		if err == nil {
			glog.V(4).Infof("pool %s size %d", pool.Name, pool.Size)
			return pool.Size, true, nil
		} else {
			if !metaErrs.IsNotFound(err) {
				return 0, false, err
			}
			// pool not found, get replicas from deployment
		}
	}
	deployment, err := p.DeploymentLister.Deployments(keyObj.Namespace).Get(keyObj.AppName)
	if err != nil {
		return 0, false, err
	}
	return int(*deployment.Spec.Replicas), false, nil
}

func getAvailableSubnet(ipam floatingip.IPAM, keyObj *util.KeyObj, policy constant.ReleasePolicy, replicas int, isPoolSizeDefined bool) (subnets []string, reserve bool, err error) {
	if keyObj.Deployment() && policy != constant.ReleasePolicyPodDelete {
		var ips []database.FloatingIP
		poolPrefix := keyObj.PoolPrefix()
		poolAppPrefix := keyObj.PoolAppPrefix()
		ips, err = ipam.ByPrefix(poolPrefix)
		if err != nil {
			err = fmt.Errorf("failed query prefix %s: %s", poolPrefix, err)
			return
		}
		usedCount := 0
		unusedSubnetSet := sets.NewString()
		for _, ip := range ips {
			if ip.Key != poolPrefix {
				if isPoolSizeDefined || keyObj.PoolName == "" {
					usedCount++
				} else {
					if strings.HasPrefix(ip.Key, poolAppPrefix) {
						// Don't counting in other deployments' used ip if sharing pool
						usedCount++
					}
				}
			} else {
				unusedSubnetSet.Insert(ip.Subnet)
			}
		}
		glog.V(4).Infof("keyObj %v, unusedSubnetSet %v, usedCount %d, replicas %d, isPoolSizeDefined %v", keyObj, unusedSubnetSet, usedCount, replicas, isPoolSizeDefined)
		// check usedCount >= replicas to ensure upgrading a deployment won't change its ips
		if usedCount >= replicas {
			if isPoolSizeDefined {
				return nil, false, fmt.Errorf("reached pool %s size limit of %d", keyObj.PoolName, replicas)
			}
			return nil, false, fmt.Errorf("deployment %s has allocated %d ips with replicas of %d, wait for releasing", keyObj.AppName, usedCount, replicas)
		}
		if unusedSubnetSet.Len() > 0 {
			return unusedSubnetSet.List(), true, nil
		}
	}
	if subnets, err = ipam.QueryRoutableSubnetByKey(""); err != nil {
		err = fmt.Errorf("failed to query allocatable subnet: %v", err)
		return
	}
	return
}

func (p *FloatingIPPlugin) Prioritize(pod *corev1.Pod, nodes []corev1.Node) (*schedulerapi.HostPriorityList, error) {
	list := &schedulerapi.HostPriorityList{}
	if !p.hasResourceName(&pod.Spec) {
		return list, nil
	}
	//TODO
	return list, nil
}

func (p *FloatingIPPlugin) allocateIP(ipam floatingip.IPAM, key string, nodeName string, pod *corev1.Pod) (*constant.IPInfo, error) {
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
	glog.Infof("[%s] started at %d %s ip %s, policy %v, attr %s for %s", ipam.Name(), started.UnixNano(), how, ipInfo.IPInfo.IP.String(), policy, attr, key)
	return &ipInfo.IPInfo, nil
}

func (p *FloatingIPPlugin) cloudProviderAssignIP(req *rpc.AssignIPRequest) error {
	if p.cloudProvider == nil {
		return nil
	}
	reply, err := p.cloudProvider.AssignIP(req)
	if err != nil {
		return fmt.Errorf("cloud provider AssignIP reply err %v", err)
	}
	if reply == nil {
		return fmt.Errorf("cloud provider AssignIP nil reply")
	}
	if !reply.Success {
		return fmt.Errorf("cloud provider AssignIP reply failed, message %s", reply.Msg)
	}
	glog.Infof("AssignIP %v success", req)
	return nil
}

func (p *FloatingIPPlugin) cloudProviderUnAssignIP(req *rpc.UnAssignIPRequest) error {
	if p.cloudProvider == nil {
		return nil
	}
	reply, err := p.cloudProvider.UnAssignIP(req)
	if err != nil {
		return fmt.Errorf("cloud provider UnAssignIP reply err %v", err)
	}
	if reply == nil {
		return fmt.Errorf("cloud provider UnAssignIP nil reply")
	}
	if !reply.Success {
		return fmt.Errorf("cloud provider UnAssignIP reply failed, message %s", reply.Msg)
	}
	glog.Infof("UnAssignIP %v success", req)
	return nil
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
	if err := ipam.Release(key, ipInfo.IPInfo.IP.IP); err != nil {
		return fmt.Errorf("failed to release floating ip of %s because of %s: %v", key, reason, err)
	}
	glog.Infof("[%s] released floating ip %s from %s because of %s", ipam.Name(), ipInfo.IPInfo.IP.String(), key, reason)
	return nil
}

func (p *FloatingIPPlugin) AddPod(pod *corev1.Pod) error {
	return nil
}

func (p *FloatingIPPlugin) Bind(args *schedulerapi.ExtenderBindingArgs) error {
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
			ObjectMeta: v1.ObjectMeta{Namespace: args.PodNamespace, Name: args.PodName, UID: args.PodUID, Annotations: bindAnnotation},
			Target: corev1.ObjectReference{
				Kind: "Node",
				Name: args.Node,
			},
		}); err != nil {
			err1 = err
			return false, nil
		}
		glog.Infof("bind pod %s to %s with ip %v", keyObj.KeyInDB, args.Node, bindAnnotation[constant.ExtendedCNIArgsAnnotation])
		return true, nil
	}); err != nil {
		// If fails to update, depending on resync to update
		return fmt.Errorf("failed to update pod %s: %v", keyObj.KeyInDB, err1)
		//TODO release ip
	}
	return nil
}

func (p *FloatingIPPlugin) UpdatePod(oldPod, newPod *corev1.Pod) error {
	if !p.hasResourceName(&newPod.Spec) {
		return nil
	}
	if !evicted(oldPod) && evicted(newPod) {
		// Deployments will leave evicted pods
		// If it's a evicted one, release its ip
		p.unreleased <- &releaseEvent{pod: newPod}
	}
	if err := p.syncPodIP(newPod); err != nil {
		glog.Warningf("failed to sync pod ip: %v", err)
	}
	return nil
}

func (p *FloatingIPPlugin) DeletePod(pod *corev1.Pod) error {
	if !p.hasResourceName(&pod.Spec) {
		return nil
	}
	glog.Infof("handle pod delete event: %s_%s", pod.Name, pod.Namespace)
	p.unreleased <- &releaseEvent{pod: pod}
	return nil
}

func (p *FloatingIPPlugin) unbind(pod *corev1.Pod) error {
	glog.V(3).Infof("handle unbind pod %s", pod.Name)
	keyObj := util.FormatKey(pod)
	key := keyObj.KeyInDB
	if p.cloudProvider != nil {
		if pod.Annotations == nil || pod.Annotations[constant.ExtendedCNIArgsAnnotation] == "" {
			// If a pod has not been allocated an ip, e.g. ip pool is drained, do nothing
			// If the annotation is deleted manually, we count on resync to release ips
			return nil
		}
		ipInfos, err := constant.ParseIPInfo(pod.Annotations[constant.ExtendedCNIArgsAnnotation])
		if err != nil || len(ipInfos) == 0 || ipInfos[0].IP == nil {
			return fmt.Errorf("bad format of %s: %s, err %v", key, pod.Annotations[constant.ExtendedCNIArgsAnnotation], err)
		} else {
			glog.Infof("UnAssignIP nodeName %s, ip %s, key %s", pod.Spec.NodeName, ipInfos[0].IP.IP.String(), key)
			if err = p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
				NodeName:  pod.Spec.NodeName,
				IPAddress: ipInfos[0].IP.IP.String(),
			}); err != nil {
				return fmt.Errorf("failed to unassign ip %s from %s: %v", ipInfos[0].IP.IP.String(), key, err)
			}
		}
	}
	policy := parseReleasePolicy(&pod.ObjectMeta)
	if keyObj.Deployment() {
		return p.unbindDpPod(pod, keyObj, policy)
	}
	return p.unbindStsOrTappPod(pod, keyObj, policy)
}

func (p *FloatingIPPlugin) unbindStsOrTappPod(pod *corev1.Pod, keyObj *util.KeyObj, policy constant.ReleasePolicy) error {
	key := keyObj.KeyInDB
	if policy == constant.ReleasePolicyPodDelete {
		return p.releaseIP(key, deletedAndIPMutablePod, pod)
	} else if policy == constant.ReleasePolicyNever {
		return p.reserveIP(key, key, "never policy", p.enabledSecondIP(pod))
	} else if policy == constant.ReleasePolicyImmutable {
		var appExist bool
		var replicas int32
		// In test, p.StatefulSetLister may be nil
		if keyObj.StatefulSet() && p.StatefulSetLister != nil {
			statefulSet, err := p.StatefulSetLister.GetPodStatefulSets(pod)
			if err != nil {
				if !metaErrs.IsNotFound(err) {
					return err
				}
			} else {
				appExist = true
				if len(statefulSet) > 1 {
					glog.Warningf("multiple ss found for pod %s", util.PodName(pod))
				}
				ss := statefulSet[0]
				replicas = 1
				if ss.Spec.Replicas != nil {
					replicas = *ss.Spec.Replicas
				}
			}
		} else if keyObj.TApp() && p.TAppLister != nil {
			tapp, err := p.TAppLister.TApps(pod.Namespace).Get(keyObj.AppName)
			if err != nil {
				if !metaErrs.IsNotFound(err) {
					return err
				}
			} else {
				appExist = true
				replicas = tapp.Spec.Replicas
			}
		} else {
			return nil
		}
		shouldReserve, reason, err := p.shouldReserve(pod, keyObj, appExist, replicas)
		if err != nil {
			return err
		}
		if shouldReserve {
			return p.reserveIP(key, key, "immutable policy", p.enabledSecondIP(pod))
		} else {
			return p.releaseIP(key, reason, pod)
		}
	}
	return nil
}

func (p *FloatingIPPlugin) shouldReserve(pod *corev1.Pod, keyObj *util.KeyObj, appExist bool, replicas int32) (bool, string, error) {
	if !appExist {
		return false, deletedAndParentAppNotExistPod, nil
	}
	index, err := parsePodIndex(pod.Name)
	if err != nil {
		return false, "", fmt.Errorf("invalid pod name %s of key %s: %v", util.PodName(pod), keyObj.KeyInDB, err)
	}
	if replicas < int32(index)+1 {
		return false, deletedAndScaledDownAppPod, nil
	} else {
		return true, "", nil
	}
	return false, deletedAndParentAppNotExistPod, nil
}

func (p *FloatingIPPlugin) unbindDpPod(pod *corev1.Pod, keyObj *util.KeyObj, policy constant.ReleasePolicy) error {
	key, prefixKey := keyObj.KeyInDB, keyObj.PoolPrefix()
	dp, err := p.DeploymentLister.Deployments(keyObj.Namespace).Get(keyObj.AppName)
	replicas := 0
	if err != nil {
		if !metaErrs.IsNotFound(err) {
			return err
		}
	} else {
		replicas = int(*dp.Spec.Replicas)
	}
	if err := unbindDpPod(key, prefixKey, p.ipam, p.dpLockPool, replicas, policy, "unbinding pod"); err != nil {
		return err
	}
	if p.enabledSecondIP(pod) {
		return unbindDpPod(key, prefixKey, p.secondIPAM, p.dpLockPool, replicas, policy, "unbinding pod")
	}
	return nil
}

func unbindDpPod(key, prefixKey string, ipam floatingip.IPAM, dpLockPool *keylock.Keylock, replicas int, policy constant.ReleasePolicy, when string) error {
	if policy == constant.ReleasePolicyPodDelete {
		return releaseIP(ipam, key, deletedAndIPMutablePod)
	} else if policy == constant.ReleasePolicyNever {
		if key != prefixKey {
			return reserveIP(key, prefixKey, ipam, fmt.Sprintf("never release policy %s", when))
		}
		return nil
	}
	lockIndex := dpLockPool.GetLockIndex([]byte(prefixKey))
	dpLockPool.RawLock(lockIndex)
	defer dpLockPool.RawUnlock(lockIndex)
	fips, err := ipam.ByPrefix(prefixKey)
	if err != nil {
		return err
	}
	if len(fips) > replicas {
		return releaseIP(ipam, key, deletedAndScaledDownDpPod)
	} else {
		if key != prefixKey {
			return reserveIP(key, prefixKey, ipam, fmt.Sprintf("allocated %d <= replicas %d %s", len(fips), replicas, when))
		}
	}
	return nil
}

func (p *FloatingIPPlugin) reserveIP(old, new, reason string, enabledSecondIP bool) error {
	if err := reserveIP(old, new, p.ipam, reason); err != nil {
		return err
	}
	if enabledSecondIP {
		if err := reserveIP(old, new, p.secondIPAM, reason); err != nil {
			return err
		}
	}
	return nil
}

func reserveIP(key, prefixKey string, ipam floatingip.IPAM, reason string) error {
	if err := ipam.ReserveIP(key, prefixKey, getAttr("")); err != nil {
		return fmt.Errorf("[%s] failed to reserve ip from pod %s to %s: %v", ipam.Name(), key, prefixKey, err)
	}
	glog.Infof("[%s] reserved ip from pod %s to %s, because %s", ipam.Name(), key, prefixKey, reason)
	return nil
}

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

type releaseEvent struct {
	pod        *corev1.Pod
	retryTimes int
}

func (p *FloatingIPPlugin) loop(stop chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case event := <-p.unreleased:
			if err := p.unbind(event.pod); err != nil {
				event.retryTimes++
				glog.Warningf("unbind pod %s failed for %d times: %v", util.PodName(event.pod), event.retryTimes, err)
				if event.retryTimes > 3 {
					// leave it to resync to protect chan from explosion
					glog.Errorf("abort unbind for pod %s, retried %d times: %v", util.PodName(event.pod), event.retryTimes, err)
				} else {
					go func() {
						// backoff time if required
						time.Sleep(300 * time.Millisecond * time.Duration(event.retryTimes))
						p.unreleased <- event
					}()
				}
			}
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
