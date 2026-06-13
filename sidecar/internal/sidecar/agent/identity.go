package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"filebeat-k8s/internal/control"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Identity struct {
	AgentID               string
	ClusterID             string
	NodeName              string
	PodName               string
	Namespace             string
	AgentVersion          string
	FilebeatVersion       string
	CurrentConfigChecksum string
	NodeLabels            map[string]string
}

func Build(ctx context.Context, clusterID, nodeName, podName, namespace, agentVersion, filebeatVersion, labelOverride string) (Identity, error) {
	labels, err := ParseNodeLabels(labelOverride)
	if err != nil {
		return Identity{}, err
	}
	if len(labels) == 0 {
		labels = discoverNodeLabels(ctx, nodeName)
	}
	return Identity{
		AgentID:         control.AgentID(clusterID, nodeName),
		ClusterID:       clusterID,
		NodeName:        nodeName,
		PodName:         podName,
		Namespace:       namespace,
		AgentVersion:    agentVersion,
		FilebeatVersion: filebeatVersion,
		NodeLabels:      labels,
	}, nil
}

func ParseNodeLabels(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	if strings.HasPrefix(raw, "{") {
		out := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("invalid NODE_LABELS segment %q", part)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" {
			return nil, fmt.Errorf("invalid NODE_LABELS segment %q", part)
		}
		out[k] = v
	}
	return out, nil
}

func discoverNodeLabels(ctx context.Context, nodeName string) map[string]string {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return map[string]string{}
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return map[string]string{}
	}
	node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return map[string]string{}
	}
	return copyLabels(node)
}

func copyLabels(node *corev1.Node) map[string]string {
	out := map[string]string{}
	for k, v := range node.Labels {
		out[k] = v
	}
	return out
}
