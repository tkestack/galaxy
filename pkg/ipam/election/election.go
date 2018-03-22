package election

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	JitterFactor = 1.2

	LeaderElectionRecordAnnotationKey = "gaiastack.tencent.com/leader"
)

type Config struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	TTL       int64  `json:"ttl"`
	Bind      string `json:"-"`
	Port      int32  `json:"-"`
}

type LeaderElectionConfig struct {
	// EndpointsMeta should contain a Name and a Namespace of an
	// Endpoints object that the LeaderElector will attempt to lead.
	EndpointsMeta metav1.ObjectMeta
	// Identity is a unique identifier of the leader elector.
	Identity string

	Client *kubernetes.Clientset
	// LeaseDuration is the duration that non-leader candidates will
	// wait to force acquire leadership. This is measured against time of
	// last observed ack.
	LeaseDuration time.Duration
	// RenewDeadline is the duration that the acting master will retry
	// refreshing leadership before giving up.
	RenewDeadline time.Duration
	// RetryPeriod is the duration the LeaderElector clients should wait
	// between tries of actions.
	RetryPeriod time.Duration

	// Callbacks are callbacks that are triggered during certain lifecycle
	// events of the LeaderElector
	Callbacks leaderCallbacks

	Bind string
	Port int32
}

type leaderCallbacks struct {
	// OnStartedLeading is called when a LeaderElector client starts leading
	OnStartedLeading func(stop <-chan struct{})
	// OnStoppedLeading is called when a LeaderElector client stops leading
	OnStoppedLeading func()
	// OnNewLeader is called when the client observes a leader that is
	// not the previously observed leader. This includes the first observed
	// leader when the client starts.
	OnNewLeader func(identity string)
}

func NewConfig(config Config, c *kubernetes.Clientset) (*LeaderElectionConfig, error) {
	if config.Name == "" || config.Namespace == "" {
		return nil, fmt.Errorf("name and namespace must not be empty")
	}
	lease := time.Second * time.Duration(config.TTL+8)
	renew := time.Second * time.Duration(config.TTL+2)
	retry := time.Second

	return &LeaderElectionConfig{
		EndpointsMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
		},
		Client:        c,
		Identity:      fmt.Sprintf("%s:%d", config.Bind, config.Port),
		Bind:          config.Bind,
		Port:          config.Port,
		LeaseDuration: lease,
		RenewDeadline: renew,
		RetryPeriod:   retry,
		Callbacks: leaderCallbacks{
			OnStoppedLeading: func() {
				glog.Fatalf("leader election lost")
			},
		},
	}, nil
}

func RunOrDie(lec *LeaderElectionConfig, run func(<-chan struct{})) {
	lec.Callbacks.OnStartedLeading = run
	le, err := NewLeaderElector(*lec)
	if err != nil {
		panic(err)
	}
	le.Run()
}

type LeaderElector struct {
	config LeaderElectionConfig
	// internal bookkeeping
	observedRecord LeaderElectionRecord
	observedTime   time.Time
	// used to implement OnNewLeader(), may lag slightly from the
	// value observedRecord.HolderIdentity if the transition has
	// not yet been reported.
	reportedLeader string
}

type LeaderElectionRecord struct {
	HolderIdentity       string      `json:"holderIdentity"`
	LeaseDurationSeconds int         `json:"leaseDurationSeconds"`
	AcquireTime          metav1.Time `json:"acquireTime"`
	RenewTime            metav1.Time `json:"renewTime"`
	LeaderTransitions    int         `json:"leaderTransitions"`
}

func NewLeaderElector(lec LeaderElectionConfig) (*LeaderElector, error) {
	if lec.LeaseDuration <= lec.RenewDeadline {
		return nil, fmt.Errorf("leaseDuration must be greater than renewDeadline")
	}
	if lec.RenewDeadline <= time.Duration(JitterFactor*float64(lec.RetryPeriod)) {
		return nil, fmt.Errorf("renewDeadline must be greater than retryPeriod*JitterFactor")
	}
	if lec.Client == nil {
		return nil, fmt.Errorf("client must not be nil.")
	}
	return &LeaderElector{
		config: lec,
	}, nil
}

func (le *LeaderElector) Run() {
	defer func() {
		le.config.Callbacks.OnStoppedLeading()
	}()
	le.acquire()
	stop := make(chan struct{})
	go le.config.Callbacks.OnStartedLeading(stop)
	le.renew()
	close(stop)
}

func (le *LeaderElector) renew() {
	stop := make(chan struct{})
	wait.Until(func() {
		err := wait.Poll(le.config.RetryPeriod, le.config.RenewDeadline, func() (bool, error) {
			return le.tryAcquireOrRenew(), nil
		})
		le.maybeReportTransition()
		if err == nil {
			glog.V(4).Infof("succesfully renewed lease %v/%v", le.config.EndpointsMeta.Namespace, le.config.EndpointsMeta.Name)
			return
		}
		glog.Infof("%v stopped leading", le.config.Identity)
		glog.Infof("failed to renew lease %v/%v", le.config.EndpointsMeta.Namespace, le.config.EndpointsMeta.Name)
		close(stop)
	}, 0, stop)
}

func (le *LeaderElector) acquire() {
	stop := make(chan struct{})
	wait.JitterUntil(func() {
		succeeded := le.tryAcquireOrRenew()
		le.maybeReportTransition()
		if !succeeded {
			glog.V(4).Infof("failed to renew lease %v/%v", le.config.EndpointsMeta.Namespace, le.config.EndpointsMeta.Name)
			return
		}
		glog.Infof("%v became leader", le.config.Identity)
		glog.Infof("successfully acquired lease %v/%v", le.config.EndpointsMeta.Namespace, le.config.EndpointsMeta.Name)
		close(stop)
	}, le.config.RetryPeriod, JitterFactor, true, stop)
}

func (le *LeaderElector) tryAcquireOrRenew() bool {
	now := metav1.Now()
	leaderElectionRecord := LeaderElectionRecord{
		HolderIdentity:       le.config.Identity,
		LeaseDurationSeconds: int(le.config.LeaseDuration / time.Second),
		RenewTime:            now,
		AcquireTime:          now,
	}

	e, err := le.config.Client.CoreV1().Endpoints(le.config.EndpointsMeta.Namespace).Get(le.config.EndpointsMeta.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			glog.Errorf("error retrieving endpoint: %v", err)
			return false
		}

		leaderElectionRecordBytes, err := json.Marshal(leaderElectionRecord)
		if err != nil {
			return false
		}
		endpoint := &v1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      le.config.EndpointsMeta.Name,
				Namespace: le.config.EndpointsMeta.Namespace,
				Annotations: map[string]string{
					LeaderElectionRecordAnnotationKey: string(leaderElectionRecordBytes),
				},
			},
		}
		le.populateSubnets(endpoint)
		_, err = le.config.Client.CoreV1().Endpoints(le.config.EndpointsMeta.Namespace).Create(endpoint)
		if err != nil {
			glog.Errorf("error initially creating endpoints: %v", err)
			return false
		}
		le.observedRecord = leaderElectionRecord
		le.observedTime = time.Now()
		return true
	}

	if e.Annotations == nil {
		e.Annotations = make(map[string]string)
	}

	var oldLeaderElectionRecord LeaderElectionRecord

	if oldLeaderElectionRecordBytes, found := e.Annotations[LeaderElectionRecordAnnotationKey]; found {
		if err := json.Unmarshal([]byte(oldLeaderElectionRecordBytes), &oldLeaderElectionRecord); err != nil {
			glog.Errorf("error unmarshaling leader election record: %v", err)
			return false
		}
		if !reflect.DeepEqual(le.observedRecord, oldLeaderElectionRecord) {
			le.observedRecord = oldLeaderElectionRecord
			le.observedTime = time.Now()
		}
		if le.observedTime.Add(le.config.LeaseDuration).After(now.Time) &&
			oldLeaderElectionRecord.HolderIdentity != le.config.Identity {
			glog.Infof("lock is held by %v and has not yet expired", oldLeaderElectionRecord.HolderIdentity)
			return false
		}
	}

	// We're going to try to update. The LeaderElectionRecord is set to it's default
	// here. Let's correct it before updating.
	if oldLeaderElectionRecord.HolderIdentity == le.config.Identity {
		leaderElectionRecord.AcquireTime = oldLeaderElectionRecord.AcquireTime
	} else {
		leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions + 1
	}

	leaderElectionRecordBytes, err := json.Marshal(leaderElectionRecord)
	if err != nil {
		glog.Errorf("err marshaling leader election record: %v", err)
		return false
	}
	e.Annotations[LeaderElectionRecordAnnotationKey] = string(leaderElectionRecordBytes)
	le.populateSubnets(e)

	_, err = le.config.Client.CoreV1().Endpoints(le.config.EndpointsMeta.Namespace).Update(e)
	if err != nil {
		glog.Errorf("err: %v", err)
		return false
	}
	le.observedRecord = leaderElectionRecord
	le.observedTime = time.Now()
	return true
}

func (l *LeaderElector) maybeReportTransition() {
	if l.observedRecord.HolderIdentity == l.reportedLeader {
		return
	}
	l.reportedLeader = l.observedRecord.HolderIdentity
	if l.config.Callbacks.OnNewLeader != nil {
		go l.config.Callbacks.OnNewLeader(l.reportedLeader)
	}
}

func (le *LeaderElector) populateSubnets(endpoint *v1.Endpoints) {
	endpoint.Subsets = nil
	// 0.0.0.0 is an invalid value, kubernetes will complain:
	// Endpoints "galaxy-ipam" is invalid: subsets[0].addresses[0].ip: Invalid value: "0.0.0.0": may not be unspecified (0.0.0.0)
	if le.config.Bind != "" && le.config.Bind != "0.0.0.0" {
		endpoint.Subsets = []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{{IP: le.config.Bind}},
				Ports:     []v1.EndpointPort{{Port: le.config.Port}},
			},
		}
	}
}
