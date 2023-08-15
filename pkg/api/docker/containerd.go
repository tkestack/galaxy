package docker

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"
	"net"
	"net/url"
	"os"
	"time"
)

const (
	// unixProtocol is the network protocol of unix socket.
	unixProtocol         = "unix"
	maxMsgSize           = 1024 * 1024 * 16
	maxBackoffDelay      = 3 * time.Second
	baseBackoffDelay     = 100 * time.Millisecond
	minConnectionTimeout = 5 * time.Second
)

func newContainerdClient() (criapi.RuntimeServiceClient, error) {
	addr, dialer, err := GetAddressAndDialer(os.Getenv("CONTAINERD_HOST"))
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	dailCtx, cancle := context.WithTimeout(ctx, 30*time.Second)
	defer cancle()

	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)))

	connParams := grpc.ConnectParams{
		Backoff: backoff.DefaultConfig,
	}
	connParams.MinConnectTimeout = minConnectionTimeout
	connParams.Backoff.BaseDelay = baseBackoffDelay
	connParams.Backoff.MaxDelay = maxBackoffDelay
	dialOpts = append(dialOpts,
		grpc.WithConnectParams(connParams),
		grpc.WithBlock(),
	)

	conn, err := grpc.DialContext(dailCtx, addr, dialOpts...)
	if err != nil {
		klog.ErrorS(err, "Connect remote runtime failed", "address", addr)
		return nil, err
	}

	runtimeClient := criapi.NewRuntimeServiceClient(conn)
	return runtimeClient, nil
}

// GetAddressAndDialer returns the address parsed from the given endpoint and a context dialer.
func GetAddressAndDialer(endpoint string) (string, func(ctx context.Context, addr string) (net.Conn, error), error) {
	protocol, addr, err := parseEndpointWithFallbackProtocol(endpoint, unixProtocol)
	if err != nil {
		return "", nil, err
	}
	if protocol != unixProtocol {
		return "", nil, fmt.Errorf("only support unix socket endpoint")
	}

	return addr, dial, nil
}

func dial(ctx context.Context, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, unixProtocol, addr)
}

func parseEndpointWithFallbackProtocol(endpoint string, fallbackProtocol string) (protocol string, addr string, err error) {
	if protocol, addr, err = parseEndpoint(endpoint); err != nil && protocol == "" {
		fallbackEndpoint := fallbackProtocol + "://" + endpoint
		protocol, addr, err = parseEndpoint(fallbackEndpoint)
		if err == nil {
			klog.InfoS("Using this endpoint is deprecated, please consider using full URL format", "endpoint", endpoint, "URL", fallbackEndpoint)
		}
	}
	return
}

func parseEndpoint(endpoint string) (string, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", err
	}

	switch u.Scheme {
	case "tcp":
		return "tcp", u.Host, nil

	case "unix":
		return "unix", u.Path, nil

	case "":
		return "", "", fmt.Errorf("using %q as endpoint is deprecated, please consider using full url format", endpoint)

	default:
		return u.Scheme, "", fmt.Errorf("protocol %q not supported", u.Scheme)
	}
}
