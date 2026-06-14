package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filebeat-k8s/internal/control"

	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var filebeatPolicyGVR = schema.GroupVersionResource{
	Group:    "filebeat.ops.io",
	Version:  "v1alpha1",
	Resource: "filebeatpolicies",
}

type ClusterOptions struct {
	Namespaces []string            `json:"namespaces"`
	Workloads  []WorkloadOption    `json:"workloads"`
	Pods       []PodOption         `json:"pods"`
	Containers []ContainerOption   `json:"containers"`
	NodeLabels map[string][]string `json:"node_labels"`
	Degraded   bool                `json:"degraded,omitempty"`
	Message    string              `json:"message,omitempty"`
}

type WorkloadOption struct {
	Namespace      string `json:"namespace"`
	ControllerType string `json:"controller_type"`
	Name           string `json:"name"`
}

type PodOption struct {
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	NodeName       string `json:"node_name"`
	ControllerType string `json:"controller_type,omitempty"`
	ControllerName string `json:"controller_name,omitempty"`
}

type ContainerOption struct {
	Namespace      string `json:"namespace"`
	Pod            string `json:"pod"`
	Name           string `json:"name"`
	ControllerType string `json:"controller_type,omitempty"`
	ControllerName string `json:"controller_name,omitempty"`
}

func (s *server) clusterOptions(c *gin.Context) {
	client, _, err := buildKubeClients()
	if err != nil {
		c.JSON(200, ClusterOptions{Degraded: true, Message: err.Error(), NodeLabels: map[string][]string{}})
		return
	}
	ctx := c.Request.Context()
	options := ClusterOptions{NodeLabels: map[string][]string{}}
	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		c.JSON(200, ClusterOptions{Degraded: true, Message: err.Error(), NodeLabels: map[string][]string{}})
		return
	}
	for _, ns := range namespaces.Items {
		options.Namespaces = append(options.Namespaces, ns.Name)
	}
	sort.Strings(options.Namespaces)

	appendWorkloads(ctx, client, &options)
	appendPods(ctx, client, &options)
	appendNodeLabels(ctx, client, &options)
	c.JSON(200, options)
}

func appendWorkloads(ctx context.Context, client kubernetes.Interface, options *ClusterOptions) {
	deployments, _ := client.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, v := range deployments.Items {
		options.Workloads = append(options.Workloads, workload(v.Namespace, "deployment", v.Name))
	}
	statefulsets, _ := client.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, v := range statefulsets.Items {
		options.Workloads = append(options.Workloads, workload(v.Namespace, "statefulset", v.Name))
	}
	daemonsets, _ := client.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	for _, v := range daemonsets.Items {
		options.Workloads = append(options.Workloads, workload(v.Namespace, "daemonset", v.Name))
	}
	jobs, _ := client.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	for _, v := range jobs.Items {
		options.Workloads = append(options.Workloads, workload(v.Namespace, "job", v.Name))
	}
	cronjobs, _ := client.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
	for _, v := range cronjobs.Items {
		options.Workloads = append(options.Workloads, workload(v.Namespace, "cronjob", v.Name))
	}
	sort.Slice(options.Workloads, func(i, j int) bool {
		a, b := options.Workloads[i], options.Workloads[j]
		return a.Namespace+"/"+a.ControllerType+"/"+a.Name < b.Namespace+"/"+b.ControllerType+"/"+b.Name
	})
}

func appendPods(ctx context.Context, client kubernetes.Interface, options *ClusterOptions) {
	replicaSetOwners := replicaSetOwnerIndex(ctx, client)
	jobOwners := jobOwnerIndex(ctx, client)
	pods, _ := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	for _, pod := range pods.Items {
		controllerType, controllerName := controllerScopeForPod(pod, replicaSetOwners, jobOwners)
		options.Pods = append(options.Pods, PodOption{
			Namespace:      pod.Namespace,
			Name:           pod.Name,
			NodeName:       pod.Spec.NodeName,
			ControllerType: controllerType,
			ControllerName: controllerName,
		})
		for _, c := range pod.Spec.Containers {
			options.Containers = append(options.Containers, ContainerOption{
				Namespace:      pod.Namespace,
				Pod:            pod.Name,
				Name:           c.Name,
				ControllerType: controllerType,
				ControllerName: controllerName,
			})
		}
	}
	sort.Slice(options.Pods, func(i, j int) bool {
		a, b := options.Pods[i], options.Pods[j]
		return a.Namespace+"/"+a.ControllerType+"/"+a.ControllerName+"/"+a.Name < b.Namespace+"/"+b.ControllerType+"/"+b.ControllerName+"/"+b.Name
	})
	sort.Slice(options.Containers, func(i, j int) bool {
		a, b := options.Containers[i], options.Containers[j]
		return a.Namespace+"/"+a.ControllerType+"/"+a.ControllerName+"/"+a.Pod+"/"+a.Name < b.Namespace+"/"+b.ControllerType+"/"+b.ControllerName+"/"+b.Pod+"/"+b.Name
	})
}

func appendNodeLabels(ctx context.Context, client kubernetes.Interface, options *ClusterOptions) {
	nodes, _ := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	sets := map[string]map[string]bool{}
	for _, node := range nodes.Items {
		for k, v := range node.Labels {
			if sets[k] == nil {
				sets[k] = map[string]bool{}
			}
			sets[k][v] = true
		}
	}
	for k, values := range sets {
		for v := range values {
			options.NodeLabels[k] = append(options.NodeLabels[k], v)
		}
		sort.Strings(options.NodeLabels[k])
	}
}

func workload(namespace, controllerType, name string) WorkloadOption {
	return WorkloadOption{Namespace: namespace, ControllerType: controllerType, Name: name}
}

func buildKubeClients() (kubernetes.Interface, dynamic.Interface, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			if home, homeErr := os.UserHomeDir(); homeErr == nil {
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		}
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, nil, err
		}
	}
	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	return typed, dyn, nil
}

func (s *server) runOperator(ctx context.Context) {
	_, dyn, err := buildKubeClients()
	if err != nil {
		s.log.Warn("operator disabled because Kubernetes client is unavailable", "error", err)
		return
	}
	namespace := s.cfg.OperatorNamespace
	for {
		if err := s.operatorLoop(ctx, dyn, namespace); err != nil && ctx.Err() == nil {
			s.log.Warn("operator loop failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cfg.OperatorResyncInterval):
		}
	}
}

func (s *server) operatorLoop(ctx context.Context, dyn dynamic.Interface, namespace string) error {
	resource := dyn.Resource(filebeatPolicyGVR).Namespace(namespace)
	list, err := resource.List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range list.Items {
		s.reconcileFilebeatPolicy(ctx, dyn, namespace, &list.Items[i])
	}
	w, err := resource.Watch(ctx, metav1.ListOptions{ResourceVersion: list.GetResourceVersion()})
	if err != nil {
		return err
	}
	defer w.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-w.ResultChan():
			if !ok {
				return nil
			}
			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}
			switch event.Type {
			case watch.Added, watch.Modified:
				s.reconcileFilebeatPolicy(ctx, dyn, namespace, obj)
			case watch.Deleted:
				id := policyIDFromCR(obj)
				if err := s.db.deletePolicy(ctx, id); err != nil && !errorsIsNotFound(err) {
					s.log.Warn("operator delete policy failed", "policy_id", id, "error", err)
				}
			}
		}
	}
}

func (s *server) reconcileFilebeatPolicy(ctx context.Context, dyn dynamic.Interface, namespace string, obj *unstructured.Unstructured) {
	p := policyFromCR(obj, s.cfg.OperatorClusterID)
	control.ApplyPolicyDefaults(&p)
	if err := control.ValidatePolicy(p); err != nil {
		s.updateCRStatus(ctx, dyn, namespace, obj, "Failed", err.Error(), p.ID, 0, "")
		return
	}
	var out control.Policy
	var err error
	if _, getErr := s.db.getPolicy(ctx, p.ID); getErr != nil {
		out, err = s.db.createPolicy(ctx, p, "operator:filebeatpolicy/"+obj.GetNamespace()+"/"+obj.GetName())
	} else {
		out, err = s.db.updatePolicy(ctx, p.ID, p, "operator:filebeatpolicy/"+obj.GetNamespace()+"/"+obj.GetName())
	}
	if err != nil {
		s.updateCRStatus(ctx, dyn, namespace, obj, "Failed", err.Error(), p.ID, 0, "")
		return
	}
	s.updateCRStatus(ctx, dyn, namespace, obj, "Synced", "policy synced", out.ID, out.CurrentRevision, out.RenderedChecksum)
}

func (s *server) updateCRStatus(ctx context.Context, dyn dynamic.Interface, namespace string, obj *unstructured.Unstructured, phase, message, policyID string, revision int, checksum string) {
	status := map[string]any{
		"phase":              phase,
		"message":            message,
		"policyID":           policyID,
		"revision":           int64(revision),
		"checksum":           checksum,
		"observedGeneration": obj.GetGeneration(),
		"lastSyncTime":       time.Now().UTC().Format(time.RFC3339),
	}
	copy := obj.DeepCopy()
	copy.Object["status"] = status
	_, err := dyn.Resource(filebeatPolicyGVR).Namespace(obj.GetNamespace()).UpdateStatus(ctx, copy, metav1.UpdateOptions{})
	if err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		s.log.Warn("operator status update failed", "name", obj.GetName(), "namespace", obj.GetNamespace(), "error", err)
	}
}

func policyFromCR(obj *unstructured.Unstructured, defaultClusterID string) control.Policy {
	p := control.Policy{
		ID:             stringField(obj, "spec", "id"),
		Name:           stringField(obj, "spec", "name"),
		ClusterID:      stringField(obj, "spec", "clusterID"),
		Namespace:      stringField(obj, "spec", "namespace"),
		ControllerType: strings.ToLower(stringField(obj, "spec", "controllerType")),
		ControllerName: stringField(obj, "spec", "controllerName"),
		ContainerName:  stringField(obj, "spec", "containerName"),
		PodSelector:    matchLabelsSelector(obj, "podSelector"),
		NodeSelector:   matchLabelsSelector(obj, "nodeSelector"),
		LogType:        stringField(obj, "spec", "logType"),
		LogPath:        stringField(obj, "spec", "logPath"),
		Priority:       intField(obj, "spec", "priority"),
		Enabled:        true,
		CustomFields:   stringMapField(obj, "spec", "customFields"),
	}
	if p.ID == "" {
		p.ID = "crd-" + obj.GetNamespace() + "-" + obj.GetName()
	}
	if p.Name == "" {
		p.Name = obj.GetName()
	}
	if p.ClusterID == "" {
		p.ClusterID = defaultClusterID
	}
	if p.Namespace == "" {
		p.Namespace = namespaceSelectorFallback(obj)
	}
	if enabled, ok, _ := unstructured.NestedBool(obj.Object, "spec", "enabled"); ok {
		p.Enabled = enabled
	}
	return p
}

func policyIDFromCR(obj *unstructured.Unstructured) string {
	id := stringField(obj, "spec", "id")
	if id == "" {
		id = "crd-" + obj.GetNamespace() + "-" + obj.GetName()
	}
	return control.SafeName(id)
}

func stringField(obj *unstructured.Unstructured, fields ...string) string {
	v, _, _ := unstructured.NestedString(obj.Object, fields...)
	return v
}

func intField(obj *unstructured.Unstructured, fields ...string) int {
	v, ok, _ := unstructured.NestedInt64(obj.Object, fields...)
	if !ok {
		return 0
	}
	return int(v)
}

func stringMapField(obj *unstructured.Unstructured, fields ...string) map[string]string {
	raw, ok, _ := unstructured.NestedStringMap(obj.Object, fields...)
	if ok {
		return raw
	}
	return map[string]string{}
}

func matchLabelsSelector(obj *unstructured.Unstructured, selectorName string) string {
	labels, ok, _ := unstructured.NestedStringMap(obj.Object, "spec", selectorName, "matchLabels")
	if !ok {
		return ""
	}
	return control.SelectorFromLabels(labels)
}

func namespaceSelectorFallback(obj *unstructured.Unstructured) string {
	names, ok, _ := unstructured.NestedStringSlice(obj.Object, "spec", "namespaceSelector", "matchNames")
	if ok && len(names) > 0 {
		return names[0]
	}
	return ""
}

func errorsIsNotFound(err error) bool {
	return err == errNotFound || apierrors.IsNotFound(err)
}

func controllerNameFromOwner(pod corev1.Pod) (string, string) {
	if len(pod.OwnerReferences) == 0 {
		return "pod", pod.Name
	}
	owner := pod.OwnerReferences[0]
	return strings.ToLower(owner.Kind), owner.Name
}

func replicaSetOwnerIndex(ctx context.Context, client kubernetes.Interface) map[string]WorkloadOption {
	index := map[string]WorkloadOption{}
	replicaSets, _ := client.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	for _, rs := range replicaSets.Items {
		controllerType, controllerName := normalizeDeploymentOwner(rs)
		index[objectKey(rs.Namespace, "replicaset", rs.Name)] = workload(rs.Namespace, controllerType, controllerName)
	}
	return index
}

func jobOwnerIndex(ctx context.Context, client kubernetes.Interface) map[string]WorkloadOption {
	index := map[string]WorkloadOption{}
	jobs, _ := client.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	for _, job := range jobs.Items {
		controllerType, controllerName := normalizeCronJobOwner(job)
		index[objectKey(job.Namespace, "job", job.Name)] = workload(job.Namespace, controllerType, controllerName)
	}
	return index
}

func controllerScopeForPod(pod corev1.Pod, replicaSetOwners, jobOwners map[string]WorkloadOption) (string, string) {
	controllerType, controllerName := controllerNameFromOwner(pod)
	switch controllerType {
	case "replicaset":
		if owner, ok := replicaSetOwners[objectKey(pod.Namespace, "replicaset", controllerName)]; ok {
			return owner.ControllerType, owner.Name
		}
	case "job":
		if owner, ok := jobOwners[objectKey(pod.Namespace, "job", controllerName)]; ok {
			return owner.ControllerType, owner.Name
		}
	}
	return controllerType, controllerName
}

func normalizeDeploymentOwner(rs appsv1.ReplicaSet) (string, string) {
	for _, owner := range rs.OwnerReferences {
		if owner.Kind == "Deployment" {
			return "deployment", owner.Name
		}
	}
	return "replicaset", rs.Name
}

func normalizeCronJobOwner(job batchv1.Job) (string, string) {
	for _, owner := range job.OwnerReferences {
		if owner.Kind == "CronJob" {
			return "cronjob", owner.Name
		}
	}
	return "job", job.Name
}

func objectKey(namespace, kind, name string) string {
	return fmt.Sprintf("%s/%s/%s", namespace, kind, name)
}
