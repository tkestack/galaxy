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
package options

import (
	"github.com/spf13/pflag"
)

// ServerRunOptions contains the options while running a server
type ServerRunOptions struct {
	Master               string
	KubeConf             string
	BridgeNFCallIptables bool
	IPForward            bool
	RouteENI             bool
	JsonConfigPath       string
	NetworkPolicy        bool
	PProf                bool
	// The same as [`confDir`](https://github.com/intel/multus-cni/blob/master/doc/configuration.md) in multus
	// To support dynamic changing network config or node specific network config
	NetworkConfDir string
	CNIPaths       []string
}

func NewServerRunOptions() *ServerRunOptions {
	opt := &ServerRunOptions{
		IPForward:            true,
		BridgeNFCallIptables: true,
		RouteENI:             false,
		JsonConfigPath:       "/etc/galaxy/galaxy.json",
		NetworkPolicy:        false,
		NetworkConfDir:       "/etc/cni/net.d/",
		CNIPaths:             []string{"/opt/cni/galaxy/bin"},
	}
	return opt
}

// AddFlags add flags for a specific ASServer to the specified FlagSet
func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	// TODO the options for legacy galaxy is api-servers and kubeconf
	fs.StringVar(&s.Master, "master", s.Master, "The address and port of the Kubernetes API server")
	fs.StringVar(&s.KubeConf, "kubeconfig", s.KubeConf, "The kube config file location of APISwitch, used to "+
		"support TLS")
	fs.BoolVar(&s.BridgeNFCallIptables, "bridge-nf-call-iptables", s.BridgeNFCallIptables, "Ensure "+
		"bridge-nf-call-iptables is set/unset")
	fs.BoolVar(&s.IPForward, "ip-forward", s.IPForward, "Ensure ip-forward is set/unset")
	fs.BoolVar(&s.RouteENI, "route-eni", s.RouteENI, "Ensure route-eni is set/unset")
	fs.StringVar(&s.JsonConfigPath, "json-config-path", s.JsonConfigPath, "The json config file location of galaxy")
	fs.BoolVar(&s.NetworkPolicy, "network-policy", s.NetworkPolicy, "Enable network policy function")
	fs.BoolVar(&s.PProf, "pprof", s.PProf, "Enable pprof")
	fs.StringVar(&s.NetworkConfDir, "network-conf-dir", s.NetworkConfDir,
		"Directory to additional network configs apart from those in json config")
	fs.StringSliceVar(&s.CNIPaths, "cni-paths", s.CNIPaths, "Additional cni paths apart from those received from kubelet")
}
