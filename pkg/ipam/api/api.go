package api

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/floatingip"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/schedulerplugin/util"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/httputil"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/nets"
	pageutil "git.code.oa.com/gaiastack/galaxy/pkg/utils/page"
	"github.com/emicklei/go-restful"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/glog"
)

type Controller struct {
	ipam, secondIpam floatingip.IPAM
	podLister        v1.PodLister
}

func NewController(ipam, secondIpam floatingip.IPAM, lister v1.PodLister) *Controller {
	return &Controller{
		ipam:       ipam,
		secondIpam: secondIpam,
		podLister:  lister,
	}
}

type FloatingIP struct {
	IP           string `json:"ip"`
	Namespace    string `json:"namespace"`
	AppName      string `json:"appName"`
	PodName      string `json:"podName"`
	PoolName     string `json:"poolName"`
	Policy       uint16 `json:"policy"`
	IsDeployment bool   `json:"isDeployment"`
	UpdateTime   int64  `json:"updateTime"`
	Status       string `json:"status"`
	Releasable   bool   `json:"releasable"`
	attr         string
}

func (c *Controller) ListIPs(req *restful.Request, resp *restful.Response) {
	keyword := req.QueryParameter("keyword")

	ret, err := listIPs(keyword, c.ipam, c.secondIpam)
	if err != nil {
		httputil.InternalError(resp, err)
		return
	}
	sortParam, page, size := pageutil.PagingParams(req)
	pagin := format(sortParam, page, size, ret)
	if err := fillReleasableAndStatus(c.podLister, ret); err != nil {
		httputil.InternalError(resp, err)
		return
	}
	resp.WriteEntity(*pagin) // nolint: errcheck
}

func fillReleasableAndStatus(lister v1.PodLister, ips []FloatingIP) error {
	for i := range ips {
		ips[i].Releasable = true
		if ips[i].PodName == "" {
			continue
		}
		pod, err := lister.Pods(ips[i].Namespace).Get(ips[i].PodName)
		if err != nil || pod == nil {
			ips[i].Status = "Deleted"
			continue
		}
		ips[i].Status = string(pod.Status.Phase)
		// On public cloud, we can't release exist pod's ip, because we need to call unassign ip first
		// TODO while on private environment, we can
		ips[i].Releasable = false
	}
	return nil
}

var finishedStateMap = sets.NewString(string(corev1.PodFailed), string(corev1.PodSucceeded), "Completed", "Terminated")

func isFinishedState(state string) bool {
	return finishedStateMap.Has(state)
}

func format(sortParam string, page, size int, ret []FloatingIP) *pageutil.Page {
	sort.Sort(bySortParam{array: ret, lessFunc: sortFunc(sortParam)})
	start, end, pagin := pageutil.Pagination(page, size, len(ret))
	pagin.Content = ret[start:end]
	return pagin
}

type bySortParam struct {
	lessFunc func(a, b int, array []FloatingIP) bool
	array    []FloatingIP
}

func (by bySortParam) Less(a, b int) bool {
	return by.lessFunc(a, b, by.array)
}

func (by bySortParam) Swap(a, b int) {
	by.array[a], by.array[b] = by.array[b], by.array[a]
}

func (by bySortParam) Len() int {
	return len(by.array)
}

func sortFunc(sort string) func(a, b int, array []FloatingIP) bool {
	switch strings.ToLower(sort) {
	case "project":
		fallthrough
	case "namespace asc":
		return func(a, b int, array []FloatingIP) bool {
			return array[a].Namespace < array[b].Namespace
		}
	case "namespace desc":
		return func(a, b int, array []FloatingIP) bool {
			return array[a].Namespace > array[b].Namespace
		}
	case "podname":
		fallthrough
	case "podname asc":
		return func(a, b int, array []FloatingIP) bool {
			return array[a].PodName < array[b].PodName
		}
	case "podname desc":
		return func(a, b int, array []FloatingIP) bool {
			return array[a].PodName > array[b].PodName
		}
	case "policy":
		fallthrough
	case "policy asc":
		return func(a, b int, array []FloatingIP) bool {
			return array[a].Policy < array[b].Policy
		}
	case "policy desc":
		return func(a, b int, array []FloatingIP) bool {
			return array[a].Policy > array[b].Policy
		}
	case "ip desc":
		return func(a, b int, array []FloatingIP) bool {
			return array[a].IP > array[b].IP
		}
	case "ip":
		fallthrough
	case "ip asc":
		fallthrough
	default:
		return func(a, b int, array []FloatingIP) bool {
			return array[a].IP < array[b].IP
		}
	}
}

type ReleaseIPReq struct {
	IPs []FloatingIP `json:"ips"`
}

func (c *Controller) ReleaseIPs(req *restful.Request, resp *restful.Response) {
	var releaseIPReq ReleaseIPReq
	if err := req.ReadEntity(&releaseIPReq); err != nil {
		httputil.BadRequest(resp, err)
		return
	}
	expectIPtoKey := make(map[uint32]string)
	for i := range releaseIPReq.IPs {
		temp := releaseIPReq.IPs[i]
		ip := net.ParseIP(temp.IP)
		if ip == nil {
			httputil.BadRequest(resp, fmt.Errorf("%q is not a valid ip", temp.IP))
			return
		}
		ipInt := nets.IPToInt(ip)
		keyObj := util.NewKeyObj(temp.IsDeployment, temp.Namespace, temp.AppName, temp.PodName, temp.PoolName)
		expectIPtoKey[ipInt] = keyObj.KeyInDB
	}
	if err := fillReleasableAndStatus(c.podLister, releaseIPReq.IPs); err != nil {
		httputil.BadRequest(resp, err)
		return
	}
	for _, ip := range releaseIPReq.IPs {
		if !ip.Releasable {
			httputil.BadRequest(resp, fmt.Errorf("%s is not releasable", ip.IP))
			return
		}
	}
	if err := batchDeleteIPs(expectIPtoKey, c.ipam, c.secondIpam); err != nil {
		httputil.BadRequest(resp, err)
		return
	}
	httputil.Ok(resp)
}

type logObj struct {
	IP  net.IP
	Key string
}

func listIPs(keyword string, ipam floatingip.IPAM, secondIpam floatingip.IPAM) ([]FloatingIP, error) {
	var resp []FloatingIP
	fips, err := ipam.GetAllIPs(keyword)
	if err != nil {
		return resp, err
	}
	for _, fip := range fips {
		keyObj := util.ParseKey(fip.Key)
		tmp := FloatingIP{
			IP:           strconv.FormatUint(uint64(fip.IP), 10),
			Namespace:    keyObj.Namespace,
			AppName:      keyObj.AppName,
			PodName:      keyObj.PodName,
			PoolName:     keyObj.PoolName,
			IsDeployment: keyObj.IsDeployment,
			Policy:       fip.Policy,
			attr:         fip.Attr,
		}
		resp = append(resp, tmp)
	}
	if secondIpam != nil {
		secondFips, err := secondIpam.GetAllIPs(keyword)
		if err != nil {
			return resp, err
		}
		for _, fip := range secondFips {
			keyObj := util.ParseKey(fip.Key)
			tmp := FloatingIP{
				IP:           strconv.FormatUint(uint64(fip.IP), 10),
				Namespace:    keyObj.Namespace,
				AppName:      keyObj.AppName,
				PodName:      keyObj.PodName,
				PoolName:     keyObj.PoolName,
				IsDeployment: keyObj.IsDeployment,
				Policy:       fip.Policy,
				attr:         fip.Attr,
			}
			resp = append(resp, tmp)
		}
	}
	return resp, nil
}

func batchDeleteIPs(ipToKey map[uint32]string, ipam floatingip.IPAM, secondIpam floatingip.IPAM) error {
	deletedIP, err := ipam.ReleaseIPs(ipToKey)
	if len(deletedIP) > 0 {
		glog.Infof("releaseIPs %v", deletedIP)
	}
	if err != nil {
		return err
	}
	if secondIpam != nil {
		deletedIP2, err := secondIpam.ReleaseIPs(ipToKey)
		if len(deletedIP2) > 0 {
			glog.Infof("releaseIPs in second IPAM %v", deletedIP2)
		}
		if err != nil {
			if !(strings.Contains(err.Error(), "Table") && strings.Contains(err.Error(), "doesn't exist")) {
				return err
			}
		}
	}
	return nil
}
