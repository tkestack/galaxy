package e2e

import (
	"errors"
	"fmt"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/constant"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

var extraPods *v1.PodList

func CreateFakeClient() *fake.Clientset {
	fakeClient := &fake.Clientset{}
	extraPods = &v1.PodList{}
	fakeClient.AddReactor("list", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		obj := &v1.PodList{}
		podNamePrefix := "mypod"
		namespace := "mynamespace"
		for i := 0; i < 5; i++ {
			podName := fmt.Sprintf("%s-%d", podNamePrefix, i)
			pod := v1.Pod{
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
					PodIP: "10.0.0.1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					UID:       types.UID(podName),
					Namespace: namespace,
					Labels: map[string]string{
						"app": "mypod",
					},
					Annotations: map[string]string{
						constant.MultusCNIAnnotation: "mynamespace/galaxy-flannel@eth0, mynamespace/galaxy-flannel@eth1",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "containerName",
							Image: "containerImage",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "volumeMountName",
									ReadOnly:  false,
									MountPath: "/mnt",
								},
							},
						},
					},
					NodeName: "mynode",
				},
			}
			obj.Items = append(obj.Items, pod)
		}
		for _, pod := range extraPods.Items {
			obj.Items = append(obj.Items, pod)
		}
		return true, obj, nil
	})
	fakeClient.AddReactor("create", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(core.CreateAction)
		pod := createAction.GetObject().(*v1.Pod)
		extraPods.Items = append(extraPods.Items, *pod)
		return true, createAction.GetObject(), nil
	})
	fakeClient.AddReactor("get", "pods", func(action core.Action) (bool, runtime.Object, error) {
		testPod := &v1.Pod{
			Status: v1.PodStatus{
				Phase: v1.PodRunning,
				PodIP: "10.0.0.2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "mynamespace",
				Name:      "mypod-0",
				Annotations: map[string]string{
					constant.MultusCNIAnnotation: "mynamespace/galaxy-flannel@eth0, mynamespace/galaxy-flannel@eth1",
				},
			},
			Spec: v1.PodSpec{
				NodeName: "node1",
			},
		}
		podName := action.(core.GetAction).GetName()
		if podName == testPod.Name {
			return true, testPod, nil
		}
		return true, nil, errors.New(fmt.Sprintf("No pod named %s", podName))
	})

	return fakeClient
}
