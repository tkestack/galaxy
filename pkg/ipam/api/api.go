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
	"sort"
	"strings"
	"time"

	"github.com/emicklei/go-restful"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/listers/core/v1"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/floatingip"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
	"tkestack.io/galaxy/pkg/utils/httputil"
	pageutil "tkestack.io/galaxy/pkg/utils/page"
)

// Controller is the API controller
type Controller struct {
	ipam        floatingip.IPAM
	releaseFunc func(r *schedulerplugin.ReleaseRequest) error
	podLister   v1.PodLister
}

// NewController construct a controller object
func NewController(
	ipam floatingip.IPAM, lister v1.PodLister, releaseFunc func(r *schedulerplugin.ReleaseRequest) error) *Controller {
	return &Controller{
		ipam:        ipam,
		podLister:   lister,
		releaseFunc: releaseFunc,
	}
}

// FloatingIP is the floating ip info
type FloatingIP struct {
	IP         string            `json:"ip"`
	Namespace  string            `json:"namespace,omitempty"`
	AppName    string            `json:"appName,omitempty"`
	PodName    string            `json:"podName,omitempty"`
	PoolName   string            `json:"poolName,omitempty"`
	Policy     uint16            `json:"policy"`
	AppType    string            `json:"appType,omitempty"`
	UpdateTime time.Time         `json:"updateTime,omitempty"`
	Status     string            `json:"status,omitempty"`
	Releasable bool              `json:"releasable,omitempty"`
	labels     map[string]string `json:"-"`
}

// SwaggerDoc is to generate Swagger docs
func (FloatingIP) SwaggerDoc() map[string]string {
	return map[string]string{
		"ip":         "ip",
		"namespace":  "namespace",
		"appName":    "deployment or statefulset name",
		"podName":    "pod name",
		"policy":     "ip release policy",
		"appType":    "deployment, statefulset or tapp, default statefulset",
		"updateTime": "last allocate or release time of this ip",
		"status":     "pod status if exists",
		"releasable": "if the ip is releasable. An ip is releasable if it isn't belong to any pod",
	}
}

// ListIPResp is the ListIPs response
type ListIPResp struct {
	pageutil.Page
	Content []FloatingIP `json:"content,omitempty"`
}

// ListIPs lists floating ips
func (c *Controller) ListIPs(req *restful.Request, resp *restful.Response) {
	keyword := req.QueryParameter("keyword")
	key := keyword
	fuzzyQuery := true
	if keyword == "" {
		fuzzyQuery = false
		poolName := req.QueryParameter("poolName")
		appName := req.QueryParameter("appName")
		podName := req.QueryParameter("podName")
		namespace := req.QueryParameter("namespace")
		appType := req.QueryParameter("appType")
		var appTypePrefix string
		if appType == "" {
			appTypePrefix = util.StatefulsetPrefixKey
		} else {
			appTypePrefix = util.GetAppTypePrefix(appType)
		}
		if appTypePrefix == "" {
			httputil.BadRequest(resp, fmt.Errorf("invalid appType %s", appType))
			return
		}
		key = util.NewKeyObj(appTypePrefix, namespace, appName, podName, poolName).KeyInDB
	}
	glog.V(4).Infof("list ips by %s, fuzzyQuery %v", key, fuzzyQuery)
	fips, err := listIPs(key, c.ipam, fuzzyQuery)
	if err != nil {
		httputil.InternalError(resp, err)
		return
	}
	sortParam, page, size := pageutil.PagingParams(req)
	sort.Sort(bySortParam{array: fips, lessFunc: sortFunc(sortParam)})
	start, end, pagin := pageutil.Pagination(page, size, len(fips))
	pagedFips := fips[start:end]
	for i := range pagedFips {
		releasable, status := c.checkReleasableAndStatus(&pagedFips[i])
		pagedFips[i].Status = status
		pagedFips[i].Releasable = releasable
	}
	resp.WriteEntity(ListIPResp{Page: *pagin, Content: pagedFips}) // nolint: errcheck
}

func (c *Controller) checkReleasableAndStatus(fip *FloatingIP) (releasable bool, status string) {
	if fip.labels != nil {
		if _, ok := fip.labels[constant.ReserveFIPLabel]; ok {
			return
		}
	}
	if fip.PodName == "" && fip.AppName == "" && fip.PoolName == "" {
		return
	}
	if fip.PodName == "" {
		releasable = true
		status = "Deleted"
		return
	}
	pod, err := c.podLister.Pods(fip.Namespace).Get(fip.PodName)
	if err == nil {
		status = string(pod.Status.Phase)
		return
	}
	if errors.IsNotFound(err) {
		releasable = true
		status = "Deleted"
	} else {
		status = "Unknown"
	}
	return
}

// bySortParam defines sort funcs for FloatingIP array
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

// sortFunc defines the sort algorithm
// #lizard forgives
func sortFunc(sort string) func(a, b int, array []FloatingIP) bool {
	switch strings.ToLower(sort) {
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

// ReleaseIPReq is the request to release ips
type ReleaseIPReq struct {
	IPs []FloatingIP `json:"ips"`
}

// ReleaseIPResp is the response of release ip
type ReleaseIPResp struct {
	httputil.Resp
	Unreleased []string `json:"unreleased,omitempty"`
	// Reason is the reason why this ip is not released
	Reason []string `json:"reasons,omitempty"`
}

// SwaggerDoc generates swagger doc for release ip response
func (ReleaseIPResp) SwaggerDoc() map[string]string {
	return map[string]string{
		"unreleased": "unreleased ips, have been released or allocated to other pods, or are not within valid range",
	}
}

// ReleaseIPs releases floating ips
// #lizard forgives
func (c *Controller) ReleaseIPs(req *restful.Request, resp *restful.Response) {
	var releaseIPReq ReleaseIPReq
	if err := req.ReadEntity(&releaseIPReq); err != nil {
		httputil.BadRequest(resp, err)
		return
	}
	var (
		released, unreleasedIP, reasons []string
		unbindRequests                  []*schedulerplugin.ReleaseRequest
	)
	for i := range releaseIPReq.IPs {
		temp := releaseIPReq.IPs[i]
		ip := net.ParseIP(temp.IP)
		if ip == nil {
			httputil.BadRequest(resp, fmt.Errorf("%q is not a valid ip", temp.IP))
			return
		}
		var appTypePrefix string
		if temp.AppType == "" {
			appTypePrefix = util.StatefulsetPrefixKey
		}
		appTypePrefix = util.GetAppTypePrefix(temp.AppType)
		if appTypePrefix == "" {
			httputil.BadRequest(resp, fmt.Errorf("unknown app type %q", temp.AppType))
			return
		}
		releasable, status := c.checkReleasableAndStatus(&temp)
		if !releasable {
			unreleasedIP = append(unreleasedIP, temp.IP)
			reasons = append(reasons, "releasable is false, pod status "+status)
			continue
		}
		keyObj := util.NewKeyObj(appTypePrefix, temp.Namespace, temp.AppName, temp.PodName, temp.PoolName)
		unbindRequests = append(unbindRequests, &schedulerplugin.ReleaseRequest{IP: ip, KeyObj: keyObj})
	}
	for _, req := range unbindRequests {
		if err := c.releaseFunc(req); err != nil {
			unreleasedIP = append(unreleasedIP, req.IP.String())
			reasons = append(reasons, err.Error())
		} else {
			released = append(released, req.IP.String())
		}
	}
	glog.Infof("releaseIPs %v", released)
	var res *ReleaseIPResp
	if len(unreleasedIP) > 0 {
		res = &ReleaseIPResp{Resp: httputil.NewResp(
			http.StatusAccepted, fmt.Sprintf("Released %d ips, %d ips failed, please check the reasons "+
				"why they failed", len(released), len(unreleasedIP)))}
	} else {
		res = &ReleaseIPResp{Resp: httputil.NewResp(http.StatusOK, "")}
	}
	res.Unreleased = unreleasedIP
	res.Reason = reasons
	resp.WriteHeaderAndEntity(res.Code, res)
}

// listIPs lists ips from ipams
func listIPs(keyword string, ipam floatingip.IPAM, fuzzyQuery bool) ([]FloatingIP, error) {
	var result []FloatingIP
	if fuzzyQuery {
		fips, err := ipam.ByKeyword(keyword)
		if err != nil {
			return nil, err
		}
		for i := range fips {
			result = append(result, convert(&fips[i]))
		}
	} else {
		fips, err := ipam.ByPrefix(keyword)
		if err != nil {
			return nil, err
		}
		for i := range fips {
			result = append(result, convert(&fips[i].FloatingIP))
		}
	}
	return result, nil
}

// convert converts `floatingip.FloatingIP` to `FloatingIP`
func convert(fip *floatingip.FloatingIP) FloatingIP {
	keyObj := util.ParseKey(fip.Key)
	return FloatingIP{IP: fip.IP.String(),
		Namespace:  keyObj.Namespace,
		AppName:    keyObj.AppName,
		PodName:    keyObj.PodName,
		PoolName:   keyObj.PoolName,
		AppType:    util.GetAppType(keyObj.AppTypePrefix),
		Policy:     fip.Policy,
		UpdateTime: fip.UpdatedAt,
		labels:     fip.Labels}
}
