package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"git.code.oa.com/gaia/tapp-controller/pkg/client/clientset/versioned"
	tappInformers "git.code.oa.com/gaia/tapp-controller/pkg/client/informers/externalversions"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/eventhandler"
	"git.code.oa.com/gaiastack/galaxy/pkg/api/k8s/schedulerapi"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/schedulerplugin"
	"git.code.oa.com/gaiastack/galaxy/pkg/ipam/server/options"
	"github.com/emicklei/go-restful"
	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
)

type JsonConf struct {
	SchedulePluginConf schedulerplugin.Conf `json:"schedule_plugin"`
}

const COMPONENT_NAME = "galaxy-ipam"

type Server struct {
	JsonConf
	*options.ServerRunOptions
	client               *kubernetes.Clientset
	tappClient           *versioned.Clientset
	plugin               *schedulerplugin.FloatingIPPlugin
	informerFactory      informers.SharedInformerFactory
	tappInformerFactory  tappInformers.SharedInformerFactory
	stopChan             chan struct{}
	leaderElectionConfig *leaderelection.LeaderElectionConfig
}

func NewServer() *Server {
	return &Server{
		ServerRunOptions: options.NewServerRunOptions(),
		stopChan:         make(chan struct{}),
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

	s.informerFactory = informers.NewFilteredSharedInformerFactory(s.client, time.Minute, v1.NamespaceAll, nil)
	s.tappInformerFactory = tappInformers.NewSharedInformerFactory(s.tappClient, time.Minute)
	podInformer := s.informerFactory.Core().V1().Pods()
	statefulsetInformer := s.informerFactory.Apps().V1().StatefulSets()
	tappInformer := s.tappInformerFactory.Tappcontroller().V1alpha1().TApps()
	deploymentInformer := s.informerFactory.Apps().V1().Deployments()
	pluginArgs := &schedulerplugin.PluginFactoryArgs{
		PodLister:         podInformer.Lister(),
		TAppLister:        tappInformer.Lister(),
		StatefulSetLister: statefulsetInformer.Lister(),
		DeploymentLister:  deploymentInformer.Lister(),
		Client:            s.client,
		TAppClient:        s.tappClient,
		PodHasSynced:      podInformer.Informer().HasSynced,
		TAppHasSynced:     tappInformer.Informer().HasSynced,
		StatefulSetSynced: statefulsetInformer.Informer().HasSynced,
		DeploymentSynced:  deploymentInformer.Informer().HasSynced,
	}
	s.plugin, err = schedulerplugin.NewFloatingIPPlugin(s.SchedulePluginConf, pluginArgs)
	if err != nil {
		return err
	}
	podInformer.Informer().AddEventHandler(eventhandler.NewPodEventHandler(s.plugin))
	return nil
}

func (s *Server) Start() error {
	if err := s.init(); err != nil {
		return fmt.Errorf("init server: %v", err)
	}
	if s.LeaderElection.LeaderElect && s.leaderElectionConfig != nil {
		leaderelection.RunOrDie(*s.leaderElectionConfig)
		return nil
	}
	return s.Run()
}

func (s *Server) Run() error {
	if err := s.plugin.Init(); err != nil {
		return err
	}
	s.plugin.Run(s.stopChan)
	go s.informerFactory.Start(s.stopChan)
	go s.tappInformerFactory.Start(s.stopChan)
	s.startServer()
	return nil
}

func (s *Server) initk8sClient() {
	cfg, err := clientcmd.BuildConfigFromFlags(s.Master, s.KubeConf)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}
	cfg.QPS = 1000.0
	cfg.Burst = 2000
	glog.Infof("QPS: %e, Burst: %d", cfg.QPS, cfg.Burst)

	s.client, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %v", err)
	}
	glog.Infof("connected to apiserver %v", cfg)

	s.tappClient, err = versioned.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building example clientset: %v", err)
	}
	recorder, err := newRecoder(cfg)
	if err != nil {
		glog.Fatalf("failed init event recorder: %v", err)
	}
	if s.LeaderElection.LeaderElect {
		leaderElectionClient := kubernetes.NewForConfigOrDie(restclient.AddUserAgent(cfg, "leader-election"))
		rl, err := resourcelock.New(s.LeaderElection.ResourceLock,
			"kube-system",
			COMPONENT_NAME,
			leaderElectionClient.CoreV1(),
			resourcelock.ResourceLockConfig{
				Identity:      fmt.Sprintf("%s:%d", s.Bind, s.Port),
				EventRecorder: recorder,
			})
		if err != nil {
			glog.Fatalf("error creating lock: %v", err)
		}
		s.leaderElectionConfig = &leaderelection.LeaderElectionConfig{
			Lock:          rl,
			LeaseDuration: s.LeaderElection.LeaseDuration.Duration,
			RenewDeadline: s.LeaderElection.RenewDeadline.Duration,
			RetryPeriod:   s.LeaderElection.RetryPeriod.Duration,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(<-chan struct{}) {
					if err := s.Run(); err != nil {
						glog.Fatal(err)
					}
				},
				OnStoppedLeading: func() {
					glog.Fatalf("leaderelection lost")
				},
			},
		}
	}
}

func newRecoder(kubeCfg *restclient.Config) (record.EventRecorder, error) {
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	kubeClient, err := kubernetes.NewForConfig(kubeCfg)
	if err != nil {
		return nil, err
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: COMPONENT_NAME}), nil
}

func (s *Server) startServer() {
	ws := new(restful.WebService)
	ws.
		Path("/v1").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)
	ws.Route(ws.POST("/filter").To(s.filter).Reads(schedulerapi.ExtenderArgs{}).Writes(schedulerapi.ExtenderFilterResult{}))
	ws.Route(ws.POST("/priority").To(s.priority).Reads(schedulerapi.ExtenderArgs{}).Writes(schedulerapi.HostPriorityList{}))
	ws.Route(ws.POST("/bind").To(s.bind).Reads(schedulerapi.ExtenderBindingArgs{}).Writes(schedulerapi.ExtenderBindingResult{}))
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
