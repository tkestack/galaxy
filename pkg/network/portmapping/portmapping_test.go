package portmapping

import (
	"testing"

	"git.code.oa.com/tkestack/galaxy/pkg/api/k8s"
)

func TestOpenRandomPort(t *testing.T) {
	hp := &hostport{protocol: "tcp"}
	closer, err := openLocalPort(hp)
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if hp.port == 0 {
		t.Fatal()
	}
	hp = &hostport{protocol: "udp"}
	closer, err = openLocalPort(hp)
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if hp.port == 0 {
		t.Fatal()
	}
}

func TestOpenHostports(t *testing.T) {
	pm := &PortMappingHandler{
		podPortMap: make(map[string]map[hostport]closeable),
	}
	ports := []k8s.Port{
		{
			ContainerPort: 80,
			Protocol:      "tcp",
		},
		{
			ContainerPort: 53,
			Protocol:      "tcp",
		},
		{
			ContainerPort: 53,
			Protocol:      "udp",
		},
	}
	if err := pm.OpenHostports("pod1_default", false, ports); err != nil {
		t.Fatal(err)
	}
	if len(pm.podPortMap) != 0 {
		t.Fatal("expect not listen random host port")
	}
	if err := pm.OpenHostports("pod1_default", true, ports); err != nil {
		t.Fatal(err)
	}
	if len(pm.podPortMap) != 1 || len(pm.podPortMap["pod1_default"]) != 3 {
		t.Fatal("expect listen 3 socket")
	}
	var firstListen hostport
	for firstListen = range pm.podPortMap["pod1_default"] {
		break
	}
	ports = append(ports, k8s.Port{
		ContainerPort: 81,
		Protocol:      firstListen.protocol,
		HostPort:      firstListen.port,
	})
	if err := pm.OpenHostports("pod2_default", true, ports); err == nil {
		t.Fatal("expect error for existed host port")
	}
	if len(pm.podPortMap) != 1 || len(pm.podPortMap["pod1_default"]) != 3 {
		t.Fatal("expect listen 3 socket for 1 pod")
	}
	pm.CloseHostports("pod1_default")
	if len(pm.podPortMap) != 0 {
		t.Fatal("expect release all listen socket")
	}
}
