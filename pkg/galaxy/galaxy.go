package galaxy

import (
	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/gc"
)

type Galaxy struct {
	quitChannels []chan error
	cleaner      gc.GC
}

func NewGalaxy() (*Galaxy, error) {
	dockerClient, err := docker.NewDockerInterface()
	if err != nil {
		return nil, err
	}
	g := &Galaxy{}
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
	return nil
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
