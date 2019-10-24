package schedulerplugin

import (
	"fmt"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/schedulerplugin/util"
)

func (p *FloatingIPPlugin) unbindStsOrTappPod(pod *corev1.Pod, keyObj *util.KeyObj,
	policy constant.ReleasePolicy) error {
	key := keyObj.KeyInDB
	if policy == constant.ReleasePolicyPodDelete {
		return p.releaseIP(key, deletedAndIPMutablePod, pod)
	} else if policy == constant.ReleasePolicyNever {
		return p.reserveIP(key, key, "never policy", p.enabledSecondIP(pod))
	} else if policy == constant.ReleasePolicyImmutable {
		appExist, replicas, err := p.checkAppAndReplicas(pod, keyObj)
		if err != nil {
			return err
		}
		shouldReserve, reason, err := p.shouldReserve(pod, keyObj, appExist, replicas)
		if err != nil {
			return err
		}
		if shouldReserve {
			return p.reserveIP(key, key, "immutable policy", p.enabledSecondIP(pod))
		} else {
			return p.releaseIP(key, reason, pod)
		}
	}
	return nil
}

func (p *FloatingIPPlugin) checkAppAndReplicas(pod *corev1.Pod,
	keyObj *util.KeyObj) (appExist bool, replicas int32, retErr error) {
	if keyObj.StatefulSet() {
		return p.getStsReplicas(pod, keyObj)
	} else if keyObj.TApp() {
		return p.getTAppReplicas(pod, keyObj)
	} else {
		retErr = fmt.Errorf("Unknown app")
		return
	}
	return
}

func (p *FloatingIPPlugin) getStsReplicas(pod *corev1.Pod,
	keyObj *util.KeyObj) (appExist bool, replicas int32, retErr error) {
	statefulSet, err := p.StatefulSetLister.GetPodStatefulSets(pod)
	if err != nil {
		if !metaErrs.IsNotFound(err) {
			retErr = err
			return
		}
	} else {
		appExist = true
		if len(statefulSet) > 1 {
			glog.Warningf("multiple ss found for pod %s", util.PodName(pod))
		}
		ss := statefulSet[0]
		replicas = 1
		if ss.Spec.Replicas != nil {
			replicas = *ss.Spec.Replicas
		}
	}
	return
}

func (p *FloatingIPPlugin) shouldReserve(pod *corev1.Pod, keyObj *util.KeyObj,
	appExist bool, replicas int32) (bool, string, error) {
	if !appExist {
		return false, deletedAndParentAppNotExistPod, nil
	}
	index, err := parsePodIndex(pod.Name)
	if err != nil {
		return false, "", fmt.Errorf("invalid pod name %s of key %s: %v", util.PodName(pod), keyObj.KeyInDB, err)
	}
	if replicas < int32(index)+1 {
		return false, deletedAndScaledDownAppPod, nil
	} else {
		return true, "", nil
	}
	return false, deletedAndParentAppNotExistPod, nil
}

func (p *FloatingIPPlugin) shouldReleaseDuringResync(keyObj *util.KeyObj, releasePolicy constant.ReleasePolicy,
	parentAppExist bool, replicas int32) (bool, string) {
	if !parentAppExist {
		if releasePolicy != constant.ReleasePolicyNever {
			return true, deletedAndParentAppNotExistPod
		}
		return false, ""
	}
	if releasePolicy != constant.ReleasePolicyImmutable {
		// 2. deleted pods whose parent statefulset or tapp exist but is not ip immutable
		return true, deletedAndIPMutablePod
	}
	index, err := parsePodIndex(keyObj.KeyInDB)
	if err != nil {
		glog.Errorf("invalid pod name of key %s: %v", keyObj.KeyInDB, err)
		return false, ""
	}
	if replicas < int32(index)+1 {
		return true, deletedAndScaledDownAppPod
	}
	return false, ""
}

func (p *FloatingIPPlugin) getSSMap() (map[string]*appv1.StatefulSet, error) {
	sss, err := p.StatefulSetLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	key2App := make(map[string]*appv1.StatefulSet)
	for i := range sss {
		if !p.hasResourceName(&sss[i].Spec.Template.Spec) {
			continue
		}
		key2App[util.StatefulsetName(sss[i])] = sss[i]
	}
	glog.V(5).Infof("%v", key2App)
	return key2App, nil
}
