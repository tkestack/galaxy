package galaxy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/golang/glog"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/flags"
	"git.code.oa.com/gaiastack/galaxy/pkg/gc"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/firewall"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/kernel"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/portmapping"
)

type Galaxy struct {
	quitChannels []chan error
	cleaner      gc.GC
	netConf      map[string]map[string]interface{}
	flannelConf  []byte
	pmhandler    *portmapping.PortMappingHandler
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
	g.pmhandler = portmapping.New()
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
	firewall.SetupEbtables(g.newQuitChannel())
	firewall.EnsureIptables(g.pmhandler, g.newQuitChannel())
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
		if _, ok := g.netConf[k]["type"]; !ok {
			g.netConf[k]["type"] = k
		}
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
	glog.Infof("normalized network config %v", g.netConf)
	return nil
}
