package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"git.code.oa.com/gaiastack/galaxy/cni/zhiyun-ipam/api"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/version"
)

func cmdAdd(args *skel.CmdArgs) error {
	conf, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}
	resp, err := api.Allocate(conf)
	if err != nil {
		return err
	}
	if _, err := reserve(args.ContainerID, resp.IP); err != nil {
		api.Recycle(conf, resp.IP) // try to do recycle
		return fmt.Errorf("failed to store ip on disk: %v", err)
	}
	return (&t020.Result{
		IP4: &t020.IPConfig{
			IP:      net.IPNet{IP: resp.IP, Mask: resp.Mask},
			Gateway: resp.Gateway,
			Routes: []types.Route{{
				Dst: net.IPNet{
					IP:   net.IPv4(0, 0, 0, 0),
					Mask: net.IPv4Mask(0, 0, 0, 0),
				},
			}},
		},
	}).Print()
}

func cmdDel(args *skel.CmdArgs) error {
	conf, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}
	ip, err := releaseByID(args.ContainerID)
	if err != nil {
		return fmt.Errorf("failed to release ip file on disk: %v", err)
	}
	return api.Recycle(conf, ip)
}

func loadConf(bytes []byte) (*api.Conf, error) {
	n := &api.Conf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load vethconf: %v", err)
	}
	return n, nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.Legacy)
}

func reserve(id string, ip net.IP) (bool, error) {
	if err := os.MkdirAll(api.DefaultDataDir, 0755); err != nil {
		return false, err
	}
	fname := filepath.Join(api.DefaultDataDir, ip.String())
	f, err := os.OpenFile(fname, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0644)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(id); err != nil {
		f.Close()
		os.Remove(f.Name())
		return false, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return false, err
	}
	return true, nil
}

// N.B. This function eats errors to be tolerant and
// release as much as possible
func releaseByID(id string) (net.IP, error) {
	var ip net.IP
	err := filepath.Walk(api.DefaultDataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}
		if string(data) == id {
			if err := os.Remove(path); err != nil {
				return nil
			}
		}
		ip = net.ParseIP(info.Name())
		return nil
	})
	return ip, err
}
