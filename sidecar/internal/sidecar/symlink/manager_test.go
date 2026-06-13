package symlink

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestResolveControllerIdentityDeployment(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payment-api-abc",
				Namespace: "payment",
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "Deployment",
					Name: "payment-api",
				}},
			},
		},
	)
	manager := NewManagerWithClient(Config{}, client, nil)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "payment-api-abc-123",
		Namespace: "payment",
		OwnerReferences: []metav1.OwnerReference{{
			Kind: "ReplicaSet",
			Name: "payment-api-abc",
		}},
	}}
	kind, name := manager.ResolveControllerIdentity(ctx, pod)
	if kind != "deployment" || name != "payment-api" {
		t.Fatalf("unexpected identity %s/%s", kind, name)
	}
}
