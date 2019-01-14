package gc

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"git.code.oa.com/gaiastack/galaxy/cni/zhiyun-ipam/api"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/docker"
	"git.code.oa.com/gaiastack/galaxy/pkg/wait"
	"github.com/golang/glog"
)

type ZhiyunGC struct {
	conf      *api.Conf
	dockerCli *docker.DockerInterface
	quit      chan error
}

func NewZhiyunGC(dockerCli *docker.DockerInterface, quit chan error, conf *api.Conf) GC {
	return &ZhiyunGC{
		dockerCli: dockerCli,
		quit:      quit,
		conf:      conf,
	}
}

func (gc *ZhiyunGC) Run() {
	go wait.UntilQuitSignal("zhiyun gc cleanup ip", func() {
		if err := cleanupZhiYunIP(gc.conf, gc.dockerCli); err != nil {
			glog.Errorf("Error executing zhiyun gc cleanup ip %v", err)
		}
	}, *flagFlannelGCInterval, gc.quit)
}

func cleanupZhiYunIP(conf *api.Conf, dockerCli *docker.DockerInterface) error {
	fis, err := ioutil.ReadDir(api.DefaultDataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, fi := range fis {
		ip := net.ParseIP(fi.Name())
		if len(ip) == 0 {
			continue
		}
		ipFile := filepath.Join(api.DefaultDataDir, fi.Name())
		containerIdData, err := ioutil.ReadFile(ipFile)
		if os.IsNotExist(err) || len(containerIdData) == 0 {
			continue
		}
		containerId := string(containerIdData)
		if err != nil {
			if !os.IsNotExist(err) {
				glog.Warningf("Error read file %s: %v", fi.Name(), err)
			}
			continue
		}
		if _, err := dockerCli.InspectContainer(containerId); err != nil {
			if _, ok := err.(docker.ContainerNotFoundError); ok {
				if err := api.Recycle(conf, ip); err != nil {
					glog.Warningf("failed to Recycle zhiyun ip %s: %v", ip.String(), err)
					continue
				}
				if err := os.Remove(ipFile); err != nil && !os.IsNotExist(err) {
					glog.Warningf("Error deleting leaky ip file %s/%s container: %v", ipFile, containerId, err)
				} else {
					if err == nil {
						glog.Infof("Deleted leaky ip file %s container %s", ipFile, containerId)
					}
				}
			} else {
				glog.Warningf("Error inspect container %s: %v", containerId, err)
			}
		}
	}
	return nil
}
