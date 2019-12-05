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
package portmapping

import (
	"bytes"
	"testing"

	"tkestack.io/galaxy/pkg/api/k8s"
	utiliptables "tkestack.io/galaxy/pkg/utils/iptables"
	iptablesTest "tkestack.io/galaxy/pkg/utils/iptables/testing"
)

func TestHostportChainName(t *testing.T) {
	m := make(map[string]int)
	chain := hostportChainName(k8s.Port{PodName: "testrdma-2", HostPort: 57119, Protocol: "TCP", ContainerPort: 30008}, "testrdma-2")
	m[string(chain)] = 1
	chain = hostportChainName(k8s.Port{PodName: "testrdma-2", HostPort: 55429, Protocol: "TCP", ContainerPort: 30001}, "testrdma-2")
	m[string(chain)] = 1
	chain = hostportChainName(k8s.Port{PodName: "testrdma-2", HostPort: 56833, Protocol: "TCP", ContainerPort: 30004}, "testrdma-2")
	m[string(chain)] = 1
	if len(m) != 3 {
		t.Fatal(m)
	}
}

func TestEnsureBasicRule(t *testing.T) {
	fakeCli := iptablesTest.NewFakeIPTables()
	h := &PortMappingHandler{
		Interface:        fakeCli,
		podPortMap:       make(map[string]map[hostport]closeable),
		natInterfaceName: "",
	}
	if err := h.EnsureBasicRule(); err != nil {
		t.Fatal(err)
	}
	buf := bytes.NewBuffer(nil)
	fakeCli.SaveInto(utiliptables.TableNAT, buf)
	expectTxt := `*nat
:INPUT - [0:0]
:KUBE-HOSTPORTS - [0:0]
:OUTPUT - [0:0]
:POSTROUTING - [0:0]
:PREROUTING - [0:0]
-A OUTPUT -m comment --comment "kube hostport portals" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS
-A PREROUTING -m comment --comment "kube hostport portals" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS
COMMIT
`
	if buf.String() != expectTxt {
		t.Errorf("expect %s, real %s", expectTxt, buf.String())
	}

	h.natInterfaceName = "test0"
	if err := h.EnsureBasicRule(); err != nil {
		t.Fatal(err)
	}
	buf = bytes.NewBuffer(nil)
	fakeCli.SaveInto(utiliptables.TableNAT, buf)
	expectTxt = `*nat
:INPUT - [0:0]
:KUBE-HOSTPORTS - [0:0]
:OUTPUT - [0:0]
:POSTROUTING - [0:0]
:PREROUTING - [0:0]
-A OUTPUT -m comment --comment "kube hostport portals" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS
-A POSTROUTING -m comment --comment "SNAT for localhost access to hostports" -o test0 -s 127.0.0.0/8 -j MASQUERADE
-A PREROUTING -m comment --comment "kube hostport portals" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS
COMMIT
`
	if buf.String() != expectTxt {
		t.Errorf("expect %s, real %s", expectTxt, buf.String())
	}
}

func TestSetupAndCleanPortMapping(t *testing.T) {
	fakeCli := iptablesTest.NewFakeIPTables()
	h := &PortMappingHandler{
		Interface:        fakeCli,
		podPortMap:       make(map[string]map[hostport]closeable),
		natInterfaceName: "test0",
	}
	if err := h.SetupPortMapping([]k8s.Port{
		{PodName: "testrdma-2", HostPort: 57119, Protocol: "TCP", ContainerPort: 30008, PodIP: "192.168.0.1"},
		{PodName: "pod-2", HostPort: 9090, Protocol: "UDP", ContainerPort: 9090, PodIP: "192.168.0.2"},
	}); err != nil {
		t.Fatal(err)
	}
	buf := bytes.NewBuffer(nil)
	fakeCli.SaveInto(utiliptables.TableNAT, buf)
	expectTxt := `*nat
:INPUT - [0:0]
:KUBE-HOSTPORTS - [0:0]
:KUBE-HP-5MLSI4DJJZLGHUZA - [0:0]
:KUBE-HP-BF3WJKNWB2BP2PEW - [0:0]
:KUBE-MARK-MASQ - [0:0]
:OUTPUT - [0:0]
:POSTROUTING - [0:0]
:PREROUTING - [0:0]
-A KUBE-HOSTPORTS -m comment --comment "testrdma-2 hostport 57119" -m tcp -p tcp --dport 57119 -j KUBE-HP-BF3WJKNWB2BP2PEW
-A KUBE-HOSTPORTS -m comment --comment "pod-2 hostport 9090" -m udp -p udp --dport 9090 -j KUBE-HP-5MLSI4DJJZLGHUZA
-A KUBE-HP-5MLSI4DJJZLGHUZA -m comment --comment "pod-2 hostport 9090" -s 192.168.0.2/32 -j KUBE-MARK-MASQ
-A KUBE-HP-5MLSI4DJJZLGHUZA -m comment --comment "pod-2 hostport 9090" -m udp -p udp -j DNAT --to-destination 192.168.0.2:9090
-A KUBE-HP-BF3WJKNWB2BP2PEW -m comment --comment "testrdma-2 hostport 57119" -s 192.168.0.1/32 -j KUBE-MARK-MASQ
-A KUBE-HP-BF3WJKNWB2BP2PEW -m comment --comment "testrdma-2 hostport 57119" -m tcp -p tcp -j DNAT --to-destination 192.168.0.1:30008
-A KUBE-MARK-MASQ -j MARK --set-xmark 0x4000/0x4000
COMMIT
`
	if buf.String() != expectTxt {
		t.Errorf("expect %s, real %s", expectTxt, buf.String())
	}

	h.CleanPortMapping([]k8s.Port{
		{PodName: "testrdma-2", HostPort: 57119, Protocol: "TCP", ContainerPort: 30008, PodIP: "192.168.0.1"},
	})
	buf = bytes.NewBuffer(nil)
	fakeCli.SaveInto(utiliptables.TableNAT, buf)
	expectTxt = `*nat
:INPUT - [0:0]
:KUBE-HOSTPORTS - [0:0]
:KUBE-HP-5MLSI4DJJZLGHUZA - [0:0]
:KUBE-MARK-MASQ - [0:0]
:OUTPUT - [0:0]
:POSTROUTING - [0:0]
:PREROUTING - [0:0]
-A KUBE-HOSTPORTS -m comment --comment "pod-2 hostport 9090" -m udp -p udp --dport 9090 -j KUBE-HP-5MLSI4DJJZLGHUZA
-A KUBE-HP-5MLSI4DJJZLGHUZA -m comment --comment "pod-2 hostport 9090" -s 192.168.0.2/32 -j KUBE-MARK-MASQ
-A KUBE-HP-5MLSI4DJJZLGHUZA -m comment --comment "pod-2 hostport 9090" -m udp -p udp -j DNAT --to-destination 192.168.0.2:9090
-A KUBE-MARK-MASQ -j MARK --set-xmark 0x4000/0x4000
COMMIT
`
	if buf.String() != expectTxt {
		t.Errorf("expect %s, real %s", expectTxt, buf.String())
	}
}

func TestSetupPortMappingForAllPods(t *testing.T) {
	// test SetupPortMappingForAllPods cleans outdated rules
	fakeCli := iptablesTest.NewFakeIPTables()
	h := &PortMappingHandler{
		Interface:        fakeCli,
		podPortMap:       make(map[string]map[hostport]closeable),
		natInterfaceName: "test0",
	}
	if err := h.SetupPortMapping([]k8s.Port{
		{PodName: "deletedpod-1", HostPort: 80, Protocol: "TCP", ContainerPort: 80, PodIP: "192.168.0.3"},
	}); err != nil {
		t.Fatal(err)
	}
	buf := bytes.NewBuffer(nil)
	fakeCli.SaveInto(utiliptables.TableNAT, buf)
	expectTxt := `*nat
:INPUT - [0:0]
:KUBE-HOSTPORTS - [0:0]
:KUBE-HP-HSO4NMZ7BUPOGJTD - [0:0]
:KUBE-MARK-MASQ - [0:0]
:OUTPUT - [0:0]
:POSTROUTING - [0:0]
:PREROUTING - [0:0]
-A KUBE-HOSTPORTS -m comment --comment "deletedpod-1 hostport 80" -m tcp -p tcp --dport 80 -j KUBE-HP-HSO4NMZ7BUPOGJTD
-A KUBE-HP-HSO4NMZ7BUPOGJTD -m comment --comment "deletedpod-1 hostport 80" -s 192.168.0.3/32 -j KUBE-MARK-MASQ
-A KUBE-HP-HSO4NMZ7BUPOGJTD -m comment --comment "deletedpod-1 hostport 80" -m tcp -p tcp -j DNAT --to-destination 192.168.0.3:80
-A KUBE-MARK-MASQ -j MARK --set-xmark 0x4000/0x4000
COMMIT
`
	if buf.String() != expectTxt {
		t.Errorf("expect %s, real %s", expectTxt, buf.String())
	}

	if err := h.SetupPortMappingForAllPods([]k8s.Port{
		{PodName: "testrdma-2", HostPort: 57119, Protocol: "TCP", ContainerPort: 30008, PodIP: "192.168.0.1"},
		{PodName: "pod-2", HostPort: 9090, Protocol: "UDP", ContainerPort: 9090, PodIP: "192.168.0.2"},
	}); err != nil {
		t.Fatal(err)
	}
	buf = bytes.NewBuffer(nil)
	fakeCli.SaveInto(utiliptables.TableNAT, buf)
	expectTxt = `*nat
:INPUT - [0:0]
:KUBE-HOSTPORTS - [0:0]
:KUBE-HP-5MLSI4DJJZLGHUZA - [0:0]
:KUBE-HP-BF3WJKNWB2BP2PEW - [0:0]
:KUBE-MARK-MASQ - [0:0]
:OUTPUT - [0:0]
:POSTROUTING - [0:0]
:PREROUTING - [0:0]
-A KUBE-HOSTPORTS -m comment --comment "testrdma-2 hostport 57119" -m tcp -p tcp --dport 57119 -j KUBE-HP-BF3WJKNWB2BP2PEW
-A KUBE-HOSTPORTS -m comment --comment "pod-2 hostport 9090" -m udp -p udp --dport 9090 -j KUBE-HP-5MLSI4DJJZLGHUZA
-A KUBE-HP-5MLSI4DJJZLGHUZA -m comment --comment "pod-2 hostport 9090" -s 192.168.0.2/32 -j KUBE-MARK-MASQ
-A KUBE-HP-5MLSI4DJJZLGHUZA -m comment --comment "pod-2 hostport 9090" -m udp -p udp -j DNAT --to-destination 192.168.0.2:9090
-A KUBE-HP-BF3WJKNWB2BP2PEW -m comment --comment "testrdma-2 hostport 57119" -s 192.168.0.1/32 -j KUBE-MARK-MASQ
-A KUBE-HP-BF3WJKNWB2BP2PEW -m comment --comment "testrdma-2 hostport 57119" -m tcp -p tcp -j DNAT --to-destination 192.168.0.1:30008
-A KUBE-MARK-MASQ -j MARK --set-xmark 0x4000/0x4000
-A OUTPUT -m comment --comment "kube hostport portals" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS
-A POSTROUTING -m comment --comment "SNAT for localhost access to hostports" -o test0 -s 127.0.0.0/8 -j MASQUERADE
-A PREROUTING -m comment --comment "kube hostport portals" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS
COMMIT
`
	if buf.String() != expectTxt {
		t.Errorf("expect %s, real %s", expectTxt, buf.String())
	}
}
