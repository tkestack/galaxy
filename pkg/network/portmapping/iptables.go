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
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	glog "k8s.io/klog"
	utildbus "k8s.io/kubernetes/pkg/util/dbus"
	utilexec "k8s.io/utils/exec"
	"tkestack.io/galaxy/pkg/api/k8s"
	utiliptables "tkestack.io/galaxy/pkg/utils/iptables"
)

const (
	// the hostport chain
	kubeHostportsChain utiliptables.Chain = "KUBE-HOSTPORTS"
	// prefix for hostport chains
	kubeHostportChainPrefix string = "KUBE-HP-"

	KubeMarkMasqChain utiliptables.Chain = "KUBE-MARK-MASQ"
)

type PortMappingHandler struct {
	utiliptables.Interface
	podPortMap map[string]map[hostport]closeable
	sync.Mutex
	natInterfaceName string
}

func New(natInterfaceName string) *PortMappingHandler {
	return &PortMappingHandler{
		Interface:        utiliptables.New(utilexec.New(), utildbus.New(), utiliptables.ProtocolIpv4),
		podPortMap:       make(map[string]map[hostport]closeable),
		natInterfaceName: natInterfaceName,
	}
}

func (h *PortMappingHandler) SetupPortMapping(ports []k8s.Port) error {
	var kubeHostportsChainRules [][]string
	natChains := bytes.NewBuffer(nil)
	natRules := bytes.NewBuffer(nil)
	writeLine(natChains, "*nat")
	writeKubeMarkRule(natChains, natRules)

	for _, containerPort := range ports {
		protocol := strings.ToLower(containerPort.Protocol)
		hostportChain := hostportChainName(containerPort, containerPort.PodName)
		// write chain name
		writeLine(natChains, utiliptables.MakeChainLine(hostportChain))

		// Redirect to hostport chain
		// We may run iptables-restore concurrently for multiple pods, so we
		// can't know exactly all the mapped ports at any given time cause we
		// don't hold a lock before executing iptables-restore. So we have to
		// execute add or delete rules of KUBE-HOSTPORTS chain separately
		args := []string{
			"-m", "comment", "--comment",
			fmt.Sprintf(`"%s hostport %d"`, containerPort.PodName, containerPort.HostPort),
			"-m", protocol, "-p", protocol,
			"--dport", fmt.Sprintf("%d", containerPort.HostPort),
		}
		if containerPort.HostIP != "" {
			args = append(args, "-d", containerPort.HostIP)
		}
		args = append(args, "-j", string(hostportChain))
		kubeHostportsChainRules = append(kubeHostportsChainRules, args)

		containerPortChainRules(&containerPort, protocol, hostportChain, natRules)
	}

	writeLine(natRules, "COMMIT")

	natLines := append(natChains.Bytes(), natRules.Bytes()...)
	err := h.RestoreAll(natLines, utiliptables.NoFlushTables, utiliptables.RestoreCounters)
	if err != nil {
		return fmt.Errorf("Failed to execute iptables-restore for ruls %s: %v", string(natLines), err)
	}

	for _, rule := range kubeHostportsChainRules {
		if _, err := h.EnsureRule(utiliptables.Append, utiliptables.TableNAT, kubeHostportsChain, rule...); err != nil {
			return fmt.Errorf("failed to add rule %s: %v", rule, err)
		}
	}
	return nil
}

func containerPortChainRules(containerPort *k8s.Port, protocol string, hostportChain utiliptables.Chain,
	natRules *bytes.Buffer) {
	// Assuming kubelet is syncing iptables KUBE-MARK-MASQ chain
	// If the request comes from the pod that is serving the hostport, then SNAT
	args := []string{
		"-A", string(hostportChain),
		"-m", "comment", "--comment", fmt.Sprintf(`"%s hostport %d"`, containerPort.PodName, containerPort.HostPort),
		"-s", containerPort.PodIP, "-j", string(KubeMarkMasqChain),
	}
	writeLine(natRules, args...)

	// Create hostport chain to DNAT traffic to final destination
	// IPTables will maintained the stats for this chain
	args = []string{
		"-A", string(hostportChain),
		"-m", "comment", "--comment", fmt.Sprintf(`"%s hostport %d"`, containerPort.PodName, containerPort.HostPort),
		"-m", protocol, "-p", protocol,
		"-j", "DNAT", fmt.Sprintf("--to-destination=%s:%d", containerPort.PodIP, containerPort.ContainerPort),
	}
	writeLine(natRules, args...)
}

func (h *PortMappingHandler) CleanPortMapping(ports []k8s.Port) error {
	var kubeHostportsChainRules [][]string
	natChains := bytes.NewBuffer(nil)
	natRules := bytes.NewBuffer(nil)
	writeLine(natChains, "*nat")

	for _, containerPort := range ports {
		protocol := strings.ToLower(containerPort.Protocol)
		hostportChain := hostportChainName(containerPort, containerPort.PodName)
		// write chain name
		writeLine(natChains, utiliptables.MakeChainLine(hostportChain))
		writeLine(natRules, "-X", string(hostportChain))
		args := []string{
			"-m", "comment", "--comment",
			fmt.Sprintf(`"%s hostport %d"`, containerPort.PodName, containerPort.HostPort),
			"-m", protocol, "-p", protocol,
			"--dport", fmt.Sprintf("%d", containerPort.HostPort),
			"-j", string(hostportChain),
		}
		kubeHostportsChainRules = append(kubeHostportsChainRules, args)
	}

	writeLine(natRules, "COMMIT")

	natLines := append(natChains.Bytes(), natRules.Bytes()...)

	for _, rule := range kubeHostportsChainRules {
		if err := h.withRetry(func() error {
			return h.DeleteRule(utiliptables.TableNAT, kubeHostportsChain, rule...)
		}); err != nil {
			err = fmt.Errorf("failed to delete rule %s: %v", rule, err)
			glog.Warning(err)
			return err
		}
	}
	if err := h.withRetry(func() error {
		return h.RestoreAll(natLines, utiliptables.NoFlushTables, utiliptables.RestoreCounters)
	}); err != nil {
		err = fmt.Errorf("failed to execute iptables-restore for rules %s: %v", string(natLines), err)
		glog.Warning(err)
		return err
	}
	return nil
}

func (h *PortMappingHandler) withRetry(f func() error) error {
	return wait.PollImmediate(time.Millisecond*100, time.Second*30, func() (done bool, err error) {
		if err = f(); err == nil {
			return true, nil
		} else if strings.Contains(err.Error(), "Resource temporarily unavailable") {
			return false, nil
		} else {
			glog.Error(err)
			return false, err
		}
	})
}

// SetupPortMappingForAllPods setup iptables for all pods at start time
func (h *PortMappingHandler) SetupPortMappingForAllPods(ports []k8s.Port) error {
	if err := h.EnsureBasicRule(); err != nil {
		return err
	}

	iptablesSaveRaw := bytes.NewBuffer(nil)
	// Get iptables-save output so we can check for existing chains and rules.
	// This will be a map of chain name to chain with rules as stored in iptables-save/iptables-restore
	existingNATChains := make(map[utiliptables.Chain]string) // nolint: staticcheck
	err := h.Interface.SaveInto(utiliptables.TableNAT, iptablesSaveRaw)
	if err != nil { // if we failed to get any rules
		return fmt.Errorf("Failed to execute iptables-save, syncing all rules: %v", err)
	} else { // otherwise parse the output
		existingNATChains = utiliptables.GetChainLines(utiliptables.TableNAT, iptablesSaveRaw.Bytes())
	}

	natChains := bytes.NewBuffer(nil)
	natRules := bytes.NewBuffer(nil)
	writeLine(natChains, "*nat")
	writeKubeMarkRule(natChains, natRules)
	// Make sure we keep stats for the top-level chains, if they existed
	// (which most should have because we created them above).
	if chain, ok := existingNATChains[kubeHostportsChain]; ok {
		writeLine(natChains, chain)
	} else {
		writeLine(natChains, utiliptables.MakeChainLine(kubeHostportsChain))
	}

	// Accumulate NAT chains to keep.
	activeNATChains := map[utiliptables.Chain]bool{} // use a map as a set

	for _, containerPort := range ports {
		protocol := strings.ToLower(containerPort.Protocol)
		hostportChain := hostportChainName(containerPort, containerPort.PodName)
		if chain, ok := existingNATChains[hostportChain]; ok {
			writeLine(natChains, chain)
		} else {
			writeLine(natChains, utiliptables.MakeChainLine(hostportChain))
		}

		activeNATChains[hostportChain] = true

		// Redirect to hostport chain
		args := []string{
			"-A", string(kubeHostportsChain),
			"-m", "comment", "--comment",
			fmt.Sprintf(`"%s hostport %d"`, containerPort.PodName, containerPort.HostPort),
			"-m", protocol, "-p", protocol,
			"--dport", fmt.Sprintf("%d", containerPort.HostPort),
			"-j", string(hostportChain),
		}
		writeLine(natRules, args...)

		containerPortChainRules(&containerPort, protocol, hostportChain, natRules)
	}

	// Delete chains no longer in use.
	for chain := range existingNATChains {
		if !activeNATChains[chain] {
			chainString := string(chain)
			if !strings.HasPrefix(chainString, kubeHostportChainPrefix) {
				// Ignore chains that aren't ours.
				continue
			}
			// We must (as per iptables) write a chain-line for it, which has
			// the nice effect of flushing the chain.  Then we can remove the
			// chain.
			writeLine(natChains, existingNATChains[chain])
			writeLine(natRules, "-X", chainString)
		}
	}
	writeLine(natRules, "COMMIT")

	natLines := append(natChains.Bytes(), natRules.Bytes()...)
	err = h.Interface.RestoreAll(natLines, utiliptables.NoFlushTables, utiliptables.RestoreCounters)
	if err != nil {
		return fmt.Errorf("Failed to execute iptables-restore for ruls %s: %v", string(natLines), err)
	}
	return nil
}

// Join all words with spaces, terminate with newline and write to buf.
func writeLine(buf *bytes.Buffer, words ...string) {
	buf.WriteString(strings.Join(words, " ") + "\n")
}

//hostportChainName takes containerPort for a pod and returns associated iptables chain.
// This is computed by hashing (sha256)
// then encoding to base32 and truncating with the prefix "KUBE-SVC-".  We do
// this because IPTables Chain Names must be <= 28 chars long, and the longer
// they are the harder they are to read.
func hostportChainName(port k8s.Port, podFullName string) utiliptables.Chain {
	hash := sha256.Sum256([]byte(strconv.Itoa(int(port.HostPort)) + port.Protocol +
		strconv.Itoa(int(port.ContainerPort)) + podFullName))
	encoded := base32.StdEncoding.EncodeToString(hash[:])
	return utiliptables.Chain(kubeHostportChainPrefix + encoded[:16])
}

func (h *PortMappingHandler) EnsureBasicRule() error {
	if err := h.Interface.EnsurePolicy(utiliptables.TableFilter, utiliptables.ChainForward, "ACCEPT"); err != nil {
		glog.Warningf("set policy for %v/%v failed: %v", utiliptables.TableFilter,
			utiliptables.ChainForward, err.Error())
	}
	if _, err := h.Interface.EnsureChain(utiliptables.TableNAT, kubeHostportsChain); err != nil {
		return fmt.Errorf("Failed to ensure that %s chain %s exists: %v", utiliptables.TableNAT,
			kubeHostportsChain, err)
	}
	tableChainsNeedJumpServices := []struct {
		table utiliptables.Table
		chain utiliptables.Chain
	}{
		{utiliptables.TableNAT, utiliptables.ChainOutput},
		{utiliptables.TableNAT, utiliptables.ChainPrerouting},
	}
	args := []string{"-m", "comment", "--comment", "kube hostport portals",
		"-m", "addrtype", "--dst-type", "LOCAL",
		"-j", string(kubeHostportsChain)}
	for _, tc := range tableChainsNeedJumpServices {
		if _, err := h.Interface.EnsureRule(utiliptables.Prepend, tc.table, tc.chain, args...); err != nil {
			return fmt.Errorf("Failed to ensure that %s chain %s jumps to %s: %v", tc.table, tc.chain,
				kubeHostportsChain, err)
		}
	}
	if h.natInterfaceName != "" {
		// Need to SNAT traffic from localhost
		args = []string{
			"-m", "comment", "--comment", "SNAT for localhost access to hostports",
			"-o", h.natInterfaceName, "-s", "127.0.0.0/8", "-j", "MASQUERADE"}
		if _, err := h.Interface.EnsureRule(utiliptables.Append, utiliptables.TableNAT, utiliptables.ChainPostrouting,
			args...); err != nil {
			return fmt.Errorf("Failed to ensure that %s chain %s jumps to MASQUERADE: %v", utiliptables.TableNAT,
				utiliptables.ChainPostrouting, err)
		}
	}
	return nil
}

func writeKubeMarkRule(natChains, natRules *bytes.Buffer) {
	writeLine(natChains, utiliptables.MakeChainLine(KubeMarkMasqChain))
	writeLine(natRules, "-A", string(KubeMarkMasqChain), "-j", "MARK", "--set-xmark", "0x4000/0x4000")
}
