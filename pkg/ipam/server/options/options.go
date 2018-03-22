package options

import (
	"flag"

	"github.com/spf13/pflag"
)

// ServerRunOptions contains the options while running a server
type ServerRunOptions struct {
	Profiling bool
	Bind      string
	Port      int
	Master    string
	KubeConf  string
	EnableHA  bool
}

var (
	JsonConfigPath string
)

func init() {
	flag.StringVar(&JsonConfigPath, "config", "/etc/sysconfig/galaxy-ipam.json", "The json config file location of galaxy-ipam")
}

func NewServerRunOptions() *ServerRunOptions {
	return &ServerRunOptions{
		Profiling: true,
		Bind:      "0.0.0.0",
		Port:      9040,
		EnableHA:  true,
	}
}

// AddFlags add flags for a specific ASServer to the specified FlagSet
func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&s.Profiling, "profiling", s.Profiling, "Enable profiling via web interface host:port/debug/pprof/")
	fs.StringVar(&s.Bind, "bind", s.Bind, "The IP address on which to listen")
	fs.IntVar(&s.Port, "port", s.Port, "The port on which to serve")
	fs.StringVar(&s.Master, "master", s.Master, "The address and port of the Kubernetes API server")
	fs.StringVar(&s.KubeConf, "kubeconfig", s.KubeConf, "The kube config file location of APISwitch, used to support TLS")
	fs.BoolVar(&s.EnableHA, "ha", s.EnableHA, "If enable high availability, multiple instances of galaxy-ipams try to create a Kubernetes endpoint with expire time in annotation as an leader lock")
}
