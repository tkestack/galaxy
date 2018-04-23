package policy

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"net"
	"strings"
	"sync"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	utildbus "git.code.oa.com/gaiastack/galaxy/pkg/utils/dbus"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/ipset"
	utiliptables "git.code.oa.com/gaiastack/galaxy/pkg/utils/iptables"
	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	utilexec "k8s.io/utils/exec"
)

var (
	NamePrefix        = "GLX"
	policyChainPrefix = NamePrefix + "-PLCY"
	podChainPrefix    = NamePrefix + "-POD"
	ingressChain      = utiliptables.Chain(NamePrefix + "-INGRESS")
	egressChain       = utiliptables.Chain(NamePrefix + "-EGRESS")
	chainNotExistErr  = "No chain/target/match by that name"
)

// PolicyManager implements kubernetes network policy for pods
// iptable ingress chain topology is like
//  FORWARD            GLX-POD-XXXX - GLX-PLCY-XXXX
//        \           /            \ /
//         GLX-INGRESS             /\
//        /           \           /  \
//  OUTPUT             GLX-POD-XXXX - GLX-PLCY-XXXX

// iptable egress chain topology is like
//  FORWARD            GLX-POD-XXXX - GLX-PLCY-XXXX
//        \           /            \ /
//         GLX-EGRESS              /\
//        /           \           /  \
//  INPUT             GLX-POD-XXXX - GLX-PLCY-XXXX
type PolicyManager struct {
	sync.Mutex
	policies      []policy
	client        *kubernetes.Clientset
	ipsetHandle   ipset.Interface
	iptableHandle utiliptables.Interface
	hostName      string
}

func New(client *kubernetes.Clientset) *PolicyManager {
	return &PolicyManager{
		client:        client,
		ipsetHandle:   ipset.New(utilexec.New()),
		iptableHandle: utiliptables.New(utilexec.New(), utildbus.New(), utiliptables.ProtocolIpv4),
		hostName:      k8s.GetHostname(""),
	}
}

func (p *PolicyManager) Run() {
	p.syncNetworkPolices()
	p.syncNetworkPolicyRules()
	p.syncPods()
}

func (p *PolicyManager) syncPods() {
	list, err := p.client.CoreV1().Pods(v1.NamespaceAll).List(v1.ListOptions{FieldSelector: fields.OneTermEqualSelector("spec.nodeName", k8s.GetHostname("")).String()})
	if err != nil {
		glog.Warningf("failed to list pods: %v", err)
		return
	}
	var wg sync.WaitGroup
	for i := range list.Items {
		wg.Add(1)
		go func(pod *corev1.Pod) {
			defer wg.Done()
			if err := p.SyncPodChains(pod); err != nil {
				glog.Warningf("failed to sync pod policy %s_%s: %v", pod.Name, pod.Namespace, err)
			}
		}(&list.Items[i])
	}
	wg.Wait()
}

func (p *PolicyManager) syncNetworkPolices() {
	list, err := p.client.NetworkingV1().NetworkPolicies(v1.NamespaceAll).List(v1.ListOptions{})
	if err != nil {
		glog.Warningf("failed to list network policies: %v", err)
		return
	}
	var (
		policies []policy
	)
	for i := range list.Items {
		ingress, egress, err := p.policyResult(&list.Items[i])
		if err != nil {
			glog.Warning(err)
			continue
		}
		policies = append(policies, policy{ingressRule: ingress, egressRule: egress, np: &list.Items[i]})
	}
	p.Lock()
	p.policies = policies
	p.Unlock()
}

func (p *PolicyManager) syncNetworkPolicyRules() {
	var policies []policy
	p.Lock()
	policies = p.policies
	p.Unlock()
	if err := p.syncRules(policies); err != nil {
		glog.Warningf("failed to sync policy rules: %v", err)
	}
}

type policy struct {
	ingressRule *ingressRule
	egressRule  *egressRule
	np          *networkv1.NetworkPolicy
}

type ingressRule struct {
	srcRules   []rule
	dstIPTable *ipsetTable
}

type egressRule struct {
	dstRules   []rule
	srcIPTable *ipsetTable
}

type rule struct {
	ipTable, netTable *ipsetTable
	tcpPorts          []string
	udpPorts          []string
}

type ipsetTable struct {
	ipset.IPSet
	entries []ipset.Entry
}

func (p *PolicyManager) policyResult(np *networkv1.NetworkPolicy) (*ingressRule, *egressRule, error) {
	tbl, err := p.podSelectorToTable(&np.Spec.PodSelector, np.Namespace)
	if err != nil {
		return nil, nil, err
	}
	npNameHash := tableNameHash(fmt.Sprintf("%s_%s", np.Name, np.Namespace))
	tbl.Name = fmt.Sprintf("%s-ip-%s", NamePrefix, npNameHash)
	var (
		inRules         *ingressRule
		eRules          *egressRule
		ingress, egress bool
	)
	for _, pt := range np.Spec.PolicyTypes {
		if pt == networkv1.PolicyTypeIngress {
			ingress = true
		} else if pt == networkv1.PolicyTypeEgress {
			egress = true
		}
	}
	if !ingress && !egress {
		ingress = true
		egress = len(np.Spec.Egress) > 0
	}
	if ingress {
		inRules = &ingressRule{dstIPTable: tbl}
		for i := range np.Spec.Ingress {
			ir := np.Spec.Ingress[i]
			rule := p.peerRule(ir.Ports, ir.From)
			if rule.ipTable != nil {
				rule.ipTable.Name = fmt.Sprintf("%s-sip-%d-%s", NamePrefix, i, npNameHash)
			}
			if rule.netTable != nil {
				rule.netTable.Name = fmt.Sprintf("%s-snet-%d-%s", NamePrefix, i, npNameHash)
			}
			inRules.srcRules = append(inRules.srcRules, *rule)
		}
	}
	if egress {
		eRules = &egressRule{srcIPTable: tbl}
		for i := range np.Spec.Egress {
			ir := np.Spec.Egress[i]
			rule := p.peerRule(ir.Ports, ir.To)
			if rule.ipTable != nil {
				rule.ipTable.Name = fmt.Sprintf("%s-dip-%d-%s", NamePrefix, i, npNameHash)
			}
			if rule.netTable != nil {
				rule.netTable.Name = fmt.Sprintf("%s-dnet-%d-%s", NamePrefix, i, npNameHash)
			}
			eRules.dstRules = append(eRules.dstRules, *rule)
		}
	}
	return inRules, eRules, nil
}

func (p *PolicyManager) peerRule(ports []networkv1.NetworkPolicyPort, peers []networkv1.NetworkPolicyPeer) *rule {
	tcpPorts, udpPorts := rulePorts(ports)
	rule := rule{tcpPorts: tcpPorts, udpPorts: udpPorts}
	for j := range peers {
		tbl, err := p.peerTable(&peers[j])
		if err != nil {
			glog.Warningf("failed to resolve peer ipset %s, %v", peers[j].String(), err)
			continue
		}
		if tbl.SetType == ipset.HashIP {
			if rule.ipTable == nil {
				rule.ipTable = tbl
			} else {
				rule.ipTable.entries = append(rule.ipTable.entries, tbl.entries...)
			}
		} else if tbl.SetType == ipset.HashNet {
			rule.netTable = tbl
		}
	}
	return &rule
}

func (p *PolicyManager) podSelectorToTable(podSelector *v1.LabelSelector, namespace string) (*ipsetTable, error) {
	//TODO MatchExpressions
	selectorStr := labels.FormatLabels(podSelector.MatchLabels)
	if len(podSelector.MatchLabels) == 0 {
		selectorStr = ""
	}
	list, err := p.client.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: selectorStr})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods by selector %s: %v", selectorStr, err)
	}
	if glog.V(5) {
		var podIPs []string
		for _, pod := range list.Items {
			if pod.Status.PodIP == "" {
				continue
			}
			podIPs = append(podIPs, pod.Status.PodIP)
		}
		glog.V(5).Infof("selectorStr %s pods %s", selectorStr, strings.Join(podIPs, " "))
	}
	return &ipsetTable{IPSet: ipset.IPSet{SetType: ipset.HashIP}, entries: entries(list.Items, ipset.HashIP)}, nil
}

func (p *PolicyManager) namespaceSelectorToTable(namespaceSelector *v1.LabelSelector) (*ipsetTable, error) {
	//TODO MatchExpressions
	namespaces, err := p.getNamespaces(namespaceSelector)
	if err != nil {
		return nil, err
	}
	var pods []corev1.Pod
	for i := range namespaces {
		list, err := p.client.CoreV1().Pods(namespaces[i].Name).List(v1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list pods in namespace %s: %v", namespaces[i].Name, err)
		}
		pods = append(pods, list.Items[i])
	}
	return &ipsetTable{IPSet: ipset.IPSet{SetType: ipset.HashIP}, entries: entries(pods, ipset.HashIP)}, nil
}

func (p *PolicyManager) peerTable(peer *networkv1.NetworkPolicyPeer) (*ipsetTable, error) {
	if peer.PodSelector != nil {
		return p.podSelectorToTable(peer.PodSelector, v1.NamespaceAll)
	}
	if peer.NamespaceSelector != nil {
		return p.namespaceSelectorToTable(peer.NamespaceSelector)
	}
	if peer.IPBlock != nil {
		return ipBlockToTable(peer.IPBlock.CIDR, peer.IPBlock.Except)
	}
	return nil, fmt.Errorf("invalid peer")
}

func ipBlockToTable(cidr string, except []string) (*ipsetTable, error) {
	formatedCidr, err := formatCidr(cidr)
	if err != nil {
		return nil, err
	}
	tbl := &ipsetTable{IPSet: ipset.IPSet{SetType: ipset.HashNet}, entries: []ipset.Entry{{Net: formatedCidr, SetType: ipset.HashNet}}}
	for i := range except {
		formatedExcept, err := formatCidr(except[i])
		if err != nil {
			return nil, err
		}
		tbl.entries = append(tbl.entries, ipset.Entry{Net: formatedExcept, SetType: ipset.HashNet, Options: []string{"nomatch"}})
	}
	return tbl, nil
}

func formatCidr(cidr string) (string, error) {
	// if except is 10.246.33.13/31, we have to mask it, i.e. 10.246.33.12/31
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("failed to parse cidr %s: %v", cidr, err)
	}
	//ipset doesn't print /32 suffix
	//Name: GLX-snet-0-UQ4CNXOEWXNJ42SN
	//Type: hash:net
	//Revision: 6
	//Header: family inet hashsize 1024 maxelem 65536
	//Size in memory: 576
	//References: 2
	//Members:
	//10.246.33.0/24
	//10.246.33.10 nomatch
	//10.246.33.12/31 nomatch
	return strings.TrimSuffix(ipnet.String(), "/32"), nil
}

func entries(pods []corev1.Pod, setType ipset.Type) []ipset.Entry {
	var entries []ipset.Entry
	for i := range pods {
		if pods[i].Status.PodIP == "" {
			continue
		}
		entries = append(entries, ipset.Entry{IP: pods[i].Status.PodIP, SetType: setType})
	}
	return entries
}

func tableNameHash(data string) string {
	hash := sha256.Sum256([]byte(data))
	encoded := base32.StdEncoding.EncodeToString(hash[:])
	return encoded[:16]
}

func rulePorts(npp []networkv1.NetworkPolicyPort) ([]string, []string) {
	var (
		tcpPorts []string
		udpPorts []string
	)
	for j := range npp {
		protocol := "tcp"
		if npp[j].Protocol != nil {
			protocol = strings.ToLower(string(*npp[j].Protocol))
		}
		if npp[j].Port != nil {
			if protocol == "tcp" {
				// if port is a name, we'll reassign the int value when list pods
				//TODO named ports
				tcpPorts = append(tcpPorts, npp[j].Port.String())
			} else {
				udpPorts = append(udpPorts, npp[j].Port.String())
			}
		}
	}
	return tcpPorts, udpPorts
}

// syncRules ensures GLX-sip-xxxx/GLX-snet-xxxx/GLX-dip-xxxx/GLX-dnet-xxxx/GLX-ip-xxxx ipsets including their entries are expected,
// and GLX-PLCY-XXXX iptables chain are expected.
func (p *PolicyManager) syncRules(polices []policy) error {
	// sync ipsets
	ipsets, err := p.ipsetHandle.ListSets()
	if err != nil {
		return fmt.Errorf("failed to list ipsets: %v", err)
	}
	// build new ipset table map
	newIPSetMap := make(map[string]*ipsetTable)
	for _, policy := range polices {
		ingress := policy.ingressRule
		egress := policy.egressRule
		if ingress != nil {
			newIPSetMap[ingress.dstIPTable.Name] = ingress.dstIPTable
			for _, rule := range ingress.srcRules {
				if rule.ipTable != nil {
					newIPSetMap[rule.ipTable.Name] = rule.ipTable
				}
				if rule.netTable != nil {
					newIPSetMap[rule.netTable.Name] = rule.netTable
				}
			}
		}
		if egress != nil {
			newIPSetMap[egress.srcIPTable.Name] = egress.srcIPTable
			for _, rule := range egress.dstRules {
				if rule.ipTable != nil {
					newIPSetMap[rule.ipTable.Name] = rule.ipTable
				}
				if rule.netTable != nil {
					newIPSetMap[rule.netTable.Name] = rule.netTable
				}
			}
		}
	}
	// create ipset
	for name, set := range newIPSetMap {
		if err := p.ipsetHandle.CreateSet(&set.IPSet, true); err != nil {
			return fmt.Errorf("failed to create ipset %s %s: %v", set.Name, string(set.SetType), err)
		}
		oldEntries, err := p.ipsetHandle.ListEntries(name)
		if err != nil {
			glog.Warningf("failed to list entries %s: %v", name, err)
			continue
		}
		oldEntriesSet := sets.NewString(oldEntries...)
		newEntries := sets.NewString()
		for _, entry := range set.entries {
			newEntryStr := strings.Join(append([]string{entry.String()}, entry.Options...), " ")
			newEntries.Insert(newEntryStr)
			if oldEntriesSet.Has(newEntryStr) {
				continue
			}
			if err := p.ipsetHandle.AddEntryWithOptions(&entry, &set.IPSet, true); err != nil {
				glog.Warningf("failed to add entry %v: %v", entry, err)
			}
		}
		glog.V(5).Infof("old entries %s, new entries %s", strings.Join(oldEntries, ","), strings.Join(newEntries.List(), ","))
		// clean up stale entries
		for _, old := range oldEntries {
			if !newEntries.Has(old) {
				parts := strings.Split(old, " ")
				if err := p.ipsetHandle.DelEntryWithOptions(name, parts[0], parts[1:]...); err != nil {
					glog.Warningf("failed to del entry %s from set %s: %v", old, name, err)
				}
			}
		}
	}

	defer func() {
		// clean up stale ipsets after iptables referencing these ipsets are deleted
		for _, name := range ipsets {
			if !strings.HasPrefix(name, NamePrefix) {
				continue
			}
			if _, exist := newIPSetMap[name]; !exist {
				p.ipsetHandle.DestroySet(name)
			}
		}
	}()

	// sync iptables
	iptablesSaveRaw := bytes.NewBuffer(nil)
	// Get iptables-save output so we can check for existing chains and rules.
	// This will be a map of chain name to chain with rules as stored in iptables-save/iptables-restore
	existingChains := make(map[utiliptables.Chain]string)
	if err := p.iptableHandle.SaveInto(utiliptables.TableFilter, iptablesSaveRaw); err != nil {
		return fmt.Errorf("Failed to execute iptables-save, syncing all rules: %v", err)
	} else { // otherwise parse the output
		existingChains = utiliptables.GetChainLines(utiliptables.TableFilter, iptablesSaveRaw.Bytes())
	}
	filterChains := bytes.NewBuffer(nil)
	filterRules := bytes.NewBuffer(nil)
	writeLine(filterChains, "*filter")

	// Accumulate chains to keep.
	activeChains := map[utiliptables.Chain]bool{}
	for _, policy := range polices {
		policyNameComment := fmt.Sprintf("%s_%s", policy.np.Name, policy.np.Namespace)
		policyChain := utiliptables.Chain(policyChainName(policy.np))
		// -N GLX-PLCY-XXXX
		if chain, ok := existingChains[policyChain]; ok {
			writeLine(filterChains, chain)
		} else {
			writeLine(filterChains, utiliptables.MakeChainLine(policyChain))
		}
		activeChains[policyChain] = true
		if policy.ingressRule != nil {
			for _, rule := range policy.ingressRule.srcRules {
				srcTableNames := []string{}
				if rule.ipTable != nil {
					srcTableNames = append(srcTableNames, rule.ipTable.Name)
				}
				if rule.netTable != nil {
					srcTableNames = append(srcTableNames, rule.netTable.Name)
				}
				writePolicyChainRules(filterRules, string(policyChain), policyNameComment, srcTableNames, []string{policy.ingressRule.dstIPTable.Name}, rule.tcpPorts, rule.udpPorts)
			}
		}
		if policy.egressRule != nil {
			for _, rule := range policy.egressRule.dstRules {
				dstTableNames := []string{}
				if rule.ipTable != nil {
					dstTableNames = append(dstTableNames, rule.ipTable.Name)
				}
				if rule.netTable != nil {
					dstTableNames = append(dstTableNames, rule.netTable.Name)
				}
				writePolicyChainRules(filterRules, string(policyChain), policyNameComment, []string{policy.egressRule.srcIPTable.Name}, dstTableNames, rule.tcpPorts, rule.udpPorts)
			}
		}
	}

	// Delete chains no longer in use.
	// TODO fix if any pod reference this plcy chain
	for chain := range existingChains {
		if !activeChains[chain] {
			chainString := string(chain)
			if !strings.HasPrefix(chainString, policyChainPrefix) {
				// Ignore chains that aren't ours.
				continue
			}
			// We must (as per iptables) write a chain-line for it, which has
			// the nice effect of flushing the chain.  Then we can remove the
			// chain.
			writeLine(filterChains, existingChains[chain])
			writeLine(filterRules, "-X", chainString)
		}
	}
	writeLine(filterRules, "COMMIT")

	lines := append(filterChains.Bytes(), filterRules.Bytes()...)
	err = p.iptableHandle.RestoreAll(lines, utiliptables.NoFlushTables, utiliptables.RestoreCounters)
	if err != nil {
		return fmt.Errorf("failed to execute iptables-restore for ruls %s: %v", string(lines), err)
	}
	return nil
}

// The rule maybe
// -A GLX-PLCY-XXXX -m comment --comment "name_namespace -p tcp \
// -m set --match-set GLX-sip-xxxx src \
// -m set --match-set GLX-ip-xxxx dst \
// -m multiport --dports 8080,8081 -j ACCEPT
// If there are both tcp and udp ports, this adds a rule for each protocol
func writePolicyChainRules(filterRules *bytes.Buffer, policyChainName, policyNameComment string, srcTableNames, dstTableNames, tcpPorts, udpPorts []string) {
	for _, srcTableName := range srcTableNames {
		for _, dstTableName := range dstTableNames {
			setRules := []string{"-m", "set", "--match-set", srcTableName, "src", "-m", "set", "--match-set", dstTableName, "dst"}
			if len(tcpPorts) > 0 {
				args := []string{
					"-A", policyChainName,
					"-m", "comment", "--comment", policyNameComment,
					"-p", "tcp",
				}
				args = append(args, setRules...)
				args = append(args, "-m", "multiport", "--dports", strings.Join(tcpPorts, ","))
				args = append(args, "-j", "ACCEPT")
				writeLine(filterRules, args...)
			}
			if len(udpPorts) > 0 {
				args := []string{
					"-A", policyChainName,
					"-m", "comment", "--comment", policyNameComment,
					"-p", "udp",
				}
				args = append(args, setRules...)
				args = append(args, "-m", "multiport", "--dports", strings.Join(udpPorts, ","))
				args = append(args, "-j", "ACCEPT")
				writeLine(filterRules, args...)
			}
			if len(tcpPorts) == 0 && len(udpPorts) == 0 {
				args := []string{
					"-A", policyChainName,
					"-m", "comment", "--comment", policyNameComment,
					"-p", "all",
				}
				args = append(args, setRules...)
				args = append(args, "-j", "ACCEPT")
				writeLine(filterRules, args...)
			}
		}
	}
}

// Join all words with spaces, terminate with newline and write to buf.
func writeLine(buf *bytes.Buffer, words ...string) {
	buf.WriteString(strings.Join(words, " ") + "\n")
}

func policyChainName(policy *networkv1.NetworkPolicy) string {
	return fmt.Sprintf("%s-%s", policyChainPrefix, nameHash(fmt.Sprintf("%s_%s", policy.Name, policy.Namespace)))
}

func podChainName(pod *corev1.Pod) string {
	return fmt.Sprintf("%s-%s", podChainPrefix, nameHash(fmt.Sprintf("%s_%s", pod.Name, pod.Namespace)))
}

func nameHash(data string) string {
	hash := sha256.Sum256([]byte(data))
	encoded := base32.StdEncoding.EncodeToString(hash[:])
	return encoded[:16]
}

// SyncPodIPInIPSet ensures pod ip is expected in each policy's ipset
func (p *PolicyManager) SyncPodIPInIPSet(pod *corev1.Pod, add bool) {
	var polices []policy
	p.Lock()
	polices = p.policies
	p.Unlock()
	for _, policy := range polices {
		if policy.ingressRule == nil {
			continue
		}
		//TODO MatchExpressions
		if policy.np.Namespace == pod.Namespace && labels.SelectorFromSet(labels.Set(policy.np.Spec.PodSelector.MatchLabels)).Matches(labels.Set(pod.Labels)) {
			p.addOrDelIPSetEntry(add, &policy.ingressRule.dstIPTable.IPSet, pod.Status.PodIP)
		}
		for i, ingress := range policy.np.Spec.Ingress {
			for _, peer := range ingress.From {
				if peer.PodSelector != nil {
					if labels.SelectorFromSet(labels.Set(peer.PodSelector.MatchLabels)).Matches(labels.Set(pod.Labels)) {
						p.addOrDelIPSetEntry(add, &policy.ingressRule.srcRules[i].ipTable.IPSet, pod.Status.PodIP)
					}
				} else if peer.NamespaceSelector != nil {
					namespaces, err := p.getNamespaces(peer.NamespaceSelector)
					if err != nil {
						glog.Warning(err)
						continue
					}
					for _, ns := range namespaces {
						if ns.Name == pod.Namespace {
							p.addOrDelIPSetEntry(add, &policy.ingressRule.srcRules[i].ipTable.IPSet, pod.Status.PodIP)
							break
						}
					}
				}
			}
		}
	}
}

// SyncPod ensures GLX-INGRESS/GLX-EGRESS/GLX-POD-XXXX iptable chains are expected
func (p *PolicyManager) SyncPodChains(pod *corev1.Pod) error {
	var policies []policy
	p.Lock()
	policies = p.policies
	p.Unlock()
	podChain := utiliptables.Chain(podChainName(pod))
	var (
		filteredIngressPolicy = sets.NewInt()
		filteredEgressPolicy  = sets.NewInt()
	)
	for i, policy := range policies {
		if policy.np.Namespace != pod.Namespace {
			continue
		}
		if policy.ingressRule != nil {
			//TODO MatchExpressions
			if labels.SelectorFromSet(labels.Set(policy.np.Spec.PodSelector.MatchLabels)).Matches(labels.Set(pod.Labels)) {
				filteredIngressPolicy.Insert(i)
			}
		}
		if policy.egressRule != nil {
			//TODO MatchExpressions
			if labels.SelectorFromSet(labels.Set(policy.np.Spec.PodSelector.MatchLabels)).Matches(labels.Set(pod.Labels)) {
				filteredEgressPolicy.Insert(i)
			}
		}
	}
	if filteredIngressPolicy.Len() == 0 && filteredEgressPolicy.Len() == 0 {
		glog.V(4).Infof("pod %s_%s isn't a target pod of any ingress or egress network policy, ensuring its rules cleaned up", pod.Name, pod.Namespace)
		// clean up old rules
		return p.deletePodChains(pod)
	}
	if pod.Status.PodIP == "" {
		return nil
	}
	if err := p.ensureBasicChain(); err != nil {
		return err
	}
	podNameComment := fmt.Sprintf("%s_%s", pod.Name, pod.Namespace)
	filterChains := bytes.NewBuffer(nil)
	filterRules := bytes.NewBuffer(nil)
	writeLine(filterChains, "*filter")
	writeLine(filterChains, utiliptables.MakeChainLine(podChain))
	// -A GLX-POD-XXXX -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
	writeLine(filterRules, "-A", string(podChain), "-m", "comment", "--comment", podNameComment, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT")

	for i := range policies {
		if filteredIngressPolicy.Has(i) || filteredEgressPolicy.Has(i) {
			// -A GLX-POD-XXXX -j GLX-PLCY-XXXX
			writeLine(filterRules, "-A", string(podChain), "-m", "comment", "--comment", podNameComment, "-j", policyChainName(policies[i].np))
		}
	}
	// -A GLX-POD-XXXX -j DROP
	writeLine(filterRules, "-A", string(podChain), "-m", "comment", "--comment", podNameComment, "-j", "DROP")
	writeLine(filterRules, "COMMIT")

	lines := append(filterChains.Bytes(), filterRules.Bytes()...)
	err := p.iptableHandle.RestoreAll(lines, utiliptables.NoFlushTables, utiliptables.RestoreCounters)
	if err != nil {
		return fmt.Errorf("failed to execute iptables-restore for ruls %s: %v", string(lines), err)
	}

	if filteredIngressPolicy.Len() > 0 {
		// -A GLX-INGRESS -d x.x.x.x -j GLX-POD-XXXXX , this should be added after creating pod chain
		args := []string{"-d", pod.Status.PodIP, "-m", "comment", "--comment", podNameComment, "-j", string(podChain)}
		if _, err := p.iptableHandle.EnsureRule(utiliptables.Append, utiliptables.TableFilter, ingressChain, args...); err != nil {
			return fmt.Errorf("failed to add pod policy rule %s: %v", strings.Join(args, " "), err)
		}
	}
	if filteredEgressPolicy.Len() > 0 {
		// -A GLX-EGRESS -s x.x.x.x -j GLX-POD-XXXXX , this should be added after creating pod chain
		args := []string{"-s", pod.Status.PodIP, "-m", "comment", "--comment", podNameComment, "-j", string(podChain)}
		if _, err := p.iptableHandle.EnsureRule(utiliptables.Append, utiliptables.TableFilter, egressChain, args...); err != nil {
			return fmt.Errorf("failed to add pod policy rule %s: %v", strings.Join(args, " "), err)
		}
	}
	return nil
}

func (p *PolicyManager) ensureBasicChain() error {
	// -N GLX-INGRESS
	if _, err := p.iptableHandle.EnsureChain(utiliptables.TableFilter, ingressChain); err != nil {
		return fmt.Errorf("failed to ensure policy chain %s: %v", string(ingressChain), err)
	}
	// -N GLX-EGRESS
	if _, err := p.iptableHandle.EnsureChain(utiliptables.TableFilter, egressChain); err != nil {
		return fmt.Errorf("failed to ensure policy chain %s: %v", string(egressChain), err)
	}
	// -I FORWARD -j GLX-INGRESS
	if _, err := p.iptableHandle.EnsureRule(utiliptables.Prepend, utiliptables.TableFilter, utiliptables.ChainForward, "-j", string(ingressChain)); err != nil {
		return fmt.Errorf("failed to add FORWARD jump policy chain rule: %v", err)
	}
	// -I FORWARD -j GLX-EGRESS
	if _, err := p.iptableHandle.EnsureRule(utiliptables.Prepend, utiliptables.TableFilter, utiliptables.ChainForward, "-j", string(egressChain)); err != nil {
		return fmt.Errorf("failed to add FORWARD jump policy chain rule: %v", err)
	}
	// -I OUTPUT -j GLX-INGRESS
	if _, err := p.iptableHandle.EnsureRule(utiliptables.Prepend, utiliptables.TableFilter, utiliptables.ChainOutput, "-j", string(ingressChain)); err != nil {
		return fmt.Errorf("failed to add OUTPUT jump policy chain rule: %v", err)
	}
	// -I INPUT -j GLX-EGRESS
	if _, err := p.iptableHandle.EnsureRule(utiliptables.Prepend, utiliptables.TableFilter, utiliptables.ChainInput, "-j", string(egressChain)); err != nil {
		return fmt.Errorf("failed to add INPUT jump policy chain rule: %v", err)
	}
	return nil
}

// DeletePod deletes pod chain and rules in GLX-INGRESS/GLX-EGRESS chain
func (p *PolicyManager) deletePodChains(pod *corev1.Pod) error {
	podChain := utiliptables.Chain(podChainName(pod))
	// we don't know pod ip, so delete pod rules in GLX-INGRESS/GLX-EGRESS by keyword
	if err := p.deletePodRuleByKeyword(pod, ingressChain, string(podChain)); err != nil {
		glog.Warning(err)
	}
	if err := p.deletePodRuleByKeyword(pod, egressChain, string(podChain)); err != nil {
		glog.Warning(err)
	}
	// flush and delete pod chain
	if err := p.iptableHandle.FlushChain(utiliptables.TableFilter, podChain); err != nil {
		if strings.Contains(err.Error(), chainNotExistErr) {
			return nil
		}
		glog.Warningf("failed to flush pod %s_%s chain %s: %v", pod.Name, pod.Namespace, string(podChain), err)
	}
	if err := p.iptableHandle.DeleteChain(utiliptables.TableFilter, podChain); err != nil {
		glog.Warningf("failed to delete pod %s_%s chain %s: %v", pod.Name, pod.Namespace, string(podChain), err)
	}
	//TODO egress
	return nil
}

// deletePodRuleByKeyword delete rules in chain by keyword
func (p *PolicyManager) deletePodRuleByKeyword(pod *corev1.Pod, chain utiliptables.Chain, keyword string) error {
	lines, err := p.iptableHandle.ListRule(utiliptables.TableFilter, chain)
	if err != nil {
		if !strings.Contains(err.Error(), chainNotExistErr) {
			return fmt.Errorf("failed to list rule in chain %s: %v", string(chain), err)
		}
		return nil
	}
	var podLine string
	for i := range lines {
		// -A GLX-INGRESS -d x.x.x.x -j GLX-POD-XXXXX
		// -A GLX-EGRESS -s x.x.x.x -j GLX-POD-XXXXX
		if strings.Contains(lines[i], keyword) {
			podLine = lines[i]
			break
		}
	}
	if podLine == "" {
		glog.V(5).Infof("find no pod %s_%s keyword %s rule line in %s", pod.Name, pod.Namespace, keyword, string(chain))
	} else {
		glog.V(5).Infof("find pod %s_%s keyword %s rule line in %s: %s", pod.Name, pod.Namespace, keyword, string(chain), podLine)
		parts := strings.Split(podLine, " ")
		if len(parts) < 3 {
			glog.Warningf("unexpected pod %s_%s keyword %s rule line in %s: %s", pod.Name, pod.Namespace, keyword, string(chain), podLine)
		} else {
			if err := p.iptableHandle.DeleteRule(utiliptables.TableFilter, chain, parts[2:]...); err != nil {
				glog.Warningf("failed to delete pod %s_%s keyword %s rule line in %s: %v", pod.Name, pod.Namespace, keyword, string(chain), err)
			}
		}
	}
	return nil
}

func (p *PolicyManager) getNamespaces(namespaceSelector *v1.LabelSelector) ([]corev1.Namespace, error) {
	selectorStr := labels.FormatLabels(namespaceSelector.MatchLabels)
	if len(namespaceSelector.MatchLabels) == 0 {
		selectorStr = ""
	}
	list, err := p.client.CoreV1().Namespaces().List(v1.ListOptions{LabelSelector: selectorStr})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces by selector %s: %v", selectorStr, err)
	}
	return list.Items, nil
}

func (p *PolicyManager) addOrDelIPSetEntry(add bool, set *ipset.IPSet, podIP string) {
	if add {
		if err := p.ipsetHandle.AddEntryWithOptions(&ipset.Entry{IP: podIP, SetType: set.SetType}, set, true); err != nil {
			glog.Warningf("failed to add entry %s to ipset %s: %v", podIP, set.Name, err)
		}
	} else {
		if err := p.ipsetHandle.DelEntryWithOptions(set.Name, (&ipset.Entry{IP: podIP, SetType: set.SetType}).String()); err != nil {
			glog.Warningf("failed to del entry %s from ipset %s: %v", podIP, set.Name, err)
		}
	}
}
