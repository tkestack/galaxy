package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"tkestack.io/galaxy/pkg/ipam/server"
	"github.com/spf13/pflag"
	"k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/kubernetes/pkg/version/verflag"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	s := server.NewServer()
	s.AddFlags(pflag.CommandLine)

	flag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	verflag.PrintAndExitIfRequested()

	if err := s.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err) // nolint: errcheck
		os.Exit(1)
	}
}
