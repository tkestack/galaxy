package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/utils/ips"
	"github.com/parnurzeal/gorequest"
)

type AllocateRequest struct {
	Operator string     `json:"operator"`
	HostList []HostList `json:"hostList"`
}

type HostList struct {
	HostIP string `json:"hostIP"`
	Number string `json:"number"`
}

type AllocateResponse struct {
	Code int
	Msg  string
	Data []AllocateResponseData `json:"data"`
}

type AllocateResponseData struct {
	Detail  string
	Result  int
	Gateway string `json:"svr_allocate_gateway"`
	IPs     string `json:"svr_allocate_ip"`
	Mask    string `json:"svr_allocate_mask"`
	Assert  string `json:"svr_asset_id"`
}

type RecycleRequest struct {
	Operator string   `json:"operator"`
	IPList   []string `json:"ipList"`
}

type RecycleResponse struct {
	Code int
	Msg  string
	Data []RecycleResponseData `json:"data"`
}

type RecycleResponseData struct {
	Detail string
	Result int
	IP     string `json:"svr_ip"`
	Assert string `json:"svr_asset_id"`
}

type Conf struct {
	Url         string `json:"url"`
	AllocateURI string `json:"allocate_uri"`
	RecycleURI  string `json:"recycle_uri"`
	NodeIP      string `json:"node_ip"`
	Operator    string `json:"operator"`
}

func newRequest() *gorequest.SuperAgent {
	return gorequest.New().Timeout(time.Minute)
}

type FormatResponse struct {
	IP      net.IP
	Mask    net.IPMask
	Gateway net.IP
}

func Allocate(conf *Conf) (*FormatResponse, error) {
	var result AllocateResponse
	resp, _, errs := newRequest().
		Post(conf.Url+conf.AllocateURI).
		Retry(3, time.Second, http.StatusInternalServerError).
		SendStruct(AllocateRequest{Operator: conf.Operator, HostList: []HostList{{HostIP: conf.NodeIP[:strings.IndexAny(conf.NodeIP, "-")], Number: "1"}}}).
		EndStruct(&result)
	if resp.StatusCode != http.StatusOK || len(errs) > 0 {
		return nil, fmt.Errorf("allocate ip failed, zhiyun api status code %d, errs %v", resp.StatusCode, errs)
	}
	if result.Code != 0 {
		return nil, errors.New(result.Msg)
	}
	if len(result.Data) == 0 {
		return nil, errors.New("no result ip returned from zhiyun api")
	}
	data := result.Data[0]
	if data.Result != 0 {
		return nil, errors.New(data.Detail)
	}
	ip := net.ParseIP(data.IPs)
	if ip == nil {
		return nil, fmt.Errorf("allocate ip failed, invalid ip %s", data.IPs)
	}
	mask := ips.ParseIPv4Mask(data.Mask)
	if mask == nil {
		Recycle(conf, ip)
		// no need to do recycle, galaxy will do it
		return nil, fmt.Errorf("allocate ip failed, invalid mask %s", data.Mask)
	}
	gateway := net.ParseIP(data.Gateway)
	if gateway == nil {
		Recycle(conf, ip)
		return nil, fmt.Errorf("allocate ip failed, invalid gateway %s", data.Gateway)
	}
	return &FormatResponse{IP: ip, Mask: mask, Gateway: gateway}, nil
}

func Recycle(conf *Conf, ip net.IP) error {
	var result RecycleResponse
	resp, _, errs := newRequest().
		Post(conf.Url+conf.AllocateURI).
		Retry(3, time.Second, http.StatusInternalServerError).
		SendStruct(RecycleRequest{Operator: conf.Operator, IPList: []string{ip.String()}}).
		EndStruct(&result)
	if resp.StatusCode != http.StatusOK || len(errs) > 0 {
		return fmt.Errorf("recycle ip failed, zhiyun api status code %d, errs %v", resp.StatusCode, errs)
	}
	if result.Code != 0 {
		return errors.New(result.Msg)
	}
	if len(result.Data) == 0 {
		return errors.New("no result ip returned from zhiyun api")
	}
	data := result.Data[0]
	if data.Result != 0 {
		return errors.New(data.Detail)
	}
	return nil
}

var DefaultDataDir = "/var/lib/cni/networks/zhiyun/"
