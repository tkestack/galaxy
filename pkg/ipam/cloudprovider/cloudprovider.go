package cloudprovider

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/util/trace"
)

const NotFound = "ResourceNotFound"

type CloudProvider interface {
	AssignIP(in *AssignIPRequest) (*AssignIPReply, error)
	UnAssignIP(in *UnAssignIPRequest) (*UnAssignIPReply, error)
}

type GRPCCloudProvider struct {
	init              sync.Once
	cloudProviderAddr string
	client            IPProviderServiceClient
	backOff           wait.Backoff
	timeout           time.Duration
}

func NewGRPCCloudProvider(cloudProviderAddr string) CloudProvider {
	return &GRPCCloudProvider{
		backOff: wait.Backoff{
			Steps:    4,
			Duration: 10 * time.Millisecond,
			Factor:   5.0,
			Jitter:   0.1},
		timeout:           time.Second * 3,
		cloudProviderAddr: cloudProviderAddr,
	}
}

func (p *GRPCCloudProvider) connect() {
	p.init.Do(func() {
		glog.V(3).Infof("dial cloud provider with address %s", p.cloudProviderAddr)
		conn, err := grpc.Dial(p.cloudProviderAddr, grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("tcp", addr, timeout)
		}), grpc.WithInsecure())
		if err != nil {
			glog.Fatalf("failed to connect to cloud provider %s: %v", p.cloudProviderAddr, err)
		}
		p.client = NewIPProviderServiceClient(conn)
	})
}

func (p *GRPCCloudProvider) AssignIP(in *AssignIPRequest) (reply *AssignIPReply, err error) {
	p.connect()
	glog.V(3).Infof("send %+v", *in)
	t := trace.New("AssignIP")
	defer t.LogIfLong(time.Second)
	wait.ExponentialBackoff(p.backOff, func() (done bool, err error) {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
		defer cancel()
		reply, err = p.client.AssignIP(ctx, in)
		if err != nil {
			t.Step(fmt.Sprintf("AssignIP for %+v failed: %v", *in, err))
			return false, err
		}
		return true, nil
	})
	return
}

func (p *GRPCCloudProvider) UnAssignIP(in *UnAssignIPRequest) (reply *UnAssignIPReply, err error) {
	p.connect()
	glog.V(3).Infof("send %+v", *in)
	t := trace.New("AssignIP")
	defer t.LogIfLong(time.Second)
	wait.ExponentialBackoff(p.backOff, func() (done bool, err error) {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
		defer cancel()
		reply, err = p.client.UnAssignIP(ctx, in)
		if err != nil {
			if strings.Contains(err.Error(), NotFound) {
				return true, nil
			}
			t.Step(fmt.Sprintf("AssignIP for %+v failed: %v", *in, err))
			return false, err
		}
		return true, nil
	})
	return
}
