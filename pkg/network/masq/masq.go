package masq

import (
	"bytes"
	"strings"

	utildbus "git.code.oa.com/gaiastack/galaxy/pkg/utils/dbus"
	utiliptables "git.code.oa.com/gaiastack/galaxy/pkg/utils/iptables"

	"github.com/golang/glog"
	utilexec "k8s.io/utils/exec"
)

var masqChain = utiliptables.Chain("IP-MASQ-AGENT")

func EnsureIPMasq() {
	iptablesCli := utiliptables.New(utilexec.New(), utildbus.New(), utiliptables.ProtocolIpv4)
	iptablesCli.EnsureChain(utiliptables.TableNAT, masqChain)
	comment := "ip-masq-agent: ensure nat POSTROUTING directs all non-LOCAL destination traffic to our custom IP-MASQ-AGENT chain"
	if _, err := iptablesCli.EnsureRule(utiliptables.Append, utiliptables.TableNAT, utiliptables.ChainPostrouting,
		"-m", "comment", "--comment", comment,
		"-m", "addrtype", "!", "--dst-type", "LOCAL", "-j", string(masqChain)); err != nil {
		glog.Errorf("failed to ensure that %s chain %s jumps to %s: %v", utiliptables.TableNAT, utiliptables.ChainPostrouting, masqChain, err)
		return
	}
	lines := bytes.NewBuffer(nil)
	writeLine(lines, "*nat")
	writeLine(lines, utiliptables.MakeChainLine(masqChain))
	for _, cidr := range []string{"169.254.0.0/16", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		writeNonMasqRule(lines, cidr)
	}
	// masquerade all other traffic that is not bound for a --dst-type LOCAL destination
	writeMasqRule(lines)
	writeLine(lines, "COMMIT")
	if err := iptablesCli.RestoreAll(lines.Bytes(), utiliptables.NoFlushTables, utiliptables.NoRestoreCounters); err != nil {
		glog.Errorf("commit IP-MASQ-AGENT rules failed: %s", err.Error())
	}
}

// code from ip-masq-agent
const nonMasqRuleComment = `-m comment --comment "ip-masq-agent: local traffic is not subject to MASQUERADE"`

func writeNonMasqRule(lines *bytes.Buffer, cidr string) {
	writeRule(lines, utiliptables.Append, masqChain, nonMasqRuleComment, "-d", cidr, "-j", "RETURN")
}

const masqRuleComment = `-m comment --comment "ip-masq-agent: outbound traffic is subject to MASQUERADE (must be last in chain)"`

func writeMasqRule(lines *bytes.Buffer) {
	writeRule(lines, utiliptables.Append, masqChain, masqRuleComment, "-j", "MASQUERADE")
}

// Similar syntax to utiliptables.Interface.EnsureRule, except you don't pass a table
// (you must write these rules under the line with the table name)
func writeRule(lines *bytes.Buffer, position utiliptables.RulePosition, chain utiliptables.Chain, args ...string) {
	fullArgs := append([]string{string(position), string(chain)}, args...)
	writeLine(lines, fullArgs...)
}

// Join all words with spaces, terminate with newline and write to buf.
func writeLine(lines *bytes.Buffer, words ...string) {
	lines.WriteString(strings.Join(words, " ") + "\n")
}
