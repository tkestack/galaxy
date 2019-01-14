package masq

import (
	"bytes"
	"testing"

	utiliptables "git.code.oa.com/gaiastack/galaxy/pkg/utils/iptables"
	iptablesTest "git.code.oa.com/gaiastack/galaxy/pkg/utils/iptables/testing"
)

func TestEnsureIPMasq(t *testing.T) {
	fakeCli := iptablesTest.NewFakeIPTables()
	EnsureIPMasq(fakeCli)()
	buf := bytes.NewBuffer(nil)
	fakeCli.SaveInto(utiliptables.TableNAT, buf)
	expectTxt := `*nat
:INPUT - [0:0]
:IP-MASQ-AGENT - [0:0]
:OUTPUT - [0:0]
:POSTROUTING - [0:0]
:PREROUTING - [0:0]
-A IP-MASQ-AGENT -m comment --comment "ip-masq-agent: local traffic is not subject to MASQUERADE" -d 169.254.0.0/16 -j RETURN
-A IP-MASQ-AGENT -m comment --comment "ip-masq-agent: local traffic is not subject to MASQUERADE" -d 10.0.0.0/8 -j RETURN
-A IP-MASQ-AGENT -m comment --comment "ip-masq-agent: local traffic is not subject to MASQUERADE" -d 172.16.0.0/12 -j RETURN
-A IP-MASQ-AGENT -m comment --comment "ip-masq-agent: local traffic is not subject to MASQUERADE" -d 192.168.0.0/16 -j RETURN
-A IP-MASQ-AGENT -m comment --comment "ip-masq-agent: outbound traffic is subject to MASQUERADE (must be last in chain)" -j MASQUERADE
-A POSTROUTING -m comment --comment "ip-masq-agent: ensure nat POSTROUTING directs all non-LOCAL destination traffic to our custom IP-MASQ-AGENT chain" -m addrtype ! --dst-type LOCAL -j IP-MASQ-AGENT
COMMIT
`
	if buf.String() != expectTxt {
		t.Errorf("expect %s, real %s", expectTxt, buf.String())
	}
	fakeCli.DeleteRule(utiliptables.TableNAT, masqChain, masqRuleComment, "-j", "MASQUERADE")
	fakeCli.DeleteRule(utiliptables.TableNAT, masqChain, nonMasqRuleComment, "-d", "10.0.0.0/8", "-j", "RETURN")
	EnsureIPMasq(fakeCli)()
	buf.Reset()
	fakeCli.SaveInto(utiliptables.TableNAT, buf)
	if buf.String() != expectTxt {
		t.Errorf("expect %s, real %s", expectTxt, buf.String())
	}
}
