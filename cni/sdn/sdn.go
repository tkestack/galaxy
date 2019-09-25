package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"

	galaxyapi "git.code.oa.com/tkestack/galaxy/pkg/api/galaxy"
	"git.code.oa.com/tkestack/galaxy/pkg/api/galaxy/private"
	"github.com/containernetworking/cni/pkg/skel"
	t020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/version"
)

type cniPlugin struct {
	socketPath string
}

func NewCNIPlugin(socketPath string) *cniPlugin {
	return &cniPlugin{socketPath: socketPath}
}

// Create and fill a CNIRequest with this plugin's environment and stdin which
// contain the CNI variables and configuration
func newCNIRequest(args *skel.CmdArgs) *galaxyapi.CNIRequest {
	envMap := make(map[string]string)
	for _, item := range os.Environ() {
		idx := strings.Index(item, "=")
		if idx > 0 {
			envMap[strings.TrimSpace(item[:idx])] = item[idx+1:]
		}
	}

	return &galaxyapi.CNIRequest{
		Env:    envMap,
		Config: args.StdinData,
	}
}

// Send a CNI request to the CNI server via JSON + HTTP over a root-owned unix socket,
// and return the result
func (p *cniPlugin) doCNI(url string, req *galaxyapi.CNIRequest) ([]byte, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CNI request %v: %v", req, err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(proto, addr string) (net.Conn, error) {
				return net.Dial("unix", p.socketPath)
			},
		},
	}

	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to send CNI request: %v", err)
	}
	defer resp.Body.Close() // nolint: errcheck

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read CNI result: %v", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("galaxy returns: %s", string(body))
	}

	return body, nil
}

// Send the ADD command environment and config to the CNI server, returning
// the IPAM result to the caller
func (p *cniPlugin) CmdAdd(args *skel.CmdArgs) (*t020.Result, error) {
	body, err := p.doCNI("http://dummy/cni", newCNIRequest(args))
	if err != nil {
		return nil, err
	}

	result := &t020.Result{}
	if err := json.Unmarshal(body, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response '%s': %v", string(body), err)
	}

	return result, nil
}

// Send the ADD command environment and config to the CNI server, printing
// the IPAM result to stdout when called as a CNI plugin
func (p *cniPlugin) skelCmdAdd(args *skel.CmdArgs) error {
	result, err := p.CmdAdd(args)
	if err != nil {
		return err
	}
	return result.Print()
}

// Send the DEL command environment and config to the CNI server
func (p *cniPlugin) CmdDel(args *skel.CmdArgs) error {
	_, err := p.doCNI("http://dummy/cni", newCNIRequest(args))
	return err
}

func main() {
	p := NewCNIPlugin(private.GalaxySocketPath)
	skel.PluginMain(p.skelCmdAdd, p.CmdDel, version.Legacy)
}
