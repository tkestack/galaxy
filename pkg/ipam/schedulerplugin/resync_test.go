package schedulerplugin

import (
	"testing"

	"git.code.oa.com/gaiastack/galaxy/pkg/api/galaxy/constant"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeploymentIPPoolPrefix(t *testing.T) {
	testCases := []struct {
		dp     *appv1.Deployment
		expect string
	}{
		{dp: createDeployment("dp1", "ns1", nil), expect: "_deployment_ns1_dp1_"},
		{dp: createDeployment("dp1", "", nil), expect: "_deployment__dp1_"},
		{dp: createDeployment("dp1", "ns1", map[string]string{constant.IPPoolAnnotation: "pool1"}), expect: "_ippool__pool1_"},
	}
	for i := range testCases {
		testCase := testCases[i]
		got := deploymentIPPoolPrefix(testCase.dp)
		if testCase.expect != got {
			t.Errorf("case %d, expect %s, got %s", i, testCase.expect, got)
		}
	}
}

func createDeployment(name, namespace string, annotations map[string]string) *appv1.Deployment {
	return &appv1.Deployment{
		ObjectMeta: v1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       appv1.DeploymentSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: annotations}}}}
}
