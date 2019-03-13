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
}

func NewServerRunOptions() *ServerRunOptions {
	opt := &ServerRunOptions{
		IPForward:            true,
		BridgeNFCallIptables: true,
		RouteENI:             false,
		JsonConfigPath:       "/etc/galaxy/galaxy.json",
		NetworkPolicy:        false,
	}
	return opt
}

// AddFlags add flags for a specific ASServer to the specified FlagSet
func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	// TODO the options for legacy galaxy is api-servers and kubeconf
	fs.StringVar(&s.Master, "master", s.Master, "The address and port of the Kubernetes API server")
	fs.StringVar(&s.KubeConf, "kubeconfig", s.KubeConf, "The kube config file location of APISwitch, used to support TLS")
	fs.BoolVar(&s.BridgeNFCallIptables, "bridge-nf-call-iptables", s.BridgeNFCallIptables, "Ensure bridge-nf-call-iptables is set/unset")
	fs.BoolVar(&s.IPForward, "ip-forward", s.IPForward, "Ensure ip-forward is set/unset")
	fs.BoolVar(&s.RouteENI, "route-eni", s.RouteENI, "Ensure route-eni is set/unset")
	fs.StringVar(&s.JsonConfigPath, "json-config-path", s.JsonConfigPath, "The json config file location of galaxy")
	fs.BoolVar(&s.NetworkPolicy, "network-policy", s.NetworkPolicy, "Enable network policy function")
}
