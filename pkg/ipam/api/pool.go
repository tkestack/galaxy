package api

import (
	"fmt"

	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/apis/galaxy/v1alpha1"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/client/clientset/versioned"
	list "git.code.oa.com/gaiastack/galaxy/pkg/ipam/client/listers/galaxy/v1alpha1"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/httputil"
	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PoolController struct {
	Client     versioned.Interface
	PoolLister list.PoolLister
}

type Pool struct {
	Name string `json:"name"`
	Size int    `json:"size"`
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
	resp.WriteEntity(Pool{Name: pool.Name, Size: pool.Size})
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
		if errors.IsNotFound(err) {
			// create Pool
			if _, err := c.Client.GalaxyV1alpha1().Pools("kube-system").Create(&v1alpha1.Pool{
				TypeMeta:   v1.TypeMeta{Kind: "Pool", APIVersion: "v1alpha1"},
				ObjectMeta: v1.ObjectMeta{Name: pool.Name},
				Size:       pool.Size,
			}); err != nil {
				httputil.InternalError(resp, fmt.Errorf("failed to create Pool: %v", err))
				return
			}
			glog.Infof("created pool: %v", pool)
			httputil.Ok(resp)
			return
		}
		httputil.InternalError(resp, err)
		return
	}
	if pool.Size != p.Size {
		p.Size = pool.Size
		if _, err := c.Client.GalaxyV1alpha1().Pools("kube-system").Update(p); err != nil {
			httputil.InternalError(resp, err)
			return
		}
		glog.Infof("updated pool: %v", pool)
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
		httputil.InternalError(resp, err)
		return
	}
	glog.Infof("deleted pool: %s", name)
	httputil.Ok(resp)
}
