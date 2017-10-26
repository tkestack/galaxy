package portmapping

import (
	"fmt"
	"net"
	"strings"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s"
	"github.com/golang/glog"
)

//OpenHostports opens all hostport for pod. The opened hostports are assigned to k8sPorts
func (h *PortMappingHandler) OpenHostports(podName string, portMapping bool, k8sPorts []*k8s.Port) error {
	var retErr error
	ports := make(map[hostport]closeable)
	for i := range k8sPorts {
		if k8sPorts[i].HostPort < 0 || (k8sPorts[i].HostPort == 0 && !portMapping) {
			// Ignore
			continue
		}
		hp := hostport{
			port:     k8sPorts[i].HostPort,
			protocol: strings.ToLower(string(k8sPorts[i].Protocol)),
		}
		// we bind to :0 if portmapping == true && hostport == 0 which asks kernel to allocate an unused port from its ip_local_port_range
		socket, err := openLocalPort(&hp)
		if err != nil {
			retErr = fmt.Errorf("cannot open hostport %d for %s: %v", k8sPorts[i].HostPort, podName, err)
			break
		}
		k8sPorts[i].HostPort = hp.port
		ports[hp] = socket
	}
	// If encounter any error, close all hostports that just got opened.
	if retErr != nil {
		glog.Error(retErr)
		for hp, socket := range ports {
			if err := socket.Close(); err != nil {
				glog.Errorf("Cannot clean up hostport %d for pod %s: %v", hp.port, podName, err)
			}
		}
		return retErr
	}

	if len(ports) != 0 {
		h.Lock()
		h.podPortMap[podName] = ports
		h.Unlock()
	}

	return nil
}

//CloseHostports closes all hostport for pod
func (h *PortMappingHandler) CloseHostports(podFullName string) {
	h.Lock()
	defer h.Unlock()
	// In case of kubelet restart, the port should have been closed
	if ports, ok := h.podPortMap[podFullName]; ok {
		for port, closer := range ports {
			if err := closer.Close(); err != nil {
				glog.Errorf("Cannot clean up hostport %d for pod %s: %v", port, podFullName, err)
			}
		}
		delete(h.podPortMap, podFullName)
	}
}

type closeable interface {
	Close() error
}

type hostport struct {
	port     int32
	protocol string
}

func (hp *hostport) String() string {
	return fmt.Sprintf("%s:%d", hp.protocol, hp.port)
}

func openLocalPort(hp *hostport) (closeable, error) {
	// For ports on node IPs, open the actual port and hold it, even though we
	// use iptables to redirect traffic.
	// This ensures a) that it's safe to use that port and b) that (a) stays
	// true.  The risk is that some process on the node (e.g. sshd or kubelet)
	// is using a port and we give that same port out to a Service.  That would
	// be bad because iptables would silently claim the traffic but the process
	// would never know.
	// NOTE: We should not need to have a real listen()ing socket - bind()
	// should be enough, but I can't figure out a way to e2e test without
	// it.  Tools like 'ss' and 'netstat' do not show sockets that are
	// bind()ed but not listen()ed, and at least the default debian netcat
	// has no way to avoid about 10 seconds of retries.
	var socket closeable
	switch hp.protocol {
	case "tcp":
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", hp.port))
		if err != nil {
			return nil, err
		}
		socket = listener
		hp.port = int32(listener.Addr().(*net.TCPAddr).Port)
		glog.Infof("listening to tcp %d", hp.port)
	case "udp":
		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", hp.port))
		if err != nil {
			return nil, err
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			return nil, err
		}
		socket = conn
		hp.port = int32(conn.LocalAddr().(*net.UDPAddr).Port)
		glog.Infof("listening to udp %d", hp.port)
	default:
		return nil, fmt.Errorf("unknown protocol %q", hp.protocol)
	}
	glog.V(3).Infof("Opened local port %s", hp.String())
	return socket, nil
}
