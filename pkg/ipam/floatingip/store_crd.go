package floatingip

import (
	"fmt"
	"time"

	"git.code.oa.com/tkestack/galaxy/pkg/api/galaxy/constant"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/apis/galaxy/v1alpha1"
	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (ci *crdIpam) createFloatingIP(name string, key string, policy constant.ReleasePolicy, attr string, subnet string, updateTime time.Time) error {
	glog.V(4).Infof("create floatingIP name %s, key %s, subnet %s, policy %v", name, key, subnet, policy)
	fip := &v1alpha1.FloatingIP{}
	fip.Kind = constant.ResourceKind
	fip.APIVersion = constant.ApiVersion
	fip.Name = name
	fip.Spec.Key = key
	fip.Spec.Policy = policy
	fip.Spec.Attribute = attr
	fip.Spec.Subnet = subnet
	fip.Spec.UpdateTime = metav1.NewTime(updateTime)
	ipTypeVal, err := ci.ipType.String()
	if err != nil {
		return err
	}
	label := make(map[string]string)
	label[constant.IpType] = ipTypeVal
	fip.Labels = label
	if _, err := ci.client.GalaxyV1alpha1().FloatingIPs().Create(fip); err != nil {
		return err
	}
	return nil
}

func (ci *crdIpam) deleteFloatingIP(name string) error {
	glog.V(4).Infof("delete floatingIP name %s", name)
	return ci.client.GalaxyV1alpha1().FloatingIPs().Delete(name, &metav1.DeleteOptions{})
}

func (ci *crdIpam) getFloatingIP(name string) error {
	_, err := ci.client.GalaxyV1alpha1().FloatingIPs().Get(name, metav1.GetOptions{})
	return err
}

func (ci *crdIpam) updateFloatingIP(name, key, subnet string, policy constant.ReleasePolicy, attr string, updateTime time.Time) error {
	glog.V(4).Infof("update floatingIP name %s, key %s, subnet %s, policy %v", name, key, subnet, policy)
	fip, err := ci.client.GalaxyV1alpha1().FloatingIPs().Get(name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	fip.Spec.Key = key
	fip.Spec.Policy = policy
	fip.Spec.Subnet = subnet
	fip.Spec.Attribute = attr
	fip.Spec.UpdateTime = metav1.NewTime(updateTime)
	_, err = ci.client.GalaxyV1alpha1().FloatingIPs().Update(fip)
	return err
}
