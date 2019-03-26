package floatingip

import (
	"fmt"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/constant"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/apis/floatip/v1alpha1"
	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (ci *crdIpam) listFloatIPs() (*v1alpha1.FloatIpList, error) {
	val, err := ci.ipType.String()
	if err != nil {
		return nil, err
	}
	listOpt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constant.IpType, val),
	}
	fips, err := ci.client.GalaxyV1alpha1().FloatIps(constant.NameSpace).List(listOpt)
	if err != nil {
		return nil, err
	}
	return fips, nil
}

func (ci *crdIpam) createFloatIP(name string, key string, policy constant.ReleasePolicy, attr string, subnet string) error {
	glog.V(4).Infof("create floatIP name %s, key %s, subnet %s, policy %v", name, key, subnet, policy)
	fip := &v1alpha1.FloatIp{}
	fip.Kind = constant.ResourceKind
	fip.APIVersion = constant.ApiVersion
	fip.Name = name
	fip.Spec.Key = key
	fip.Spec.Policy = policy
	fip.Spec.Attribute = attr
	fip.Spec.Subnet = subnet
	ipTypeVal, err := ci.ipType.String()
	if err != nil {
		return err
	}
	label := make(map[string]string)
	label[constant.IpType] = ipTypeVal
	fip.Labels = label
	if _, err := ci.client.GalaxyV1alpha1().FloatIps(constant.NameSpace).Create(fip); err != nil {
		return err
	}
	return nil
}

func (ci *crdIpam) deleteFloatIP(ip string) error {
	glog.V(4).Infof("delete floatIP name %s", ip)
	return ci.client.GalaxyV1alpha1().FloatIps(constant.NameSpace).Delete(ip, &metav1.DeleteOptions{})
}

func (ci *crdIpam) getFloatIP(name string) error {
	_, err := ci.client.GalaxyV1alpha1().FloatIps(constant.NameSpace).Get(name, metav1.GetOptions{})
	return err
}

func (ci *crdIpam) updateFloatIP(name, key, subnet string, policy constant.ReleasePolicy, attr string) error {
	glog.V(4).Infof("update floatIP name %s, key %s, subnet %s, policy %v", name, key, subnet, policy)
	fip, err := ci.client.GalaxyV1alpha1().FloatIps(constant.NameSpace).Get(name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	fip.Spec.Key = key
	fip.Spec.Policy = policy
	fip.Spec.Subnet = subnet
	fip.Spec.Attribute = attr
	_, err = ci.client.GalaxyV1alpha1().FloatIps(constant.NameSpace).Update(fip)
	return err
}
