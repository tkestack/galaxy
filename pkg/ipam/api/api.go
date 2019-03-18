package api

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/schedulerplugin/util"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/database"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/httputil"
	"git.code.oa.com/gaiastack/galaxy/pkg/utils/nets"
	pageutil "git.code.oa.com/gaiastack/galaxy/pkg/utils/page"
	"github.com/emicklei/go-restful"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/listers/core/v1"
)

type Controller struct {
	DB        *database.DBRecorder
	PodLister v1.PodLister
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
	fips, err := database.FloatingIPsByKeyword(c.DB, database.DefaultFloatingipTableName, keyword)
	if err != nil {
		httputil.InternalError(resp, err)
		return
	}
	secondIPs, err := database.FloatingIPsByKeyword(c.DB, database.SecondFloatingipTableName, keyword)
	if err != nil {
		if !(strings.Contains(err.Error(), "Table") && strings.Contains(err.Error(), "doesn't exist")) {
			httputil.InternalError(resp, err)
			return
		}
	}
	ret := transform(fips, secondIPs)
	sortParam, page, size := pageutil.PagingParams(req)
	pagin := format(sortParam, page, size, ret)
	// filling other fields after paging benefits api performance
	if err := c.fillReleasableAndStatus(ret); err != nil {
		httputil.InternalError(resp, err)
		return
	}
	resp.WriteEntity(*pagin) // nolint: errcheck
}

func transform(fips, secondIPs []database.FloatingIP) []FloatingIP {
	var ret []FloatingIP
	for _, fip := range [][]database.FloatingIP{fips, secondIPs} {
		for i := range fip {
			keyObj := util.ParseKey(fip[i].Key)
			ret = append(ret, FloatingIP{IP: nets.IntToIP(fip[i].IP).String(),
				Namespace:    keyObj.Namespace,
				AppName:      keyObj.AppName,
				PodName:      keyObj.PodName,
				PoolName:     keyObj.PoolName,
				IsDeployment: keyObj.IsDeployment,
				Policy:       fip[i].Policy,
				UpdateTime:   fip[i].UpdatedAt.Unix(),
				attr:         fip[i].Attr})
		}
	}
	return ret
}

func (c *Controller) fillReleasableAndStatus(ips []FloatingIP) error {
	for i := range ips {
		ips[i].Releasable = true
		if ips[i].PodName == "" {
			continue
		}
		pod, err := c.PodLister.Pods(ips[i].Namespace).Get(ips[i].PodName)
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
	allIPs := make([]uint32, len(releaseIPReq.IPs))
	for i := range releaseIPReq.IPs {
		temp := releaseIPReq.IPs[i]
		ip := net.ParseIP(temp.IP)
		if ip == nil {
			httputil.BadRequest(resp, fmt.Errorf("%q is not a valid ip", temp.IP))
			return
		}
		allIPs[i] = nets.IPToInt(ip)
		keyObj := util.NewKeyObj(temp.IsDeployment, temp.Namespace, temp.AppName, temp.PodName, temp.PoolName)
		expectIPtoKey[allIPs[i]] = keyObj.KeyInDB
	}
	fipsInDB, err := database.FloatingIPsByIPS(c.DB, database.DefaultFloatingipTableName, allIPs)
	if err != nil {
		httputil.InternalError(resp, err)
		return
	}
	secondFipsInDB, err := database.FloatingIPsByIPS(c.DB, database.SecondFloatingipTableName, allIPs)
	if err != nil {
		httputil.InternalError(resp, err)
		return
	}
	var filteredFIP, filteredSecondIP []database.FloatingIP
	for i := range fipsInDB {
		if expectKey, exist := expectIPtoKey[fipsInDB[i].IP]; exist {
			if expectKey == fipsInDB[i].Key {
				// check if ip got released and reallocated to another pod, if it does, we should not release it
				filteredFIP = append(filteredFIP, fipsInDB[i])
			}
		}
	}
	for i := range secondFipsInDB {
		if expectKey, exist := expectIPtoKey[secondFipsInDB[i].IP]; exist {
			if expectKey == secondFipsInDB[i].Key {
				// check if ip got released and reallocated to another pod, if it does, we should not release it
				filteredSecondIP = append(filteredSecondIP, secondFipsInDB[i])
			}
		}
	}
	transformed := transform(filteredFIP, filteredSecondIP)
	if err := c.fillReleasableAndStatus(transformed); err != nil {
		httputil.InternalError(resp, err)
		return
	}
	for i := range transformed {
		if !transformed[i].Releasable {
			httputil.BadRequest(resp, fmt.Errorf("%s is unreleasable, status %s", transformed[i].IP, transformed[i].Status))
			return
		}
	}
	if err := database.ReleaseFloatingIPs(c.DB, filteredFIP, filteredSecondIP); err != nil {
		httputil.InternalError(resp, err)
		return
	}
	httputil.Ok(resp)
}
