package cache

import (
	"fmt"

	"github.com/golang/glog"
	"k8s.io/client-go/1.4/pkg/api"
	"k8s.io/client-go/1.4/pkg/api/errors"
	"k8s.io/client-go/1.4/pkg/api/unversioned"
	"k8s.io/client-go/1.4/pkg/api/v1"
	extensionsv1 "k8s.io/client-go/1.4/pkg/apis/extensions/v1beta1"
	gaiav1 "k8s.io/client-go/1.4/pkg/apis/gaia/v1alpha1"
	"k8s.io/client-go/1.4/pkg/apis/policy"
	"k8s.io/client-go/1.4/pkg/labels"
	"k8s.io/client-go/1.4/tools/cache"
)

// copied from k8s.io/client-go/1.4/tools/cache/listers.go change api version to v1

//  TODO: generate these classes and methods for all resources of interest using
// a script.  Can use "go generate" once 1.4 is supported by all users.

// StoreToPodLister makes a Store have the List method of the client.PodInterface
// The Store must contain (only) Pods.
//
// Example:
// s := cache.NewStore()
// lw := cache.ListWatch{Client: c, FieldSelector: sel, Resource: "pods"}
// r := cache.NewReflector(lw, &v1.Pod{}, s).Run()
// l := StoreToPodLister{s}
// l.List()
type StoreToPodLister struct {
	cache.Indexer
}

// Please note that selector is filtering among the pods that have gotten into
// the store; there may have been some filtering that already happened before
// that.
// We explicitly don't return v1.PodList, to avoid expensive allocations, which
// in most cases are unnecessary.
func (s *StoreToPodLister) List(selector labels.Selector) (pods []*v1.Pod, err error) {
	for _, m := range s.Indexer.List() {
		pod := m.(*v1.Pod)
		if selector.Matches(labels.Set(pod.Labels)) {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

// Pods is taking baby steps to be more like the api in pkg/client
func (s *StoreToPodLister) Pods(namespace string) storePodsNamespacer {
	return storePodsNamespacer{s.Indexer, namespace}
}

type storePodsNamespacer struct {
	indexer   cache.Indexer
	namespace string
}

// Please note that selector is filtering among the pods that have gotten into
// the store; there may have been some filtering that already happened before
// that.
// We explicitly don't return v1.PodList, to avoid expensive allocations, which
// in most cases are unnecessary.
func (s storePodsNamespacer) List(selector labels.Selector) (pods []*v1.Pod, err error) {
	if s.namespace == v1.NamespaceAll {
		for _, m := range s.indexer.List() {
			pod := m.(*v1.Pod)
			if selector.Matches(labels.Set(pod.Labels)) {
				pods = append(pods, pod)
			}
		}
		return pods, nil
	}

	key := &v1.Pod{ObjectMeta: v1.ObjectMeta{Namespace: s.namespace}}
	items, err := s.indexer.Index(cache.NamespaceIndex, key)
	if err != nil {
		// Ignore error; do slow search without index.
		glog.Warningf("can not retrieve list of objects using index : %v", err)
		for _, m := range s.indexer.List() {
			pod := m.(*v1.Pod)
			if s.namespace == pod.Namespace && selector.Matches(labels.Set(pod.Labels)) {
				pods = append(pods, pod)
			}
		}
		return pods, nil
	}
	for _, m := range items {
		pod := m.(*v1.Pod)
		if selector.Matches(labels.Set(pod.Labels)) {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

func (s storePodsNamespacer) Get(name string) (*v1.Pod, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(api.Resource("pod"), name)
	}
	return obj.(*v1.Pod), nil
}

// Exists returns true if a pod matching the namespace/name of the given pod exists in the store.
func (s *StoreToPodLister) Exists(pod *v1.Pod) (bool, error) {
	_, exists, err := s.Indexer.Get(pod)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// NodeConditionPredicate is a function that indicates whether the given node's conditions meet
// some set of criteria defined by the function.
type NodeConditionPredicate func(node *v1.Node) bool

// StoreToNodeLister makes a Store have the List method of the client.NodeInterface
// The Store must contain (only) Nodes.
type StoreToNodeLister struct {
	cache.Store
}

func (s *StoreToNodeLister) List() (machines v1.NodeList, err error) {
	for _, m := range s.Store.List() {
		machines.Items = append(machines.Items, *(m.(*v1.Node)))
	}
	return machines, nil
}

// NodeCondition returns a storeToNodeConditionLister
func (s *StoreToNodeLister) NodeCondition(predicate NodeConditionPredicate) storeToNodeConditionLister {
	// TODO: Move this filtering server side. Currently our selectors don't facilitate searching through a list so we
	// have the reflector filter out the Unschedulable field and sift through node conditions in the lister.
	return storeToNodeConditionLister{s.Store, predicate}
}

// storeToNodeConditionLister filters and returns nodes matching the given type and status from the store.
type storeToNodeConditionLister struct {
	store     cache.Store
	predicate NodeConditionPredicate
}

// List returns a list of nodes that match the conditions defined by the predicate functions in the storeToNodeConditionLister.
func (s storeToNodeConditionLister) List() (nodes []*v1.Node, err error) {
	for _, m := range s.store.List() {
		node := m.(*v1.Node)
		if s.predicate(node) {
			nodes = append(nodes, node)
		} else {
			glog.V(5).Infof("Node %s matches none of the conditions", node.Name)
		}
	}
	return
}

// StoreToReplicationControllerLister gives a store List and Exists methods. The store must contain only ReplicationControllers.
type StoreToReplicationControllerLister struct {
	cache.Indexer
}

// Exists checks if the given rc exists in the store.
func (s *StoreToReplicationControllerLister) Exists(controller *v1.ReplicationController) (bool, error) {
	_, exists, err := s.Indexer.Get(controller)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// StoreToReplicationControllerLister lists all controllers in the store.
// TODO: converge on the interface in pkg/client
func (s *StoreToReplicationControllerLister) List() (controllers []v1.ReplicationController, err error) {
	for _, c := range s.Indexer.List() {
		controllers = append(controllers, *(c.(*v1.ReplicationController)))
	}
	return controllers, nil
}

func (s *StoreToReplicationControllerLister) ReplicationControllers(namespace string) storeReplicationControllersNamespacer {
	return storeReplicationControllersNamespacer{s.Indexer, namespace}
}

type storeReplicationControllersNamespacer struct {
	indexer   cache.Indexer
	namespace string
}

func (s storeReplicationControllersNamespacer) List(selector labels.Selector) ([]v1.ReplicationController, error) {
	controllers := []v1.ReplicationController{}

	if s.namespace == v1.NamespaceAll {
		for _, m := range s.indexer.List() {
			rc := *(m.(*v1.ReplicationController))
			if selector.Matches(labels.Set(rc.Labels)) {
				controllers = append(controllers, rc)
			}
		}
		return controllers, nil
	}

	key := &v1.ReplicationController{ObjectMeta: v1.ObjectMeta{Namespace: s.namespace}}
	items, err := s.indexer.Index(cache.NamespaceIndex, key)
	if err != nil {
		// Ignore error; do slow search without index.
		glog.Warningf("can not retrieve list of objects using index : %v", err)
		for _, m := range s.indexer.List() {
			rc := *(m.(*v1.ReplicationController))
			if s.namespace == rc.Namespace && selector.Matches(labels.Set(rc.Labels)) {
				controllers = append(controllers, rc)
			}
		}
		return controllers, nil
	}
	for _, m := range items {
		rc := *(m.(*v1.ReplicationController))
		if selector.Matches(labels.Set(rc.Labels)) {
			controllers = append(controllers, rc)
		}
	}
	return controllers, nil
}

func (s storeReplicationControllersNamespacer) Get(name string) (*v1.ReplicationController, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(api.Resource("replicationcontroller"), name)
	}
	return obj.(*v1.ReplicationController), nil
}

// GetPodControllers returns a list of replication controllers managing a pod. Returns an error only if no matching controllers are found.
func (s *StoreToReplicationControllerLister) GetPodControllers(pod *v1.Pod) (controllers []v1.ReplicationController, err error) {
	var selector labels.Selector
	var rc v1.ReplicationController

	if len(pod.Labels) == 0 {
		err = fmt.Errorf("no controllers found for pod %v because it has no labels", pod.Name)
		return
	}

	key := &v1.ReplicationController{ObjectMeta: v1.ObjectMeta{Namespace: pod.Namespace}}
	items, err := s.Indexer.Index(cache.NamespaceIndex, key)
	if err != nil {
		return
	}

	for _, m := range items {
		rc = *m.(*v1.ReplicationController)
		selector = labels.Set(rc.Spec.Selector).AsSelectorPreValidated()

		// If an rc with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		controllers = append(controllers, rc)
	}
	if len(controllers) == 0 {
		err = fmt.Errorf("could not find controller for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}
	return
}

// StoreToDeploymentLister gives a store List and Exists methods. The store must contain only Deployments.
type StoreToDeploymentLister struct {
	cache.Indexer
}

// Exists checks if the given deployment exists in the store.
func (s *StoreToDeploymentLister) Exists(deployment *extensionsv1.Deployment) (bool, error) {
	_, exists, err := s.Indexer.Get(deployment)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// StoreToDeploymentLister lists all deployments in the store.
// TODO: converge on the interface in pkg/client
func (s *StoreToDeploymentLister) List() (deployments []extensionsv1.Deployment, err error) {
	for _, c := range s.Indexer.List() {
		deployments = append(deployments, *(c.(*extensionsv1.Deployment)))
	}
	return deployments, nil
}

// GetDeploymentsForReplicaSet returns a list of deployments managing a replica set. Returns an error only if no matching deployments are found.
func (s *StoreToDeploymentLister) GetDeploymentsForReplicaSet(rs *extensionsv1.ReplicaSet) (deployments []extensionsv1.Deployment, err error) {
	if len(rs.Labels) == 0 {
		err = fmt.Errorf("no deployments found for ReplicaSet %v because it has no labels", rs.Name)
		return
	}

	// TODO: MODIFY THIS METHOD so that it checks for the podTemplateSpecHash label
	dList, err := s.Deployments(rs.Namespace).List(labels.Everything())
	if err != nil {
		return
	}
	for _, d := range dList {
		labelSelector, err1 := unversioned.ParseToLabelSelector(d.Spec.Selector.String())
		if err1 != nil {
			err = fmt.Errorf("invalid selector: %v", err1)
			return
		}
		selector, err1 := unversioned.LabelSelectorAsSelector(labelSelector)
		if err1 != nil {
			err = fmt.Errorf("invalid selector: %v", err1)
			return
		}
		// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(rs.Labels)) {
			continue
		}
		deployments = append(deployments, d)
	}
	if len(deployments) == 0 {
		err = fmt.Errorf("could not find deployments set for ReplicaSet %s in namespace %s with labels: %v", rs.Name, rs.Namespace, rs.Labels)
	}
	return
}

type storeToDeploymentNamespacer struct {
	indexer   cache.Indexer
	namespace string
}

// storeToDeploymentNamespacer lists deployments under its namespace in the store.
func (s storeToDeploymentNamespacer) List(selector labels.Selector) (deployments []extensionsv1.Deployment, err error) {
	if s.namespace == v1.NamespaceAll {
		for _, m := range s.indexer.List() {
			d := *(m.(*extensionsv1.Deployment))
			if selector.Matches(labels.Set(d.Labels)) {
				deployments = append(deployments, d)
			}
		}
		return
	}

	key := &extensionsv1.Deployment{ObjectMeta: v1.ObjectMeta{Namespace: s.namespace}}
	items, err := s.indexer.Index(cache.NamespaceIndex, key)
	if err != nil {
		// Ignore error; do slow search without index.
		glog.Warningf("can not retrieve list of objects using index : %v", err)
		for _, m := range s.indexer.List() {
			d := *(m.(*extensionsv1.Deployment))
			if s.namespace == d.Namespace && selector.Matches(labels.Set(d.Labels)) {
				deployments = append(deployments, d)
			}
		}
		return deployments, nil
	}
	for _, m := range items {
		d := *(m.(*extensionsv1.Deployment))
		if selector.Matches(labels.Set(d.Labels)) {
			deployments = append(deployments, d)
		}
	}
	return
}

func (s *StoreToDeploymentLister) Deployments(namespace string) storeToDeploymentNamespacer {
	return storeToDeploymentNamespacer{s.Indexer, namespace}
}

// GetDeploymentsForPods returns a list of deployments managing a pod. Returns an error only if no matching deployments are found.
func (s *StoreToDeploymentLister) GetDeploymentsForPod(pod *v1.Pod) (deployments []extensionsv1.Deployment, err error) {
	if len(pod.Labels) == 0 {
		err = fmt.Errorf("no deployments found for Pod %v because it has no labels", pod.Name)
		return
	}

	if len(pod.Labels[extensionsv1.DefaultDeploymentUniqueLabelKey]) == 0 {
		return
	}

	dList, err := s.Deployments(pod.Namespace).List(labels.Everything())
	if err != nil {
		return
	}
	for _, d := range dList {
		labelSelector, err1 := unversioned.ParseToLabelSelector(d.Spec.Selector.String())
		if err1 != nil {
			err = fmt.Errorf("invalid selector: %v", err1)
			return
		}
		selector, err1 := unversioned.LabelSelectorAsSelector(labelSelector)
		if err1 != nil {
			err = fmt.Errorf("invalid selector: %v", err1)
			return
		}
		// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		deployments = append(deployments, d)
	}
	if len(deployments) == 0 {
		err = fmt.Errorf("could not find deployments set for Pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}
	return
}

// StoreToReplicaSetLister gives a store List and Exists methods. The store must contain only ReplicaSets.
type StoreToReplicaSetLister struct {
	cache.Store
}

// Exists checks if the given ReplicaSet exists in the store.
func (s *StoreToReplicaSetLister) Exists(rs *extensionsv1.ReplicaSet) (bool, error) {
	_, exists, err := s.Store.Get(rs)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// List lists all ReplicaSets in the store.
// TODO: converge on the interface in pkg/client
func (s *StoreToReplicaSetLister) List() (rss []extensionsv1.ReplicaSet, err error) {
	for _, rs := range s.Store.List() {
		rss = append(rss, *(rs.(*extensionsv1.ReplicaSet)))
	}
	return rss, nil
}

type storeReplicaSetsNamespacer struct {
	store     cache.Store
	namespace string
}

func (s storeReplicaSetsNamespacer) List(selector labels.Selector) (rss []extensionsv1.ReplicaSet, err error) {
	for _, c := range s.store.List() {
		rs := *(c.(*extensionsv1.ReplicaSet))
		if s.namespace == v1.NamespaceAll || s.namespace == rs.Namespace {
			if selector.Matches(labels.Set(rs.Labels)) {
				rss = append(rss, rs)
			}
		}
	}
	return
}

func (s storeReplicaSetsNamespacer) Get(name string) (*extensionsv1.ReplicaSet, error) {
	obj, exists, err := s.store.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(api.Resource("replicaset"), name)
	}
	return obj.(*extensionsv1.ReplicaSet), nil
}

func (s *StoreToReplicaSetLister) ReplicaSets(namespace string) storeReplicaSetsNamespacer {
	return storeReplicaSetsNamespacer{s.Store, namespace}
}

// GetPodReplicaSets returns a list of ReplicaSets managing a pod. Returns an error only if no matching ReplicaSets are found.
func (s *StoreToReplicaSetLister) GetPodReplicaSets(pod *v1.Pod) (rss []extensionsv1.ReplicaSet, err error) {
	var selector labels.Selector
	var rs extensionsv1.ReplicaSet

	if len(pod.Labels) == 0 {
		err = fmt.Errorf("no ReplicaSets found for pod %v because it has no labels", pod.Name)
		return
	}

	for _, m := range s.Store.List() {
		rs = *m.(*extensionsv1.ReplicaSet)
		if rs.Namespace != pod.Namespace {
			continue
		}
		labelSelector, err1 := unversioned.ParseToLabelSelector(rs.Spec.Selector.String())
		if err1 != nil {
			err = fmt.Errorf("invalid selector: %v", err1)
			return
		}
		selector, err1 = unversioned.LabelSelectorAsSelector(labelSelector)
		if err1 != nil {
			err = fmt.Errorf("invalid selector: %v", err1)
			return
		}

		// If a ReplicaSet with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		rss = append(rss, rs)
	}
	if len(rss) == 0 {
		err = fmt.Errorf("could not find ReplicaSet for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}
	return
}

// StoreToServiceLister makes a Store that has the List method of the client.ServiceInterface
// The Store must contain (only) Services.
type StoreToServiceLister struct {
	cache.Store
}

func (s *StoreToServiceLister) List() (services v1.ServiceList, err error) {
	for _, m := range s.Store.List() {
		services.Items = append(services.Items, *(m.(*v1.Service)))
	}
	return services, nil
}

// TODO: Move this back to scheduler as a helper function that takes a Store,
// rather than a method of StoreToServiceLister.
func (s *StoreToServiceLister) GetPodServices(pod *v1.Pod) (services []v1.Service, err error) {
	var selector labels.Selector
	var service v1.Service

	for _, m := range s.Store.List() {
		service = *m.(*v1.Service)
		// consider only services that are in the same namespace as the pod
		if service.Namespace != pod.Namespace {
			continue
		}
		if service.Spec.Selector == nil {
			// services with nil selectors match nothing, not everything.
			continue
		}
		selector = labels.Set(service.Spec.Selector).AsSelectorPreValidated()
		if selector.Matches(labels.Set(pod.Labels)) {
			services = append(services, service)
		}
	}
	if len(services) == 0 {
		err = fmt.Errorf("could not find service for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}

	return
}

// StoreToEndpointsLister makes a Store that lists endpoints.
type StoreToEndpointsLister struct {
	cache.Store
}

// List lists all endpoints in the store.
func (s *StoreToEndpointsLister) List() (services v1.EndpointsList, err error) {
	for _, m := range s.Store.List() {
		services.Items = append(services.Items, *(m.(*v1.Endpoints)))
	}
	return services, nil
}

// GetServiceEndpoints returns the endpoints of a service, matched on service name.
func (s *StoreToEndpointsLister) GetServiceEndpoints(svc *v1.Service) (ep v1.Endpoints, err error) {
	for _, m := range s.Store.List() {
		ep = *m.(*v1.Endpoints)
		if svc.Name == ep.Name && svc.Namespace == ep.Namespace {
			return ep, nil
		}
	}
	err = fmt.Errorf("could not find endpoints for service: %v", svc.Name)
	return
}

// Typed wrapper around a store of PersistentVolumes
type StoreToPVFetcher struct {
	cache.Store
}

// GetPersistentVolumeInfo returns cached data for the PersistentVolume 'id'.
func (s *StoreToPVFetcher) GetPersistentVolumeInfo(id string) (*v1.PersistentVolume, error) {
	o, exists, err := s.Get(&v1.PersistentVolume{ObjectMeta: v1.ObjectMeta{Name: id}})

	if err != nil {
		return nil, fmt.Errorf("error retrieving PersistentVolume '%v' from cache: %v", id, err)
	}

	if !exists {
		return nil, fmt.Errorf("PersistentVolume '%v' not found", id)
	}

	return o.(*v1.PersistentVolume), nil
}

// Typed wrapper around a store of PersistentVolumeClaims
type StoreToPVCFetcher struct {
	cache.Store
}

// GetPersistentVolumeClaimInfo returns cached data for the PersistentVolumeClaim 'id'.
func (s *StoreToPVCFetcher) GetPersistentVolumeClaimInfo(namespace string, id string) (*v1.PersistentVolumeClaim, error) {
	o, exists, err := s.Get(&v1.PersistentVolumeClaim{ObjectMeta: v1.ObjectMeta{Namespace: namespace, Name: id}})
	if err != nil {
		return nil, fmt.Errorf("error retrieving PersistentVolumeClaim '%s/%s' from cache: %v", namespace, id, err)
	}

	if !exists {
		return nil, fmt.Errorf("PersistentVolumeClaim '%s/%s' not found", namespace, id)
	}

	return o.(*v1.PersistentVolumeClaim), nil
}

// StoreToTApp Lister gives a store List and Exists methods. The store must contain only TApp.
type StoreToTAppLister struct {
	cache.Store
}

// Exists checks if the given PetSet exists in the store.
func (s *StoreToTAppLister) Exists(ps *gaiav1.TApp) (bool, error) {
	_, exists, err := s.Store.Get(ps)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// List lists all TApps in the store.
func (s *StoreToTAppLister) List() (tappList []gaiav1.TApp, err error) {
	for _, tapp := range s.Store.List() {
		tappList = append(tappList, *(tapp.(*gaiav1.TApp)))
	}
	return tappList, nil
}

type storeTAppsNamespacer struct {
	store     cache.Store
	namespace string
}

func (s *StoreToTAppLister) TApps(namespace string) storeTAppsNamespacer {
	return storeTAppsNamespacer{s.Store, namespace}
}

// GetPodTApps returns a list of TApp managing a pod. Returns an error only if no matching TApp are found.
func (s *StoreToTAppLister) GetPodTApps(pod *v1.Pod) (tappList []gaiav1.TApp, err error) {
	var selector labels.Selector
	var tapp gaiav1.TApp

	if len(pod.Labels) == 0 {
		err = fmt.Errorf("no TApps found for pod %v because it has no labels", pod.Name)
		return
	}

	for _, m := range s.Store.List() {
		tapp = *m.(*gaiav1.TApp)
		if tapp.Namespace != pod.Namespace {
			continue
		}
		selector, err = unversioned.LabelSelectorAsSelector(tapp.Spec.Selector)
		if err != nil {
			err = fmt.Errorf("invalid selector: %v", err)
			return
		}

		// If a TApp with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		tappList = append(tappList, tapp)
	}
	if len(tappList) == 0 {
		err = fmt.Errorf("could not find TApp for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}
	return
}

// IndexerToNamespaceLister gives an Indexer List method
type IndexerToNamespaceLister struct {
	cache.Indexer
}

// List returns a list of namespaces
func (i *IndexerToNamespaceLister) List(selector labels.Selector) (namespaces []*v1.Namespace, err error) {
	for _, m := range i.Indexer.List() {
		namespace := m.(*v1.Namespace)
		if selector.Matches(labels.Set(namespace.Labels)) {
			namespaces = append(namespaces, namespace)
		}
	}

	return namespaces, nil
}

type StoreToPodDisruptionBudgetLister struct {
	cache.Store
}

// GetPodPodDisruptionBudgets returns a list of PodDisruptionBudgets matching a pod.  Returns an error only if no matching PodDisruptionBudgets are found.
func (s *StoreToPodDisruptionBudgetLister) GetPodPodDisruptionBudgets(pod *v1.Pod) (pdbList []policy.PodDisruptionBudget, err error) {
	var selector labels.Selector

	if len(pod.Labels) == 0 {
		err = fmt.Errorf("no PodDisruptionBudgets found for pod %v because it has no labels", pod.Name)
		return
	}

	for _, m := range s.Store.List() {
		pdb, ok := m.(*policy.PodDisruptionBudget)
		if !ok {
			glog.Errorf("Unexpected: %v is not a PodDisruptionBudget", m)
			continue
		}
		if pdb.Namespace != pod.Namespace {
			continue
		}
		selector, err = unversioned.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			glog.Warningf("invalid selector: %v", err)
			// TODO(mml): add an event to the PDB
			continue
		}

		// If a PDB with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		pdbList = append(pdbList, *pdb)
	}
	if len(pdbList) == 0 {
		err = fmt.Errorf("could not find PodDisruptionBudget for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}
	return
}
