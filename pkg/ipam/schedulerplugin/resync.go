package schedulerplugin

import (
	"fmt"
	"strings"

	tappv1 "git.code.oa.com/gaia/tapp-controller/pkg/apis/tappcontroller/v1alpha1"
	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (p *FloatingIPPlugin) storeReady() bool {
	if !p.TAppHasSynced() {
		glog.V(3).Infof("the tapp store has not been synced yet")
		return false
	}
	if !p.PodHasSynced() {
		glog.V(3).Infof("the pod store has not been synced yet")
		return false
	}
	return true
}

// resyncPod releases ips from
// 1. deleted pods whose parent is not tapp
// 2. deleted pods whose parent is an unexisting tapp
// 3. deleted pods whose parent tapp exist but is not fipInvariant
// 4. deleted pods whose parent tapp exist but instance status in tapp.spec.statuses = killed
// 5. existing pods but its status is evicted (TApp doesn't have evicted pods)
func (p *FloatingIPPlugin) resyncPod() error {
	if !p.storeReady() {
		return nil
	}
	all, err := p.ipam.QueryByPrefix("")
	if err != nil {
		return err
	}
	podInDB := make(map[string]string) // podFullName to TappFullName
	for _, key := range all {
		if key == "" {
			continue
		}
		tappName, _, namespace := resolveTAppPodName(key)
		if namespace == "" {
			glog.Warningf("unexpected key: %s", key)
			continue
		}
		podInDB[key] = fmtKey(namespace, tappName)
	}
	pods, err := p.PodLister.List(p.objectSelector)
	if err != nil {
		return err
	}
	existPods := map[string]*corev1.Pod{}
	for i := range pods {
		if evicted(pods[i]) {
			// 5. existing pods but its status is evicted (TApp doesn't have evicted pods, it simply deletes pods which are evicted by kubelet)
			if err := p.releasePodIP(keyInDB(pods[i])); err != nil {
				glog.Warning(err)
			}
		}
		existPods[keyInDB(pods[i])] = pods[i]
	}
	tappMap, err := p.getTAppMap()
	if err != nil {
		return err
	}
	glog.V(4).Infof("existPods %v", existPods)
	for podFullName, tappFullName := range podInDB {
		if _, ok := existPods[podFullName]; ok {
			continue
		}
		// we can't get labels of not exist pod, so get them from it's rs or tapp
		var tapp *tappv1.TApp
		if existTapp, ok := tappMap[tappFullName]; ok {
			tapp = existTapp
		} else {
			// 1. deleted pods whose parent is not tapp
			// 2. deleted pods whose parent is an unexisting tapp
			if err := p.releasePodIP(podFullName); err != nil {
				glog.Warning(err)
			}
			continue
		}
		if !p.wantedObject(&tapp.ObjectMeta) {
			continue
		}
		if !p.fipInvariantSeletor.Matches(labels.Set(tapp.GetLabels())) {
			// 3. deleted pods whose parent tapp exist but is not fipInvariant
			if err := p.releasePodIP(podFullName); err != nil {
				glog.Warning(err)
			}
			continue
		}
		// ns_tapp-12, "12" = str[(2+1+4+1):]
		podId := podFullName[len(tapp.Namespace)+1+len(tapp.Name)+1:]
		if status := tapp.Spec.Statuses[podId]; tappInstanceKilled(status) {
			// 4. deleted pods whose parent tapp exist but instance status in tapp.spec.statuses = killed
			if err := p.releasePodIP(podFullName); err != nil {
				glog.Warning(err)
			}
		}
	}
	return nil
}

func (p *FloatingIPPlugin) getTAppMap() (map[string]*tappv1.TApp, error) {
	tApps, err := p.TAppLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	app2TApp := make(map[string]*tappv1.TApp)
	for i := range tApps {
		if !p.wantedObject(&tApps[i].ObjectMeta) {
			continue
		}
		app2TApp[TAppFullName(tApps[i])] = tApps[i]
	}
	glog.V(4).Infof("%v", app2TApp)
	return app2TApp, nil
}

func keyInDB(pod *corev1.Pod) string {
	return fmt.Sprintf("%s_%s", pod.Namespace, pod.Name)
}

func fmtKey(name, namespace string) string {
	return fmt.Sprintf("%s_%s", namespace, name)
}

func TAppFullName(tapp *tappv1.TApp) string {
	return fmt.Sprintf("%s_%s", tapp.Namespace, tapp.Name)
}

// resolveTAppPodName returns tappname, podId, namespace
func resolveTAppPodName(podFullName string) (string, string, string) {
	// tappname-id_namespace, e.g. default_fip-0
	// _ is not a valid char in appname
	parts := strings.SplitN(podFullName, "_", 2)
	if len(parts) != 2 {
		return "", "", ""
	}
	lastIndex := strings.LastIndexByte(parts[1], '-')
	if lastIndex == -1 {
		return "", "", ""
	}
	return parts[1][:lastIndex], parts[1][lastIndex+1:], parts[0]
}

func ownerIsTApp(pod *corev1.Pod) bool {
	for i := range pod.OwnerReferences {
		if pod.OwnerReferences[i].Kind == "TApp" {
			return true
		}
	}
	return false
}
