package cloudprovider

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/cloudprovider/rpc"
	"github.com/golang/glog"
	"google.golang.org/grpc"
)

type CloudProvider interface {
	AssignIP(in *rpc.AssignIPRequest) (*rpc.AssignIPReply, error)
	UnAssignIP(in *rpc.UnAssignIPRequest) (*rpc.UnAssignIPReply, error)
}

type GRPCCloudProvider struct {
	init              sync.Once
	cloudProviderAddr string
	client            rpc.IPProviderServiceClient
	timeout           time.Duration
}

func NewGRPCCloudProvider(cloudProviderAddr string) CloudProvider {
	return &GRPCCloudProvider{
		timeout:           time.Second * 40,
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
		p.client = rpc.NewIPProviderServiceClient(conn)
	})
}

func (p *GRPCCloudProvider) AssignIP(in *rpc.AssignIPRequest) (reply *rpc.AssignIPReply, err error) {
	p.connect()
	glog.V(5).Infof("AssignIP %v", in)

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	reply, err = p.client.AssignIP(ctx, in)
	glog.V(5).Infof("request %v, reply %v, err %v", in, reply, err)
	if err != nil || reply == nil || !reply.Success {
		err = fmt.Errorf("AssignIP for %v failed: reply %v, err %v", in, reply, err)
		glog.V(5).Info(err)
	}
	return
}

func (p *GRPCCloudProvider) UnAssignIP(in *rpc.UnAssignIPRequest) (reply *rpc.UnAssignIPReply, err error) {
	p.connect()
	glog.V(5).Infof("UnAssignIP %v", in)

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	reply, err = p.client.UnAssignIP(ctx, in)
	glog.V(5).Infof("request %v, reply %v, err %v", in, reply, err)
	if err != nil || reply == nil || !reply.Success {
		err = fmt.Errorf("UnAssignIP for %v failed: reply %v, err %v", in, reply, err)
		glog.V(5).Info(err)
	}
	return
}
