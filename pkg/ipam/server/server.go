package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	galaxycache "git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/cache"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/informer"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/schedulerapi"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/schedulerplugin"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/server/options"
	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
	"k8s.io/client-go/1.4/kubernetes"
	"k8s.io/client-go/1.4/pkg/api/v1"
	gaiav1 "k8s.io/client-go/1.4/pkg/apis/gaia/v1alpha1"
	"k8s.io/client-go/1.4/pkg/fields"
	"k8s.io/client-go/1.4/tools/cache"
	"k8s.io/client-go/1.4/tools/clientcmd"
)

type JsonConf struct {
	SchedulePluginConf schedulerplugin.Conf `json:"schedule_plugin"`
}

type Server struct {
	JsonConf
	*options.ServerRunOptions
	client *kubernetes.Clientset
	*informer.PodInformer
	*informer.TAppInformer
	plugin   *schedulerplugin.FloatingIPPlugin
	stopChan chan struct{}
}

func NewServer() *Server {
	return &Server{
		ServerRunOptions: options.NewServerRunOptions(),
		PodInformer: &informer.PodInformer{
			PodLister:   &galaxycache.StoreToPodLister{},
			PodWatchers: make(map[string]informer.PodWatcher),
		},
		TAppInformer: &informer.TAppInformer{
			TAppLister:   &galaxycache.StoreToTAppLister{},
			TAppWatchers: make(map[string]informer.TAppWatcher),
		},
		stopChan: make(chan struct{}),
	}
}

func (s *Server) init() error {
	if options.JsonConfigPath == "" {
		return fmt.Errorf("json config is required")
	}
	data, err := ioutil.ReadFile(options.JsonConfigPath)
	if err != nil {
		return fmt.Errorf("read json config: %v", err)
	}
	if err := json.Unmarshal(data, &s.JsonConf); err != nil {
		return fmt.Errorf("bad config %s: %v", string(data), err)
	}
	s.initk8sClient()
	s.PodLister.Indexer, s.PodPopulator = cache.NewIndexerInformer(
		s.createAssignedNonTerminatedPodLW(),
		&v1.Pod{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    s.AddPodToCache,
			UpdateFunc: s.UpdatePodInCache,
			DeleteFunc: s.DeletePodFromCache,
		},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	s.TAppLister.Store, s.TAppPopulator = cache.NewInformer(
		s.createTAppLW(),
		&gaiav1.TApp{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    s.AddTApp,
			UpdateFunc: s.UpdateTApp,
			DeleteFunc: s.DeleteTApp,
		},
	)
	s.TAppLister.Store, s.TAppPopulator = cache.NewInformer(
		s.createTAppLW(),
		&gaiav1.TApp{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    s.AddTApp,
			UpdateFunc: s.UpdateTApp,
			DeleteFunc: s.DeleteTApp,
		},
	)
	pluginArgs := &schedulerplugin.PluginFactoryArgs{
		PodLister:     s.PodLister,
		TAppLister:    s.TAppLister,
		Client:        s.client,
		PodHasSynced:  s.PodPopulator.HasSynced,
		TAppHasSynced: s.TAppPopulator.HasSynced,
	}
	s.plugin, err = schedulerplugin.NewFloatingIPPlugin(s.SchedulePluginConf, pluginArgs)
	s.PodWatchers["floatingip"] = s.plugin
	return err
}

func (s *Server) Start() error {
	if err := s.init(); err != nil {
		return fmt.Errorf("init server: %v", err)
	}
	go s.PodPopulator.Run(s.stopChan)
	go s.TAppPopulator.Run(s.stopChan)
	s.startServer()
	return nil
}

func (s *Server) initk8sClient() {
	clientConfig, err := clientcmd.BuildConfigFromFlags(s.Master, s.KubeConf)
	if err != nil {
		glog.Fatalf("Invalid client config: error(%v)", err)
	}
	clientConfig.QPS = 1000.0
	clientConfig.Burst = 2000
	glog.Infof("QPS: %e, Burst: %d", clientConfig.QPS, clientConfig.Burst)

	s.client, err = kubernetes.NewForConfig(clientConfig)
	if err != nil {
		glog.Fatalf("Can not generate client from config: error(%v)", err)
	}
	glog.Infof("connected to apiserver %v", clientConfig)
}

// Returns a cache.ListWatch that finds all pods that are
// already scheduled.
func (s *Server) createAssignedNonTerminatedPodLW() *cache.ListWatch {
	selector := fields.ParseSelectorOrDie("spec.nodeName!=" + "" + ",status.phase!=" + string(v1.PodSucceeded) + ",status.phase!=" + string(v1.PodFailed))
	return cache.NewListWatchFromClient(s.client.CoreClient, "pods", v1.NamespaceAll, selector)
}

// Returns a cache.ListWatch that gets all changes to replicasets.
func (s *Server) createReplicaSetLW() *cache.ListWatch {
	return cache.NewListWatchFromClient(s.client.ExtensionsClient, "replicasets", v1.NamespaceAll, fields.ParseSelectorOrDie(""))
}

// Returns a cache.ListWatch that gets all changes to tapps.
func (s *Server) createTAppLW() *cache.ListWatch {
	return cache.NewListWatchFromClient(s.client.GaiaClient, "tapps", v1.NamespaceAll, fields.ParseSelectorOrDie(""))
}

func (s *Server) startServer() {
	ws := new(restful.WebService)
	ws.
		Path("/v1").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)
	ws.Route(ws.POST("/filter").To(s.filter).Reads(schedulerapi.ExtenderArgs{}).Writes(schedulerapi.ExtenderFilterResult{}))
	ws.Route(ws.POST("/priority").To(s.priority).Reads(schedulerapi.ExtenderArgs{}).Writes(schedulerapi.HostPriorityList{}))
	health := new(restful.WebService)
	health.Route(health.GET("/healthy").To(s.healthy))
	restful.Add(ws)
	restful.Add(health)
	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", s.Bind, s.Port), nil); err != nil {
		glog.Fatalf("unable to listen: %v.", err)
	}
}

func (s *Server) filter(request *restful.Request, response *restful.Response) {
	args := new(schedulerapi.ExtenderArgs)
	if err := request.ReadEntity(&args); err != nil {
		glog.Error(err)
		response.WriteError(http.StatusInternalServerError, err)
		return
	}
	glog.V(4).Infof("POST filter %v", *args)
	filteredNodes, failedNodesMap, err := s.plugin.Filter(&args.Pod, args.Nodes.Items)
	args.Nodes.Items = filteredNodes
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	response.WriteEntity(schedulerapi.ExtenderFilterResult{
		Nodes:       args.Nodes,
		FailedNodes: failedNodesMap,
		Error:       errStr,
	})
}

func (s *Server) priority(request *restful.Request, response *restful.Response) {
	args := new(schedulerapi.ExtenderArgs)
	if err := request.ReadEntity(&args); err != nil {
		glog.Error(err)
		response.WriteError(http.StatusInternalServerError, err)
		return
	}
	glog.V(4).Infof("POST priority %v", *args)
	hostPriorityList, err := s.plugin.Prioritize(&args.Pod, args.Nodes.Items)
	if err != nil {
		glog.Warningf("prioritize err: %v", err)
	}
	response.WriteEntity(*hostPriorityList)
}

func (s *Server) bind(request *restful.Request, response *restful.Response) {
	args := new(schedulerapi.ExtenderBindingArgs)
	if err := request.ReadEntity(&args); err != nil {
		glog.Error(err)
		response.WriteError(http.StatusInternalServerError, err)
		return
	}
	glog.V(4).Infof("POST bind %v", *args)
	err := s.plugin.Bind(args)
	var result schedulerapi.ExtenderBindingResult
	if err != nil {
		glog.Warningf("bind err: %v", err)
		result.Error = err.Error()
	}
	response.WriteEntity(result)
}

func (s *Server) healthy(request *restful.Request, response *restful.Response) {
	response.WriteHeader(http.StatusOK)
	response.Write([]byte("ok"))
}
