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
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/kubernetes/pkg/version/verflag"
	"tkestack.io/galaxy/pkg/ipam/server"
)

func main() {
	// initialize rand seed
	rand.Seed(time.Now().UTC().UnixNano())

	s := server.NewServer()
	// add command line args
	s.AddFlags(pflag.CommandLine)

	flag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	// if checking version, print it and exit
	verflag.PrintAndExitIfRequested()

	if err := s.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err) // nolint: errcheck
		os.Exit(1)
	}
	//TODO handle signal ?
}
