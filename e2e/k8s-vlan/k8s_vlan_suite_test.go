package k8s_vlan_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestK8sVlan(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8sVlan Suite")
}
