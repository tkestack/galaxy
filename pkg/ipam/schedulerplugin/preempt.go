package schedulerplugin

import (
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
)

func fillNodeNameToMetaVictims(args *schedulerapi.ExtenderPreemptionArgs) {
	if len(args.NodeNameToVictims) != 0 && len(args.NodeNameToMetaVictims) == 0 {
		args.NodeNameToMetaVictims = map[string]*schedulerapi.MetaVictims{}
		for node, victim := range args.NodeNameToVictims {
			metaVictim := &schedulerapi.MetaVictims{
				Pods:             []*schedulerapi.MetaPod{},
				NumPDBViolations: victim.NumPDBViolations,
			}
			for _, pod := range victim.Pods {
				metaPod := &schedulerapi.MetaPod{
					UID: string(pod.UID),
				}
				metaVictim.Pods = append(metaVictim.Pods, metaPod)
			}
			args.NodeNameToMetaVictims[node] = metaVictim
		}
	}
}

func (p *FloatingIPPlugin) Preempt(args *schedulerapi.ExtenderPreemptionArgs) map[string]*schedulerapi.MetaVictims {
	fillNodeNameToMetaVictims(args)
	policy := parseReleasePolicy(&args.Pod.ObjectMeta)
	if policy == constant.ReleasePolicyPodDelete {
		return args.NodeNameToMetaVictims
	}
	subnetSet, err := p.getSubnet(args.Pod)
	if err != nil {
		glog.Errorf("unable to get pod subnets: %v", err)
		return args.NodeNameToMetaVictims
	}
	glog.V(4).Infof("subnet for pod %v is %v", args.Pod.Name, subnetSet)
	for nodeName := range args.NodeNameToMetaVictims {
		node, err := p.NodeLister.Get(nodeName)
		if err != nil {
			glog.Errorf("unable to list node: %v", err)
			delete(args.NodeNameToMetaVictims, nodeName)
			continue
		}
		subnet, err := p.getNodeSubnet(node)
		if err != nil {
			glog.Errorf("unable to get node %v subnet: %v", nodeName, err)
			delete(args.NodeNameToMetaVictims, nodeName)
			continue
		}
		if !subnetSet.Has(subnet.String()) {
			glog.V(4).Infof("remove node %v with subnet %v from victim", node.Name, subnet.String())
			delete(args.NodeNameToMetaVictims, nodeName)
		}
	}
	return args.NodeNameToMetaVictims
}
