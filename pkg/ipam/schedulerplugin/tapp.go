package schedulerplugin

import (
	"fmt"

	"git.code.oa.com/tkestack/galaxy/pkg/ipam/schedulerplugin/util"
	corev1 "k8s.io/api/core/v1"
	metaErrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	glog "k8s.io/klog"
	tappv1 "tkestack.io/tapp-controller/pkg/apis/tappcontroller/v1"
)

func tAppFullName(tapp *tappv1.TApp) string {
	return fmt.Sprintf("%s_%s", tapp.Namespace, tapp.Name)
}

func (p *FloatingIPPlugin) getTAppMap() (map[string]*tappv1.TApp, error) {
	if p.TAppLister == nil {
		return map[string]*tappv1.TApp{}, nil
	}
	tApps, err := p.TAppLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	key2App := make(map[string]*tappv1.TApp)
	for i := range tApps {
		if !p.hasResourceName(&tApps[i].Spec.Template.Spec) {
			continue
		}
		key2App[tAppFullName(tApps[i])] = tApps[i]
	}
	glog.V(5).Infof("%v", key2App)
	return key2App, nil
}

func (p *FloatingIPPlugin) getTAppReplicas(pod *corev1.Pod, keyObj *util.KeyObj) (appExist bool, replicas int32, retErr error) {
	tapp, err := p.TAppLister.TApps(pod.Namespace).Get(keyObj.AppName)
	if err != nil {
		if !metaErrs.IsNotFound(err) {
			retErr = err
			return
		}
	} else {
		appExist = true
		replicas = tapp.Spec.Replicas
	}
	return
}
