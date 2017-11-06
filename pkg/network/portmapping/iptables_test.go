package portmapping

import (
	"testing"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
)

func TestHostportChainName(t *testing.T) {
	m := make(map[string]int)
	chain := hostportChainName(&k8s.Port{PodName: "testrdma-2", HostPort: 57119, Protocol: "TCP", ContainerPort: 30008}, "testrdma-2")
	m[string(chain)] = 1
	chain = hostportChainName(&k8s.Port{PodName: "testrdma-2", HostPort: 55429, Protocol: "TCP", ContainerPort: 30001}, "testrdma-2")
	m[string(chain)] = 1
	chain = hostportChainName(&k8s.Port{PodName: "testrdma-2", HostPort: 56833, Protocol: "TCP", ContainerPort: 30004}, "testrdma-2")
	m[string(chain)] = 1
	if len(m) != 3 {
		t.Fatal(m)
	}
}
