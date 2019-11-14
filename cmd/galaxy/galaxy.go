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
package main

import (
	"math/rand"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	glog "k8s.io/klog"
	"k8s.io/kubernetes/pkg/version/verflag"
	"tkestack.io/galaxy/pkg/galaxy"
	"tkestack.io/galaxy/pkg/signal"
)

func main() {
	// initialize rand seed
	rand.Seed(time.Now().UTC().UnixNano())
	galaxy := galaxy.NewGalaxy()
	// add command line args
	galaxy.AddFlags(pflag.CommandLine)
	flag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	// if checking version, print it and exit
	verflag.PrintAndExitIfRequested()
	if err := galaxy.Start(); err != nil {
		glog.Fatalf("Error start galaxy: %v", err)
	}
	// handle signals
	signal.BlockSignalHandler(func() {
		if err := galaxy.Stop(); err != nil {
			glog.Errorf("Error stop galaxy: %v", err)
		}
	})
}
