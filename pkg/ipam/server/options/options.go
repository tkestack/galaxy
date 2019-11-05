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
	"flag"

	"github.com/spf13/pflag"
)

// ServerRunOptions contains the options while running a server
type ServerRunOptions struct {
	Profiling      bool
	Bind           string
	Port           int
	APIPort        int
	Master         string
	KubeConf       string
	Swagger        bool
	LeaderElection LeaderElectionConfiguration
}

var (
	JsonConfigPath string
)

func init() {
	flag.StringVar(&JsonConfigPath, "config", "/etc/galaxy/galaxy-ipam.json", "The json config file location of"+
		" galaxy-ipam")
}

func NewServerRunOptions() *ServerRunOptions {
	opt := &ServerRunOptions{
		Profiling:      true,
		Bind:           "0.0.0.0",
		Port:           9040,
		APIPort:        9041,
		Swagger:        false,
		LeaderElection: DefaultLeaderElectionConfiguration(),
	}
	opt.LeaderElection.LeaderElect = true
	return opt
}

// AddFlags add flags for a specific ASServer to the specified FlagSet
func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&s.Profiling, "profiling", s.Profiling, "Enable profiling via web interface host:port/debug/pprof/")
	fs.StringVar(&s.Bind, "bind", s.Bind, "The IP address on which to listen")
	fs.IntVar(&s.Port, "port", s.Port, "The port on which to serve")
	fs.IntVar(&s.APIPort, "api-port", s.APIPort, "The API port on which to serve")
	fs.StringVar(&s.Master, "master", s.Master, "The address and port of the Kubernetes API server")
	fs.StringVar(&s.KubeConf, "kubeconfig", s.KubeConf, "The kube config file location of APISwitch, used to support TLS")
	fs.BoolVar(&s.Swagger, "swagger", s.Swagger, "Enable swagger via API web interface host:api-port/apidocs.json/")
	BindFlags(&s.LeaderElection, fs)
}
