package capability

import (
	"context"
	"os"
	"strings"

	"filebeat-k8s/internal/control"
	"filebeat-k8s/sidecar/internal/sidecar/symlink"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Options struct {
	NodeName                     string
	PodName                      string
	PodNamespace                 string
	Profile                      string
	ContainerFileMode            string
	HostFSDir                    string
	HostProcDir                  string
	ContainerdStateDir           string
	StdioLogDirCandidates        []string
	ContainerdStateDirCandidates []string
}

func Detect(ctx context.Context, opts Options) control.AgentCapabilities {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return detectWithObjects(opts, nil, nil, "", err.Error())
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return detectWithObjects(opts, nil, nil, "", err.Error())
	}
	return DetectWithClient(ctx, client, opts)
}

func DetectWithClient(ctx context.Context, client kubernetes.Interface, opts Options) control.AgentCapabilities {
	var node *corev1.Node
	var pod *corev1.Pod
	var reason string
	var probeContainerID string
	if opts.NodeName != "" {
		if got, err := client.CoreV1().Nodes().Get(ctx, opts.NodeName, metav1.GetOptions{}); err == nil {
			node = got
		} else {
			reason = err.Error()
		}
	}
	if opts.PodName != "" && opts.PodNamespace != "" {
		if got, err := client.CoreV1().Pods(opts.PodNamespace).Get(ctx, opts.PodName, metav1.GetOptions{}); err == nil {
			pod = got
		} else if reason == "" {
			reason = err.Error()
		}
	}
	probeContainerID = firstContainerID(pod)
	if probeContainerID == "" && opts.NodeName != "" {
		if pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + opts.NodeName}); err == nil {
			probeContainerID = firstContainerIDFromPods(pods.Items)
		} else if reason == "" {
			reason = err.Error()
		}
	}
	return detectWithObjects(opts, node, pod, probeContainerID, reason)
}

func detectWithObjects(opts Options, node *corev1.Node, pod *corev1.Pod, probeContainerID string, discoveryReason string) control.AgentCapabilities {
	runtime := detectRuntime(node, pod, probeContainerID)
	capabilities := control.AgentCapabilities{
		Profile: resolveProfile(opts.Profile, node),
		Runtime: runtime,
		Stdio:   detectStdio(opts.StdioLogDirCandidates),
	}
	capabilities.ContainerFile = detectContainerFile(opts, probeContainerID, runtime)
	if discoveryReason != "" {
		appendReason(&capabilities.Stdio, discoveryReason)
		appendReason(&capabilities.ContainerFile, discoveryReason)
	}
	return control.NormalizeAgentCapabilities(capabilities)
}

func resolveProfile(profile string, node *corev1.Node) string {
	profile = strings.TrimSpace(strings.ToLower(profile))
	if profile != "" && profile != "auto" {
		return profile
	}
	if node == nil {
		return control.CapabilityStatusUnknown
	}
	text := strings.ToLower(node.Spec.ProviderID + " " + labelsText(node.Labels))
	switch {
	case strings.Contains(text, "aliyun") || strings.Contains(text, "alibaba") || strings.Contains(text, "ack.aliyun") || strings.Contains(text, "aliyuncs"):
		return "ack"
	case strings.Contains(text, "aws") || strings.Contains(text, "amazonaws") || strings.Contains(text, "eks.amazonaws.com"):
		return "eks"
	case strings.Contains(text, "gce://") || strings.Contains(text, "google") || strings.Contains(text, "gke") || strings.Contains(text, "container.googleapis.com"):
		return "gke"
	case strings.Contains(text, "azure") || strings.Contains(text, "aks") || strings.Contains(text, "kubernetes.azure.com"):
		return "aks"
	case strings.Contains(text, "qcloud") || strings.Contains(text, "tencent") || strings.Contains(text, "tke"):
		return "tke"
	default:
		return "generic"
	}
}

func labelsText(labels map[string]string) string {
	parts := make([]string, 0, len(labels)*2)
	for key, value := range labels {
		parts = append(parts, key, value)
	}
	return strings.Join(parts, " ")
}

func detectRuntime(node *corev1.Node, pod *corev1.Pod, probeContainerID string) string {
	if runtime := runtimeFromContainerID(firstContainerID(pod)); runtime != "" {
		return runtime
	}
	if runtime := runtimeFromContainerID(probeContainerID); runtime != "" {
		return runtime
	}
	if node != nil {
		if runtime, _, ok := strings.Cut(node.Status.NodeInfo.ContainerRuntimeVersion, "://"); ok && runtime != "" {
			return strings.ToLower(runtime)
		}
	}
	return control.CapabilityStatusUnknown
}

func runtimeFromContainerID(containerID string) string {
	prefix, _, ok := strings.Cut(containerID, "://")
	if !ok || prefix == "" {
		return ""
	}
	switch prefix {
	case "cri-o":
		return "crio"
	default:
		return strings.ToLower(prefix)
	}
}

func detectStdio(candidates []string) control.CapabilityDetail {
	detail := control.CapabilityDetail{DetectedPaths: candidates}
	for _, candidate := range candidates {
		if isDir(candidate) {
			detail.Status = control.CapabilityStatusOK
			detail.DetectedPath = candidate
			return detail
		}
	}
	detail.Status = control.CapabilityStatusDegraded
	detail.Reason = "stdout log directory not found"
	return detail
}

func detectContainerFile(opts Options, probeContainerID string, runtime string) control.CapabilityDetail {
	if opts.ContainerFileMode == "disabled" {
		return control.CapabilityDetail{Status: control.CapabilityStatusUnsupported, Reason: "CONTAINER_FILE_MODE=disabled"}
	}
	if runtime == "docker" || runtime == "crio" {
		return control.CapabilityDetail{Status: control.CapabilityStatusUnsupported, Reason: runtime + " container_file is not supported in this version"}
	}
	if runtime == control.CapabilityStatusUnknown {
		return control.CapabilityDetail{Status: control.CapabilityStatusDegraded, Reason: "container runtime is unknown"}
	}
	containerID := normalizeContainerID(probeContainerID)
	if containerID == "" {
		return control.CapabilityDetail{Status: control.CapabilityStatusDegraded, Reason: "container id is unavailable"}
	}
	resolver := symlink.RootfsResolver{
		HostFSDir:                    opts.HostFSDir,
		HostProcDir:                  opts.HostProcDir,
		ContainerdStateDir:           opts.ContainerdStateDir,
		ContainerdStateDirCandidates: opts.ContainerdStateDirCandidates,
		MaxProcScan:                  4096,
	}
	root, strategy, err := resolver.Resolve(containerID)
	if err != nil {
		return control.CapabilityDetail{
			Status:        control.CapabilityStatusDegraded,
			Reason:        err.Error(),
			DetectedPaths: opts.ContainerdStateDirCandidates,
		}
	}
	return control.CapabilityDetail{Status: control.CapabilityStatusOK, Reason: strategy, DetectedPath: root, DetectedPaths: opts.ContainerdStateDirCandidates}
}

func firstContainerID(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}
	for _, status := range pod.Status.ContainerStatuses {
		id := normalizeContainerID(status.ContainerID)
		if id != "" {
			return id
		}
	}
	return ""
}

func firstContainerIDFromPods(pods []corev1.Pod) string {
	for i := range pods {
		pod := &pods[i]
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
			continue
		}
		if id := firstContainerID(pod); id != "" {
			return id
		}
	}
	return ""
}

func normalizeContainerID(raw string) string {
	if _, id, ok := strings.Cut(raw, "://"); ok {
		return strings.TrimSpace(id)
	}
	return strings.TrimSpace(raw)
}

func appendReason(detail *control.CapabilityDetail, reason string) {
	if detail.Reason == "" {
		detail.Reason = reason
	}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
