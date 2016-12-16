package galaxy

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/cniutil"
	galaxyapi "git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy"
	"git.code.oa.com/gaiastack/galaxy/pkg/network/flannel"
	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
)

func (g *Galaxy) startServer() error {
	g.installHandlers()
	if err := os.Remove(galaxyapi.GalaxySocketPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %v", galaxyapi.GalaxySocketPath, err)
		}
	}
	l, err := net.Listen("unix", galaxyapi.GalaxySocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on pod info socket: %v", err)
	}
	if err := os.Chmod(galaxyapi.GalaxySocketPath, 0600); err != nil {
		l.Close()
		return fmt.Errorf("failed to set pod info socket mode: %v", err)
	}

	glog.Fatal(http.Serve(l, nil))
	return nil
}

func (g *Galaxy) installHandlers() {
	ws := new(restful.WebService)
	ws.Route(ws.GET("/cni").To(g.cni))
	ws.Route(ws.POST("/cni").To(g.cni))
	restful.Add(ws)
}

func (g *Galaxy) cni(r *restful.Request, w *restful.Response) {
	req, err := galaxyapi.CniRequestToPodRequest(r.Request)
	if err != nil {
		glog.Warningf("bad request %v", err)
		http.Error(w, fmt.Sprintf("%v", err), http.StatusBadRequest)
		return
	}
	result, err := g.requestFunc(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("%v", err), http.StatusBadRequest)
	} else {
		// Empty response JSON means success with no body
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(result); err != nil {
			glog.Warningf("Error writing %s HTTP response: %v", req.Command, err)
		}
	}
}

func (g *Galaxy) requestFunc(req *galaxyapi.PodRequest) (data []byte, err error) {
	start := time.Now()
	glog.Infof("%v, %s+", req, start.Format(time.StampMicro))
	if req.Command == cniutil.COMMAND_ADD {
		defer func() {
			glog.Infof("%v, data %s, err %v, %s-", req, string(data), err, start.Format(time.StampMicro))
		}()
		result, err1 := flannel.CmdAdd(req.CmdArgs)
		if err1 != nil {
			err = err1
		} else {
			if result != nil {
				data, err = json.Marshal(result)
			}
		}
	} else if req.Command == cniutil.COMMAND_DEL {
		defer glog.Infof("%v err %v, %s-", req, err, start.Format(time.StampMicro))
		err = flannel.CmdDel(req.CmdArgs)
	} else {
		err = fmt.Errorf("unkown command %s", req.Command)
	}
	return
}
