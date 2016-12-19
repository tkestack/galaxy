package galaxy

import (
	"encoding/json"
	"fmt"
	"strings"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/flags"
	"git.code.oa.com/gaiastack/galaxy/pkg/gc"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/kernel"
	"github.com/golang/glog"
)

type Galaxy struct {
	quitChannels []chan error
	cleaner      gc.GC
	netConf      map[string]map[string]interface{}
	flannelConf  []byte
}

func NewGalaxy() (*Galaxy, error) {
	dockerClient, err := docker.NewDockerInterface()
	if err != nil {
		return nil, err
	}
	g := &Galaxy{}
	if err := g.parseConfig(); err != nil {
		return nil, err
	}
	g.cleaner = gc.NewFlannelGC(dockerClient, g.newQuitChannel(), g.newQuitChannel())
	return g, nil
}

func (g *Galaxy) newQuitChannel() chan error {
	quitChannel := make(chan error)
	g.quitChannels = append(g.quitChannels, quitChannel)
	return quitChannel
}

func (g *Galaxy) Start() error {
	g.cleaner.Run()
	kernel.BridgeNFCallIptables(g.newQuitChannel())
	return g.startServer()
}

func (g *Galaxy) Stop() error {
	// Stop and wait on all quit channels.
	for i, c := range g.quitChannels {
		// Send the exit signal and wait on the thread to exit (by closing the channel).
		c <- nil
		err := <-c
		if err != nil {
			// Remove the channels that quit successfully.
			g.quitChannels = g.quitChannels[i:]
			return err
		}
	}
	g.quitChannels = make([]chan error, 0)
	return nil
}

func (g *Galaxy) parseConfig() error {
	if strings.TrimSpace(*flagNetworkConf) == "" {
		return fmt.Errorf("No network configured")
	}
	var networkConf map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(*flagNetworkConf), &networkConf); err != nil {
		return fmt.Errorf("Error unmarshal network config %s: %v", *flagNetworkConf, err)
	}
	if len(networkConf) == 0 {
		return fmt.Errorf("No network configured")
	} else {
		g.netConf = networkConf
		glog.Infof("Network config %s", *flagNetworkConf)
	}
	for k, v := range g.netConf {
		if v == nil {
			g.netConf[k] = make(map[string]interface{})
		}
		g.netConf[k]["type"] = k
	}
	if _, ok := g.netConf["galaxy-k8s-vlan"]; ok {
		g.netConf["galaxy-k8s-vlan"]["url"] = *flagMaster
		g.netConf["galaxy-k8s-vlan"]["node_ip"] = flags.GetNodeIP()
	}
	if _, ok := g.netConf["galaxy-flannel"]; ok {
		var err error
		if g.flannelConf, err = json.Marshal(g.netConf["galaxy-flannel"]); err != nil {
			return err
		}
	}
	return nil
}
