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
package floatingip

import (
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/apis/galaxy/v1alpha1"
)

func (ci *crdIpam) listFloatingIPs() (*v1alpha1.FloatingIPList, error) {
	val, err := ci.ipType.String()
	if err != nil {
		return nil, err
	}
	listOpt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constant.IpType, val),
	}
	fips, err := ci.client.GalaxyV1alpha1().FloatingIPs().List(listOpt)
	if err != nil {
		return nil, err
	}
	return fips, nil
}

func (ci *crdIpam) createFloatingIP(allocated *FloatingIP) error {
	glog.V(4).Infof("create floatingIP %v", *allocated)
	fip := ci.newFIPCrd(allocated.IP.String())
	if err := assign(fip, allocated); err != nil {
		return err
	}
	if _, err := ci.client.GalaxyV1alpha1().FloatingIPs().Create(fip); err != nil {
		return err
	}
	return nil
}

func (ci *crdIpam) deleteFloatingIP(name string) error {
	glog.V(4).Infof("delete floatingIP name %s", name)
	return ci.client.GalaxyV1alpha1().FloatingIPs().Delete(name, &metav1.DeleteOptions{})
}

func (ci *crdIpam) updateFloatingIP(toUpdate *FloatingIP) error {
	glog.V(4).Infof("update floatingIP %v", *toUpdate)
	fip, err := ci.client.GalaxyV1alpha1().FloatingIPs().Get(toUpdate.IP.String(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := assign(fip, toUpdate); err != nil {
		return err
	}
	_, err = ci.client.GalaxyV1alpha1().FloatingIPs().Update(fip)
	return err
}

func assign(spec *v1alpha1.FloatingIP, f *FloatingIP) error {
	spec.Spec.Key = f.Key
	spec.Spec.Policy = constant.ReleasePolicy(f.Policy)
	data, err := json.Marshal(Attr{
		NodeName: f.NodeName,
		Uid:      f.PodUid,
	})
	if err != nil {
		return err
	}
	spec.Spec.Attribute = string(data)
	spec.Spec.UpdateTime = metav1.NewTime(f.UpdatedAt)
	return nil
}

// handleFIPAssign handles add event for manually created reserved ips
func (ci *crdIpam) handleFIPAssign(obj interface{}) error {
	fip, err := checkForReserved(obj)
	if err != nil {
		return err
	}
	if fip == nil {
		return nil
	}
	ipStr := fip.Name
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	if val, ok := ci.allocatedFIPs[ipStr]; ok {
		return fmt.Errorf("%s already been allocated to %s", ipStr, val.Key)
	}
	unallocated, ok := ci.unallocatedFIPs[ipStr]
	if !ok {
		return fmt.Errorf("there is no ip %s in unallocated map", ipStr)
	}
	unallocated.Assign(fip.Spec.Key, &Attr{Policy: fip.Spec.Policy}, time.Now())
	unallocated.Labels = map[string]string{constant.ReserveFIPLabel: ""}
	ci.syncCacheAfterCreate(unallocated)
	glog.Infof("reserved ip %s", ipStr)
	return nil
}

// handleFIPUnassign handles delete event for manually created reserved ips
func (ci *crdIpam) handleFIPUnassign(obj interface{}) error {
	fip, err := checkForReserved(obj)
	if err != nil {
		return err
	}
	if fip == nil {
		return nil
	}
	ipStr := fip.Name
	ci.cacheLock.Lock()
	defer ci.cacheLock.Unlock()
	allocated, ok := ci.allocatedFIPs[ipStr]
	if !ok {
		return fmt.Errorf("%s already been released", ipStr)
	}
	ci.syncCacheAfterDel(allocated)
	glog.Infof("released reserved ip %s", ipStr)
	return nil
}

func checkForReserved(obj interface{}) (*v1alpha1.FloatingIP, error) {
	fip, ok := obj.(*v1alpha1.FloatingIP)
	if !ok {
		return nil, nil
	}
	if _, ok := fip.Labels[constant.ReserveFIPLabel]; !ok {
		return nil, nil
	}
	return fip, nil
}

func (ci *crdIpam) newFIPCrd(name string) *v1alpha1.FloatingIP {
	ipType, _ := ci.ipType.String()
	crd := newFIPCrd(name)
	crd.Labels[constant.IpType] = ipType
	return crd
}

func newFIPCrd(name string) *v1alpha1.FloatingIP {
	return &v1alpha1.FloatingIP{
		TypeMeta:   metav1.TypeMeta{Kind: constant.ResourceKind, APIVersion: constant.ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}},
	}
}
