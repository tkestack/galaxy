package schedulerplugin

import (
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"tkestack.io/galaxy/pkg/api/k8s/schedulerapi"
)

func TestFillNodeNameToMetaVictims(t *testing.T) {
	args := &schedulerapi.ExtenderPreemptionArgs{
		NodeNameToVictims: map[string]*schedulerapi.Victims{
			"nod1": {
				Pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							UID: "id1",
						},
					},
				},
			},
		},
	}
	fillNodeNameToMetaVictims(args)
	assert.Equal(t, len(args.NodeNameToMetaVictims), 1)
}
