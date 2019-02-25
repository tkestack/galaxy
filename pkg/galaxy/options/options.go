package options

import (
	"github.com/spf13/pflag"
)

// ServerRunOptions contains the options while running a server
type ServerRunOptions struct {
	Master               string
	KubeConf             string
	NetworkConf          string
	BridgeNFCallIptables bool
	IPForward            bool
}

func NewServerRunOptions() *ServerRunOptions {
	opt := &ServerRunOptions{
		NetworkConf:          `{"galaxy-flannel":{"delegate":{"type":"galaxy-bridge","isDefaultGateway":true,"forceAddress":true},"subnetFile":"/run/flannel/subnet.env"}}`,
		IPForward:            true,
		BridgeNFCallIptables: true,
	}
	return opt
}

// AddFlags add flags for a specific ASServer to the specified FlagSet
func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.NetworkConf, "network-conf", s.NetworkConf, "various network configrations")
	// TODO the options for legacy galaxy is api-servers and kubeconf
	fs.StringVar(&s.Master, "master", s.Master, "The address and port of the Kubernetes API server")
	fs.StringVar(&s.KubeConf, "kubeconfig", s.KubeConf, "The kube config file location of APISwitch, used to support TLS")
	fs.BoolVar(&s.BridgeNFCallIptables, "bridge-nf-call-iptables", s.BridgeNFCallIptables, "Ensure bridge-nf-call-iptables is set/unset")
	fs.BoolVar(&s.IPForward, "ip-forward", s.IPForward, "Ensure ip-forward is set/unset")
}
