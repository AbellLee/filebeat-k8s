package capability

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"filebeat-k8s/internal/control"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDetectProfiles(t *testing.T) {
	cases := []struct {
		name       string
		providerID string
		labels     map[string]string
		want       string
	}{
		{name: "ack", providerID: "aliyun://cn-hangzhou/i-1", want: "ack"},
		{name: "eks", providerID: "aws:///us-east-1a/i-1", want: "eks"},
		{name: "gke", providerID: "gce://project/zone/node", want: "gke"},
		{name: "aks", providerID: "azure:///subscriptions/1", want: "aks"},
		{name: "tke", providerID: "qcloud:///zone/node", want: "tke"},
		{name: "generic", labels: map[string]string{"kubernetes.io/os": "linux"}, want: "generic"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(&corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: tc.labels},
				Spec:       corev1.NodeSpec{ProviderID: tc.providerID},
			})
			got := DetectWithClient(context.Background(), client, Options{NodeName: "node-1", Profile: "auto"})
			if got.Profile != tc.want {
				t.Fatalf("profile mismatch: want %s got %#v", tc.want, got)
			}
		})
	}
}

func TestDetectStdioCandidate(t *testing.T) {
	dir := t.TempDir()
	stdio := filepath.Join(dir, "containers")
	if err := os.MkdirAll(stdio, 0755); err != nil {
		t.Fatal(err)
	}
	got := DetectWithClient(context.Background(), fake.NewSimpleClientset(), Options{
		StdioLogDirCandidates: []string{filepath.Join(dir, "missing"), stdio},
	})
	if got.Stdio.Status != control.CapabilityStatusOK || got.Stdio.DetectedPath != stdio {
		t.Fatalf("unexpected stdio detection: %#v", got.Stdio)
	}
}

func TestDetectContainerFileRootfs(t *testing.T) {
	dir := t.TempDir()
	id := "abcdef1234567890"
	state := filepath.Join(dir, "state")
	hostproc := filepath.Join(dir, "proc")
	if err := os.MkdirAll(filepath.Join(state, id), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, id, "init.pid"), []byte("123"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(hostproc, "123", "root"), 0755); err != nil {
		t.Fatal(err)
	}
	client := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{
				ContainerRuntimeVersion: "containerd://2.2.3-k3s1",
			}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "filebeat"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:        "control-sidecar",
					ContainerID: "containerd://" + id,
				}},
			},
		},
	)
	got := DetectWithClient(context.Background(), client, Options{
		NodeName:                     "node-1",
		PodName:                      "agent",
		PodNamespace:                 "filebeat",
		HostProcDir:                  hostproc,
		ContainerdStateDirCandidates: []string{state},
		ContainerFileMode:            "auto",
	})
	if got.Runtime != "containerd" || got.ContainerFile.Status != control.CapabilityStatusOK {
		t.Fatalf("unexpected container_file detection: %#v", got)
	}
}

func TestDetectContainerFileFallsBackToNodePodProbe(t *testing.T) {
	dir := t.TempDir()
	id := "feedface1234567890"
	state := filepath.Join(dir, "state")
	hostproc := filepath.Join(dir, "proc")
	if err := os.MkdirAll(filepath.Join(state, id), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, id, "init.pid"), []byte("456"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(hostproc, "456", "root"), 0755); err != nil {
		t.Fatal(err)
	}
	client := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{
				ContainerRuntimeVersion: "containerd://2.2.3-k3s1",
			}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "filebeat"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "workload", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:        "app",
					ContainerID: "containerd://" + id,
				}},
			},
		},
	)
	got := DetectWithClient(context.Background(), client, Options{
		NodeName:                     "node-1",
		PodName:                      "agent",
		PodNamespace:                 "filebeat",
		HostProcDir:                  hostproc,
		ContainerdStateDirCandidates: []string{state},
		ContainerFileMode:            "auto",
	})
	if got.Runtime != "containerd" || got.ContainerFile.Status != control.CapabilityStatusOK {
		t.Fatalf("unexpected fallback detection: %#v", got)
	}
}

func TestDetectContainerFileUnsupportedRuntime(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{
				ContainerRuntimeVersion: "docker://27.0.0",
			}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "filebeat"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:        "control-sidecar",
					ContainerID: "docker://abcdef",
				}},
			},
		},
	)
	got := DetectWithClient(context.Background(), client, Options{
		NodeName:          "node-1",
		PodName:           "agent",
		PodNamespace:      "filebeat",
		ContainerFileMode: "auto",
	})
	if got.Runtime != "docker" || got.ContainerFile.Status != control.CapabilityStatusUnsupported {
		t.Fatalf("unexpected docker detection: %#v", got)
	}
}
