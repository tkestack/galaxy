package schedulerplugin

import (
	"encoding/json"
	"fmt"

	"git.code.oa.com/tkestack/galaxy/pkg/ipam/cloudprovider/rpc"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/tkestack/galaxy/pkg/utils/nets"
	glog "k8s.io/klog"
)

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

func (p *FloatingIPPlugin) resyncCloudProviderIPs(ipam floatingip.IPAM, meta *resyncMeta) {
	for key, obj := range meta.assignedPods {
		if _, ok := meta.existPods[key]; ok {
			continue
		}
		// check with apiserver to confirm it really not exist
		if p.podExist(obj.keyObj.PodName, obj.keyObj.Namespace) {
			continue
		}
		var attr Attr
		if err := json.Unmarshal([]byte(obj.fip.Attr), &attr); err != nil {
			glog.Errorf("failed to unmarshal attr %s for pod %s: %v", obj.fip.Attr, key, err)
			continue
		}
		if attr.NodeName == "" {
			glog.Errorf("empty nodeName for %s in db", key)
			continue
		}
		glog.Infof("UnAssignIP nodeName %s, ip %s, key %s during resync", attr.NodeName, nets.IntToIP(obj.fip.IP).String(), key)
		if err := p.cloudProviderUnAssignIP(&rpc.UnAssignIPRequest{
			NodeName:  attr.NodeName,
			IPAddress: nets.IntToIP(obj.fip.IP).String(),
		}); err != nil {
			// delete this record from allocatedIPs map to have a retry
			delete(meta.allocatedIPs, key)
			glog.Warningf("failed to unassign ip %s to %s: %v", nets.IntToIP(obj.fip.IP).String(), key, err)
			continue
		}
		// for tapp and sts pod, we need to clean its node attr
		if err := ipam.ReserveIP(key, key, getAttr("")); err != nil {
			glog.Errorf("failed to reserve %s ip: %v", key, err)
		}
	}
}
