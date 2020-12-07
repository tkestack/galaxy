/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/emicklei/go-restful"
	"github.com/emicklei/go-restful-swagger12"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
	extensionClient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/k8s/eventhandler"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
	"tkestack.io/galaxy/pkg/ipam/api"
	"tkestack.io/galaxy/pkg/ipam/client/clientset/versioned"
	ipamcontext "tkestack.io/galaxy/pkg/ipam/context"
	"tkestack.io/galaxy/pkg/ipam/crd"
	"tkestack.io/galaxy/pkg/ipam/metrics"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin"
	"tkestack.io/galaxy/pkg/ipam/server/options"
	"tkestack.io/galaxy/pkg/utils/httputil"
	pageutil "tkestack.io/galaxy/pkg/utils/page"
)

type JsonConf struct {
	SchedulePluginConf schedulerplugin.Conf `json:"schedule_plugin"`
}

const COMPONENT_NAME = "galaxy-ipam"

type Server struct {
	JsonConf
	*options.ServerRunOptions
	*ipamcontext.IPAMContext
	plugin               *schedulerplugin.FloatingIPPlugin
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
	s.plugin, err = schedulerplugin.NewFloatingIPPlugin(s.SchedulePluginConf, s.IPAMContext)
	if err != nil {
		return err
	}
	s.PodInformer.Informer().AddEventHandler(eventhandler.NewPodEventHandler(s.plugin))
	return nil
}

func (s *Server) Start() error {
	if err := s.init(); err != nil {
		return fmt.Errorf("init server: %v", err)
	}
	s.StartInformers(s.stopChan)
	if s.LeaderElection.LeaderElect && s.leaderElectionConfig != nil {
		leaderelection.RunOrDie(context.Background(), *s.leaderElectionConfig)
		return nil
	}
	return s.Run()
}

func (s *Server) Run() error {
	if err := s.plugin.Init(); err != nil {
		return err
	}
	s.plugin.Run(s.stopChan)
	go s.startAPIServer()
	s.startServer()
	return nil
}

// #lizard forgives
func (s *Server) initk8sClient() {
	cfg, err := clientcmd.BuildConfigFromFlags(s.Master, s.KubeConf)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}
	cfg.QPS = 1000.0
	cfg.Burst = 2000
	glog.Infof("QPS: %e, Burst: %d", cfg.QPS, cfg.Burst)

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %v", err)
	}
	galaxyClient, err := versioned.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building float ip clientset: %v", err)
	}
	extClient, err := extensionClient.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building extension clientset: %v", err)
	}
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building dynamic clientset: %v", err)
	}
	s.IPAMContext = ipamcontext.NewIPAMContext(client, galaxyClient, extClient, dynamicClient)
	glog.Infof("connected to apiserver %v", cfg)
	if err := crd.EnsureCRDCreated(extClient); err != nil {
		glog.Fatalf("Ensure crd created: %v", err)
	}

	// Identity used to distinguish between multiple cloud controller manager instances
	id, err := os.Hostname()
	if err != nil {
		glog.Fatalf("Error getting host name: %v", err)
	}
	// add a uniquifier so that two processes on the same host don't accidentally both become active
	id = id + "_" + string(uuid.NewUUID())

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
			leaderElectionClient.CoordinationV1(),
			resourcelock.ResourceLockConfig{
				Identity:      id,
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
				OnStartedLeading: func(ctx context.Context) {
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
	ws.Route(ws.POST("/filter").To(s.filter).Reads(schedulerapi.ExtenderArgs{}).
		Writes(schedulerapi.ExtenderFilterResult{}))
	ws.Route(ws.POST("/priority").To(s.priority).Reads(schedulerapi.ExtenderArgs{}).
		Writes(schedulerapi.HostPriorityList{}))
	ws.Route(ws.POST("/bind").To(s.bind).Reads(schedulerapi.ExtenderBindingArgs{}).
		Writes(schedulerapi.ExtenderBindingResult{}))
	health := new(restful.WebService)
	health.Route(health.GET("/healthy").To(s.healthy))
	container := restful.NewContainer()
	container.Add(ws)
	container.Add(health)
	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", s.Bind, s.Port), container); err != nil {
		glog.Fatalf("unable to listen: %v.", err)
	}
}

func (s *Server) startAPIServer() {
	ws := new(restful.WebService)
	ws.
		Path("/v1").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)
	c := api.NewController(s.plugin.GetIpam(), s.PodLister)
	ws.Route(ws.GET("/ip").To(c.ListIPs).
		Doc("List ips by keyword or params").
		Param(ws.QueryParameter("keyword", "keyword").DataType("string")).
		Param(ws.QueryParameter("poolName", "pool name").DataType("string")).
		Param(ws.QueryParameter("appName", "app name").DataType("string")).
		Param(ws.QueryParameter("podName", "pod name").DataType("string")).
		Param(ws.QueryParameter("namespace", "namespace").DataType("string")).
		Param(ws.QueryParameter("appType", "app type, deployment, statefulset or tapp, default statefulset").DataType("string")).
		Param(ws.QueryParameter("page", "page number, valid range [0,99999]").DataType("integer")).
		Param(ws.QueryParameter("size", "page size, valid range (0,9999]").DataType("integer").DefaultValue("10")).
		Param(ws.QueryParameter("sort", "sort by which field, supports ip/namespace/podname/policy asc/desc").
			DataType("string").DefaultValue("ip asc")).
		Returns(http.StatusOK, "request succeed", api.ListIPResp{
			Page: pageutil.Page{Last: true, TotalElements: 2, TotalPages: 1, First: true, NumberOfElements: 2,
				Size: 10, Number: 0},
			Content: []api.FloatingIP{
				{IP: "10.0.70.93", PoolName: "sample-pool", Policy: 2, UpdateTime: time.Unix(1555924386, 0),
					Releasable: true, AppType: "deployment"},
				{IP: "10.0.70.118", PoolName: "sample-pool2", Namespace: "default", AppName: "app",
					PodName: "app-xxx-yyy", Policy: 2, UpdateTime: time.Unix(1555924279, 0), Status: "Running",
					AppType: "deployment"},
			}}).
		Writes(api.ListIPResp{}))

	ws.Route(ws.POST("/ip").To(c.ReleaseIPs).
		Doc("Release ips").
		Reads(api.ReleaseIPReq{}).
		Returns(http.StatusBadRequest, "10.0.0 is not a valid ip", nil).
		Returns(http.StatusBadRequest, "10.0.0.2 is not releasable", nil).
		Returns(http.StatusInternalServerError, "internal server error", nil).
		Returns(http.StatusAccepted, "Unreleased ips have been released or allocated to other pods, or are not "+
			"within valid range", api.ReleaseIPResp{Unreleased: []string{"10.0.70.32"}}).
		Returns(http.StatusOK, "request succeed", api.ReleaseIPResp{Resp: httputil.Resp{Code: http.StatusOK}}).
		Writes(api.ReleaseIPResp{Resp: httputil.Resp{Code: http.StatusOK}}))

	poolController := api.PoolController{PoolLister: s.PoolLister, Client: s.GalaxyClient,
		LockPoolFunc: s.plugin.LockDpPool, IPAM: s.plugin.GetIpam()}
	ws.Route(ws.GET("/pool/{name}").To(poolController.Get).
		Doc("Get pool by name").
		Param(ws.PathParameter("name", "pool name").DataType("string").Required(true)).
		Returns(http.StatusNotFound, "pool not found", nil).
		Returns(http.StatusBadRequest, "pool name is empty", nil).
		Returns(http.StatusInternalServerError, "internal server error", nil).
		Returns(http.StatusOK, "request succeed", api.GetPoolResp{Resp: httputil.NewResp(http.StatusOK, ""),
			Pool: api.Pool{Name: "sample-pool", Size: 4}}).
		Writes(api.GetPoolResp{}))

	ws.Route(ws.POST("/pool").To(poolController.CreateOrUpdate).
		Doc("Create or update pool").
		Reads(api.Pool{Name: "sample-pool"}).
		Returns(http.StatusBadRequest, "pool name is empty", nil).
		Returns(http.StatusInternalServerError, "internal server error", nil).
		Returns(http.StatusAccepted, "No enough IPs", api.UpdatePoolResp{
			Resp: httputil.NewResp(http.StatusAccepted, "No enough IPs"), RealPoolSize: 3}).
		Returns(http.StatusOK, "request succeed", api.UpdatePoolResp{
			Resp: httputil.NewResp(http.StatusOK, ""), RealPoolSize: 3}).
		Writes(httputil.Resp{Code: http.StatusOK}))

	ws.Route(ws.DELETE("/pool/{name}").To(poolController.Delete).
		Doc("Delete pool by name").
		Param(ws.PathParameter("name", "pool name").DataType("string").Required(true)).
		Returns(http.StatusNotFound, "pool not found", nil).
		Returns(http.StatusBadRequest, "pool name is empty", nil).
		Returns(http.StatusInternalServerError, "internal server error", nil).
		Returns(http.StatusOK, "request succeed", httputil.Resp{Code: http.StatusOK}).
		Writes(httputil.Resp{Code: http.StatusOK}))

	restful.Add(ws)
	// register prometheus metrics
	prometheus.MustRegister(s.plugin.GetIpam())
	metrics.MustRegister()
	restful.DefaultContainer.Handle("/metrics", promhttp.Handler())
	addSwaggerUISupport(restful.DefaultContainer)
	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", s.Bind, s.APIPort), nil); err != nil {
		glog.Fatalf("unable to listen: %v.", err)
	}
}

func addSwaggerUISupport(container *restful.Container) {
	config := swagger.Config{
		WebServices:     restful.RegisteredWebServices(),
		ApiPath:         "/apidocs.json",
		SwaggerPath:     "/apidocs/",
		SwaggerFilePath: "/etc/swagger-ui/dist",
	}

	swagger.RegisterSwaggerService(config, container)
}

func (s *Server) filter(request *restful.Request, response *restful.Response) {
	args := new(schedulerapi.ExtenderArgs)
	if err := request.ReadEntity(&args); err != nil {
		glog.Error(err)
		_ = response.WriteError(http.StatusInternalServerError, err)
		return
	}
	glog.V(5).Infof("POST filter %v", *args)
	start := time.Now()
	glog.V(3).Infof("filtering %s_%s, start at %d+", args.Pod.Name, args.Pod.Namespace, start.UnixNano())
	filteredNodes, failedNodesMap, err := s.plugin.Filter(&args.Pod, args.Nodes.Items)
	glog.V(3).Infof("filtering %s_%s, start at %d-", args.Pod.Name, args.Pod.Namespace, start.UnixNano())
	args.Nodes.Items = filteredNodes
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	_ = response.WriteEntity(schedulerapi.ExtenderFilterResult{
		Nodes:       args.Nodes,
		FailedNodes: failedNodesMap,
		Error:       errStr,
	})
}

func (s *Server) priority(request *restful.Request, response *restful.Response) {
	args := new(schedulerapi.ExtenderArgs)
	if err := request.ReadEntity(&args); err != nil {
		glog.Error(err)
		_ = response.WriteError(http.StatusInternalServerError, err)
		return
	}
	glog.V(5).Infof("POST priority %v", *args)
	hostPriorityList, err := s.plugin.Prioritize(&args.Pod, args.Nodes.Items)
	if err != nil {
		glog.Warningf("prioritize err: %v", err)
	}
	_ = response.WriteEntity(*hostPriorityList)
}

func (s *Server) bind(request *restful.Request, response *restful.Response) {
	args := new(schedulerapi.ExtenderBindingArgs)
	if err := request.ReadEntity(&args); err != nil {
		glog.Error(err)
		_ = response.WriteError(http.StatusInternalServerError, err)
		return
	}
	glog.V(5).Infof("POST bind %v", *args)
	start := time.Now()
	glog.V(3).Infof("binding %s_%s to %s, start at %d+", args.PodName, args.PodNamespace, args.Node, start.UnixNano())
	err := s.plugin.Bind(args)
	glog.V(3).Infof("binding %s_%s to %s, start at %d-", args.PodName, args.PodNamespace, args.Node, start.UnixNano())
	var result schedulerapi.ExtenderBindingResult
	if err != nil {
		glog.Warningf("bind err: %v", err)
		result.Error = err.Error()
	}
	_ = response.WriteEntity(result)
}

func (s *Server) healthy(request *restful.Request, response *restful.Response) {
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write([]byte("ok"))
}
