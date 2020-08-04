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
package api

import (
	"fmt"
	"net"
	"net/http"

	"github.com/emicklei/go-restful"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/apis/galaxy/v1alpha1"
	"tkestack.io/galaxy/pkg/ipam/client/clientset/versioned"
	list "tkestack.io/galaxy/pkg/ipam/client/listers/galaxy/v1alpha1"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	"tkestack.io/galaxy/pkg/utils/httputil"
)

type PoolController struct {
	Client       versioned.Interface
	PoolLister   list.PoolLister
	LockPoolFunc func(poolName string) func() // returns unlock func
	IPAM         floatingip.IPAM
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
	resp.WriteEntity(GetPoolResp{Resp: httputil.NewResp(http.StatusOK, ""), Pool: Pool{
		Name: pool.Name, Size: pool.Size, PreAllocateIP: pool.PreAllocateIP}})
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
		c.preAllocateIP(req, resp, &pool)
		return
	}
	httputil.Ok(resp)
	return
}

func (c *PoolController) preAllocateIP(req *restful.Request, resp *restful.Response, pool *Pool) {
	poolPrefix := util.NewKeyObj(util.DeploymentPrefixKey, "", "", "", pool.Name).PoolPrefix()
	defer c.LockPoolFunc(poolPrefix)()
	fips, err := c.IPAM.ByPrefix(poolPrefix)
	if err != nil {
		httputil.InternalError(resp, err)
		return
	}
	subnetSet, err := c.IPAM.NodeSubnetsByKey("")
	if err != nil {
		httputil.InternalError(resp, err)
		return
	}
	if subnetSet.Len() == 0 {
		resp.WriteHeaderAndEntity(http.StatusAccepted, UpdatePoolResp{
			Resp: httputil.NewResp(http.StatusAccepted, "No enough IPs"), RealPoolSize: len(fips)})
		return
	}
	j := 0
	subnets := subnetSet.UnsortedList()
	// TODO allocate in multiple subnets with distribution strategy?
	_, subnetIPNet, _ := net.ParseCIDR(subnets[0])
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
				resp.WriteHeaderAndEntity(http.StatusAccepted, UpdatePoolResp{
					Resp:         httputil.NewResp(http.StatusAccepted, "No enough IPs"),
					RealPoolSize: pool.Size - needAllocateIPs + i})
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
