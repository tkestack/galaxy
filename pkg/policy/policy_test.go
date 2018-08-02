package policy

import (
	"bytes"
	"testing"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/ipset"
	ipsetTest "git.code.oa.com/gaiastack/galaxy/pkg/utils/ipset/testing"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/iptables"
	iptablesTest "git.code.oa.com/gaiastack/galaxy/pkg/utils/iptables/testing"
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func newTestPolicyManager() *PolicyManager {
	return &PolicyManager{
		ipsetHandle:   ipsetTest.NewFake(""),
		iptableHandle: iptablesTest.NewFakeIPTables(),
		hostName:      k8s.GetHostname(),
		quitChan:      make(chan struct{}),
	}
}

func TestSyncRules(t *testing.T) {
	pm := newTestPolicyManager()
	policies := []policy{
		{
			ingressRule: &ingressRule{
				srcRules: []rule{
					{
						ipTable: &ipsetTable{
							IPSet: ipset.IPSet{Name: "GLX-sip-0-XX0", SetType: ipset.HashIP},
							entries: []ipset.Entry{
								{IP: "1.0.0.1", SetType: ipset.HashIP}, {IP: "1.0.0.2", SetType: ipset.HashIP},
							}},
					},
				},
				dstIPTable: &ipsetTable{
					IPSet:   ipset.IPSet{Name: "GLX-ip-XX1", SetType: ipset.HashIP},
					entries: []ipset.Entry{{IP: "1.0.0.3", SetType: ipset.HashIP}}},
			},
			np: &networkv1.NetworkPolicy{ObjectMeta: v1.ObjectMeta{Name: "test1", Namespace: "ns1"}},
		},
		{
			ingressRule: &ingressRule{
				srcRules: []rule{
					{
						ipTable: &ipsetTable{
							IPSet: ipset.IPSet{Name: "GLX-sip-0-XX2", SetType: ipset.HashIP},
							entries: []ipset.Entry{
								{IP: "1.1.0.1", SetType: ipset.HashIP}, {IP: "1.1.0.2", SetType: ipset.HashIP},
							},
						},
						netTable: &ipsetTable{
							IPSet: ipset.IPSet{Name: "GLX-snet-1-XX3", SetType: ipset.HashNet},
							entries: []ipset.Entry{
								{Net: "2.1.0.0/24", SetType: ipset.HashNet}, {Net: "2.1.0.2/32", SetType: ipset.HashNet, Options: []string{"nomatch"}}, {Net: "2.1.1.2/32", SetType: ipset.HashNet},
							},
						},
						tcpPorts: []string{"53", "80"},
						udpPorts: []string{"53", "80"},
					},
				},
				dstIPTable: &ipsetTable{
					IPSet:   ipset.IPSet{Name: "GLX-ip-XX4", SetType: ipset.HashIP},
					entries: []ipset.Entry{{IP: "1.1.0.3", SetType: ipset.HashIP}}},
			},
			egressRule: &egressRule{
				dstRules: []rule{
					{
						ipTable: &ipsetTable{
							IPSet: ipset.IPSet{Name: "GLX-dip-0-XX5", SetType: ipset.HashIP},
							entries: []ipset.Entry{
								{IP: "3.1.0.1", SetType: ipset.HashIP}, {IP: "3.1.0.2", SetType: ipset.HashIP},
							},
						},
						netTable: &ipsetTable{
							IPSet: ipset.IPSet{Name: "GLX-dnet-1-XX6", SetType: ipset.HashNet},
							entries: []ipset.Entry{
								{Net: "3.1.0.0/24", SetType: ipset.HashNet}, {Net: "3.1.0.2/32", SetType: ipset.HashNet, Options: []string{"nomatch"}}, {Net: "3.1.1.2/32", SetType: ipset.HashNet},
							},
						},
						tcpPorts: []string{"53", "80"},
						udpPorts: []string{"53", "80"},
					},
				},
				srcIPTable: &ipsetTable{
					IPSet:   ipset.IPSet{Name: "GLX-ip-XX7", SetType: ipset.HashIP},
					entries: []ipset.Entry{{IP: "1.1.0.3", SetType: ipset.HashIP}}},
			},
			np: &networkv1.NetworkPolicy{ObjectMeta: v1.ObjectMeta{Name: "test2", Namespace: "ns2"}},
		},
	}
	if err := pm.syncRules(policies); err != nil {
		t.Fatal(err)
	}
	buf := bytes.NewBuffer(nil)
	if err := pm.iptableHandle.SaveInto(iptables.TableFilter, buf); err != nil {
		t.Fatal(err)
	}
	expectIPtables := `*filter
:FORWARD - [0:0]
:GLX-PLCY-352SVDAM4WOU2SIO - [0:0]
:GLX-PLCY-Q6GMFAO3AMRGLUBA - [0:0]
:INPUT - [0:0]
:OUTPUT - [0:0]
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p tcp -m set --match-set GLX-sip-0-XX2 src -m set --match-set GLX-ip-XX4 dst -m multiport --dports 53,80 -j ACCEPT
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p udp -m set --match-set GLX-sip-0-XX2 src -m set --match-set GLX-ip-XX4 dst -m multiport --dports 53,80 -j ACCEPT
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p tcp -m set --match-set GLX-snet-1-XX3 src -m set --match-set GLX-ip-XX4 dst -m multiport --dports 53,80 -j ACCEPT
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p udp -m set --match-set GLX-snet-1-XX3 src -m set --match-set GLX-ip-XX4 dst -m multiport --dports 53,80 -j ACCEPT
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p tcp -m set --match-set GLX-ip-XX7 src -m set --match-set GLX-dip-0-XX5 dst -m multiport --dports 53,80 -j ACCEPT
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p udp -m set --match-set GLX-ip-XX7 src -m set --match-set GLX-dip-0-XX5 dst -m multiport --dports 53,80 -j ACCEPT
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p tcp -m set --match-set GLX-ip-XX7 src -m set --match-set GLX-dnet-1-XX6 dst -m multiport --dports 53,80 -j ACCEPT
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p udp -m set --match-set GLX-ip-XX7 src -m set --match-set GLX-dnet-1-XX6 dst -m multiport --dports 53,80 -j ACCEPT
-A GLX-PLCY-Q6GMFAO3AMRGLUBA -m comment --comment test1_ns1 -p all -m set --match-set GLX-sip-0-XX0 src -m set --match-set GLX-ip-XX1 dst -j ACCEPT
COMMIT
`
	if buf.String() != expectIPtables {
		t.Errorf("expect %s, real %s", expectIPtables, buf.String())
	}
	data, err := pm.ipsetHandle.SaveAllSets()
	if err != nil {
		t.Fatal(err)
	}

	expectIPSets := `Name: GLX-dip-0-XX5
Type: hash:ip
Members:
3.1.0.1
3.1.0.2

Name: GLX-dnet-1-XX6
Type: hash:net
Members:
3.1.0.0/24
3.1.0.2/32 nomatch
3.1.1.2/32

Name: GLX-ip-XX1
Type: hash:ip
Members:
1.0.0.3

Name: GLX-ip-XX4
Type: hash:ip
Members:
1.1.0.3

Name: GLX-ip-XX7
Type: hash:ip
Members:
1.1.0.3

Name: GLX-sip-0-XX0
Type: hash:ip
Members:
1.0.0.1
1.0.0.2

Name: GLX-sip-0-XX2
Type: hash:ip
Members:
1.1.0.1
1.1.0.2

Name: GLX-snet-1-XX3
Type: hash:net
Members:
2.1.0.0/24
2.1.0.2/32 nomatch
2.1.1.2/32
`
	if string(data) != expectIPSets {
		t.Errorf("expect %d %s, real %d %s", len(expectIPSets), expectIPSets, len(string(data)), string(data))
	}
}

func TestSyncPodChains(t *testing.T) {
	pm := newTestPolicyManager()
	port80 := intstr.FromInt(80)
	port8080 := intstr.FromInt(8080)
	udpProtocol := corev1.Protocol("UDP")
	selectorMap := map[string]string{"app": "hello"}
	policies := []policy{
		{
			ingressRule: &ingressRule{
				srcRules: []rule{
					{
						ipTable: &ipsetTable{
							IPSet: ipset.IPSet{Name: "GLX-sip-0-XX2", SetType: ipset.HashIP},
							entries: []ipset.Entry{
								{IP: "1.1.0.1", SetType: ipset.HashIP}, {IP: "1.1.0.2", SetType: ipset.HashIP},
							},
						},
						netTable: &ipsetTable{
							IPSet: ipset.IPSet{Name: "GLX-snet-1-XX3", SetType: ipset.HashNet},
							entries: []ipset.Entry{
								{Net: "2.1.0.0/24", SetType: ipset.HashNet}, {Net: "2.1.0.2/32", SetType: ipset.HashNet, Options: []string{"nomatch"}}, {Net: "2.1.1.2/32", SetType: ipset.HashNet},
							},
						},
						tcpPorts: []string{"80"},
					},
				},
				dstIPTable: &ipsetTable{
					IPSet:   ipset.IPSet{Name: "GLX-ip-XX4", SetType: ipset.HashIP},
					entries: []ipset.Entry{{IP: "1.1.0.3", SetType: ipset.HashIP}}},
			},
			egressRule: &egressRule{
				dstRules: []rule{
					{
						netTable: &ipsetTable{
							IPSet: ipset.IPSet{Name: "GLX-dnet-1-XX6", SetType: ipset.HashNet},
							entries: []ipset.Entry{
								{Net: "3.1.0.0/24", SetType: ipset.HashNet}, {Net: "3.1.0.2/32", SetType: ipset.HashNet, Options: []string{"nomatch"}},
							},
						},
						udpPorts: []string{"8080"},
					},
				},
				srcIPTable: &ipsetTable{
					IPSet:   ipset.IPSet{Name: "GLX-ip-XX7", SetType: ipset.HashIP},
					entries: []ipset.Entry{{IP: "1.1.0.3", SetType: ipset.HashIP}}},
			},
			np: &networkv1.NetworkPolicy{
				ObjectMeta: v1.ObjectMeta{Name: "test2", Namespace: "ns2"},
				Spec: networkv1.NetworkPolicySpec{
					PodSelector: v1.LabelSelector{MatchLabels: selectorMap},
					Ingress: []networkv1.NetworkPolicyIngressRule{
						{
							From: []networkv1.NetworkPolicyPeer{
								{PodSelector: &v1.LabelSelector{MatchLabels: map[string]string{"app": "test"}}},
								{IPBlock: &networkv1.IPBlock{CIDR: "2.1.0.0/24", Except: []string{"2.1.0.2/32"}}},
							},
							Ports: []networkv1.NetworkPolicyPort{{Port: &port80}},
						},
						{
							From: []networkv1.NetworkPolicyPeer{
								{IPBlock: &networkv1.IPBlock{CIDR: "2.1.1.2/32"}},
							},
							Ports: []networkv1.NetworkPolicyPort{{Port: &port80}},
						},
					},
					Egress: []networkv1.NetworkPolicyEgressRule{
						{
							To: []networkv1.NetworkPolicyPeer{
								{IPBlock: &networkv1.IPBlock{CIDR: "3.1.0.0/24", Except: []string{"3.1.0.2/32"}}},
							},
							Ports: []networkv1.NetworkPolicyPort{{Port: &port8080, Protocol: &udpProtocol}},
						},
					},
				},
			},
		},
	}
	pm.policies = policies
	if err := pm.syncRules(policies); err != nil {
		t.Fatal(err)
	}
	if err := pm.SyncPodChains(&corev1.Pod{
		ObjectMeta: v1.ObjectMeta{Name: "hello", Namespace: "ns2", Labels: selectorMap},
		Status:     corev1.PodStatus{PodIP: "192.168.0.1"},
	}); err != nil {
		t.Fatal(err)
	}
	buf := bytes.NewBuffer(nil)
	if err := pm.iptableHandle.SaveInto(iptables.TableFilter, buf); err != nil {
		t.Fatal(err)
	}
	expectIPtables := `*filter
:FORWARD - [0:0]
:GLX-EGRESS - [0:0]
:GLX-INGRESS - [0:0]
:GLX-PLCY-352SVDAM4WOU2SIO - [0:0]
:GLX-POD-BLFOGEWPTSIKACFR - [0:0]
:INPUT - [0:0]
:OUTPUT - [0:0]
-A FORWARD -j GLX-EGRESS
-A FORWARD -j GLX-INGRESS
-A GLX-EGRESS -s 192.168.0.1/32 -m comment --comment hello_ns2 -j GLX-POD-BLFOGEWPTSIKACFR
-A GLX-INGRESS -d 192.168.0.1/32 -m comment --comment hello_ns2 -j GLX-POD-BLFOGEWPTSIKACFR
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p tcp -m set --match-set GLX-sip-0-XX2 src -m set --match-set GLX-ip-XX4 dst -m multiport --dports 80 -j ACCEPT
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p tcp -m set --match-set GLX-snet-1-XX3 src -m set --match-set GLX-ip-XX4 dst -m multiport --dports 80 -j ACCEPT
-A GLX-PLCY-352SVDAM4WOU2SIO -m comment --comment test2_ns2 -p udp -m set --match-set GLX-ip-XX7 src -m set --match-set GLX-dnet-1-XX6 dst -m multiport --dports 8080 -j ACCEPT
-A GLX-POD-BLFOGEWPTSIKACFR -m comment --comment hello_ns2 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
-A GLX-POD-BLFOGEWPTSIKACFR -m comment --comment hello_ns2 -j GLX-PLCY-352SVDAM4WOU2SIO
-A GLX-POD-BLFOGEWPTSIKACFR -m comment --comment hello_ns2 -j DROP
-A INPUT -j GLX-EGRESS
-A OUTPUT -j GLX-INGRESS
COMMIT
`
	if buf.String() != expectIPtables {
		t.Errorf("expect %s, real %s", expectIPtables, buf.String())
	}
	data, err := pm.ipsetHandle.SaveAllSets()
	if err != nil {
		t.Fatal(err)
	}

	expectIPSets := `Name: GLX-dnet-1-XX6
Type: hash:net
Members:
3.1.0.0/24
3.1.0.2/32 nomatch

Name: GLX-ip-XX4
Type: hash:ip
Members:
1.1.0.3

Name: GLX-ip-XX7
Type: hash:ip
Members:
1.1.0.3

Name: GLX-sip-0-XX2
Type: hash:ip
Members:
1.1.0.1
1.1.0.2

Name: GLX-snet-1-XX3
Type: hash:net
Members:
2.1.0.0/24
2.1.0.2/32 nomatch
2.1.1.2/32
`
	if string(data) != expectIPSets {
		t.Errorf("expect %d %s, real %d %s", len(expectIPSets), expectIPSets, len(string(data)), string(data))
	}
}
