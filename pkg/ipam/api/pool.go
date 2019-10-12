package api

import (
	"fmt"
	"net"
	"net/http"

	"git.code.oa.com/tkestack/galaxy/pkg/api/galaxy/constant"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/apis/galaxy/v1alpha1"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/client/clientset/versioned"
	list "git.code.oa.com/tkestack/galaxy/pkg/ipam/client/listers/galaxy/v1alpha1"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/tkestack/galaxy/pkg/ipam/schedulerplugin/util"
	"git.code.oa.com/tkestack/galaxy/pkg/utils/httputil"
	"git.code.oa.com/tkestack/galaxy/pkg/utils/keylock"
	"github.com/emicklei/go-restful"
	glog "k8s.io/klog"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PoolController struct {
	Client           versioned.Interface
	PoolLister       list.PoolLister
	LockPool         *keylock.Keylock
	IPAM, SecondIPAM floatingip.IPAM
}

type Pool struct {
	Name          string `json:"name"`
	Size          int    `json:"size"`
	PreAllocateIP bool   `json:"preAllocateIP"`
}

func (Pool) SwaggerDoc() map[string]string {
	return map[string]string{
		"name":          "pool name",
		"size":          "pool size",
		"preAllocateIP": "Set to true to allocate IPs when creating or updating pool",
	}
}

type GetPoolResp struct {
	httputil.Resp
	Pool Pool `json:"pool"`
}

func (c *PoolController) Get(req *restful.Request, resp *restful.Response) {
	name := req.PathParameter("name")
	if name == "" {
		httputil.BadRequest(resp, fmt.Errorf("pool name is empty"))
		return
	}
	pool, err := c.Client.GalaxyV1alpha1().Pools("kube-system").Get(name, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			httputil.ItemNotFound(resp, fmt.Errorf("pool %s", name))
			return
		}
		httputil.InternalError(resp, err)
		return
	}
	resp.WriteEntity(GetPoolResp{Resp: httputil.NewResp(http.StatusOK, ""), Pool: Pool{Name: pool.Name, Size: pool.Size, PreAllocateIP: pool.PreAllocateIP}})
}

type UpdatePoolResp struct {
	httputil.Resp
	RealPoolSize int `json:"realPoolSize"`
}

func (UpdatePoolResp) SwaggerDoc() map[string]string {
	return map[string]string{
		"realPoolSize": "real num of IPs of this pool after creating or updating",
	}
}

func (c *PoolController) CreateOrUpdate(req *restful.Request, resp *restful.Response) {
	var pool Pool
	if err := req.ReadEntity(&pool); err != nil {
		httputil.BadRequest(resp, err)
		return
	}
	if pool.Name == "" {
		httputil.BadRequest(resp, fmt.Errorf("pool name is empty"))
		return
	}
	p, err := c.Client.GalaxyV1alpha1().Pools("kube-system").Get(pool.Name, v1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			httputil.InternalError(resp, err)
			return
		}
		// create Pool
		if _, err := c.Client.GalaxyV1alpha1().Pools("kube-system").Create(&v1alpha1.Pool{
			TypeMeta:      v1.TypeMeta{Kind: "Pool", APIVersion: "v1alpha1"},
			ObjectMeta:    v1.ObjectMeta{Name: pool.Name},
			Size:          pool.Size,
			PreAllocateIP: pool.PreAllocateIP,
		}); err != nil {
			httputil.InternalError(resp, fmt.Errorf("failed to create Pool: %v", err))
			return
		}
		glog.Infof("created pool: %v", pool)
	} else {
		if pool.Size != p.Size || p.PreAllocateIP != pool.PreAllocateIP {
			p.Size = pool.Size
			p.PreAllocateIP = pool.PreAllocateIP
			if _, err := c.Client.GalaxyV1alpha1().Pools("kube-system").Update(p); err != nil {
				httputil.InternalError(resp, err)
				return
			}
			glog.Infof("updated pool: %v", pool)
		}
	}
	if pool.PreAllocateIP {
		poolPrefix := util.NewKeyObj(util.DeploymentPrefixKey, "", "", "", pool.Name).PoolPrefix()
		lockIndex := c.LockPool.GetLockIndex([]byte(poolPrefix))
		c.LockPool.RawLock(lockIndex)
		defer c.LockPool.RawUnlock(lockIndex)
		fips, err := c.IPAM.ByPrefix(poolPrefix)
		if err != nil {
			httputil.InternalError(resp, err)
			return
		}
		subnets, err := c.IPAM.QueryRoutableSubnetByKey("")
		if err != nil {
			httputil.InternalError(resp, err)
			return
		}
		if len(subnets) == 0 {
			resp.WriteHeaderAndEntity(http.StatusAccepted, UpdatePoolResp{Resp: httputil.NewResp(http.StatusAccepted, "No enough IPs"), RealPoolSize: len(fips)})
			return
		}
		j := 0
		_, subnetIPNet, _ := net.ParseCIDR(subnets[j])
		if pool.Size <= len(fips) {
			resp.WriteEntity(UpdatePoolResp{Resp: httputil.NewResp(http.StatusOK, ""), RealPoolSize: len(fips)})
			return
		}
		needAllocateIPs := pool.Size - len(fips)
		for i := 0; i < needAllocateIPs; i++ {
			ip, err := c.IPAM.AllocateInSubnet(poolPrefix, subnetIPNet, constant.ReleasePolicyNever, "")
			if err == nil {
				glog.Infof("allocated ip %s to %s during creating or updating pool", ip.String(), poolPrefix)
				continue
			} else if err == floatingip.ErrNoEnoughIP {
				j++
				if j == len(subnets) {
					resp.WriteHeaderAndEntity(http.StatusAccepted, UpdatePoolResp{Resp: httputil.NewResp(http.StatusAccepted, "No enough IPs"), RealPoolSize: pool.Size - needAllocateIPs + i})
					return
				}
				_, subnetIPNet, _ = net.ParseCIDR(subnets[j])
				i--
			} else {
				httputil.InternalError(resp, err)
				return
			}
		}
		resp.WriteEntity(UpdatePoolResp{Resp: httputil.NewResp(http.StatusOK, ""), RealPoolSize: pool.Size})
		return
	}
	httputil.Ok(resp)
	return
}

func (c *PoolController) Delete(req *restful.Request, resp *restful.Response) {
	name := req.PathParameter("name")
	if name == "" {
		httputil.BadRequest(resp, fmt.Errorf("pool name is empty"))
		return
	}
	if err := c.Client.GalaxyV1alpha1().Pools("kube-system").Delete(name, &v1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			httputil.ItemNotFound(resp, fmt.Errorf("pool %s", name))
			return
		}
		httputil.InternalError(resp, err)
		return
	}
	glog.Infof("deleted pool: %s", name)
	httputil.Ok(resp)
}
