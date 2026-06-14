package main

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAppendPodsAnnotatesControllerScope(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns-abc",
				Namespace: "kube-system",
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "Deployment",
					Name: "coredns",
				}},
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "traefik-abc",
				Namespace: "kube-system",
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "Deployment",
					Name: "traefik",
				}},
			},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "helm-install-traefik-123",
				Namespace: "kube-system",
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "CronJob",
					Name: "helm-install-traefik",
				}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns-abc-1",
				Namespace: "kube-system",
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "ReplicaSet",
					Name: "coredns-abc",
				}},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "coredns"}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "traefik-abc-1",
				Namespace: "kube-system",
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "ReplicaSet",
					Name: "traefik-abc",
				}},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "traefik"}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "helm-install-traefik-123-1",
				Namespace: "kube-system",
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "Job",
					Name: "helm-install-traefik-123",
				}},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "helm"}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "payment-api-0",
				Namespace: "payment",
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
		},
	)

	var options ClusterOptions
	appendPods(ctx, client, &options)

	assertContainerScope(t, options, "kube-system", "coredns-abc-1", "coredns", "deployment", "coredns")
	assertContainerScope(t, options, "kube-system", "traefik-abc-1", "traefik", "deployment", "traefik")
	assertContainerScope(t, options, "kube-system", "helm-install-traefik-123-1", "helm", "cronjob", "helm-install-traefik")
	assertContainerScope(t, options, "payment", "payment-api-0", "app", "pod", "payment-api-0")
}

func assertContainerScope(t *testing.T, options ClusterOptions, namespace, pod, container, controllerType, controllerName string) {
	t.Helper()
	for _, got := range options.Containers {
		if got.Namespace == namespace && got.Pod == pod && got.Name == container {
			if got.ControllerType != controllerType || got.ControllerName != controllerName {
				t.Fatalf("unexpected scope for %s/%s/%s: %#v", namespace, pod, container, got)
			}
			return
		}
	}
	t.Fatalf("container not found: %s/%s/%s", namespace, pod, container)
}
