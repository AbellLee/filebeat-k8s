package symlink

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filebeat-k8s/internal/control"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Config struct {
	NodeName            string
	KlogDir             string
	KlogStdioDir        string
	HostFSDir           string
	HostProcDir         string
	ContainerdStateDir  string
	VarLogContainersDir string
	ReconcileInterval   time.Duration
}

type Manager struct {
	cfg      Config
	client   kubernetes.Interface
	resolver RootfsResolver
	log      *slog.Logger
}

func NewManager(cfg Config, logger *slog.Logger) (*Manager, error) {
	kcfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(kcfg)
	if err != nil {
		return nil, err
	}
	return NewManagerWithClient(cfg, client, logger), nil
}

func NewManagerWithClient(cfg Config, client kubernetes.Interface, logger *slog.Logger) *Manager {
	if cfg.ReconcileInterval <= 0 {
		cfg.ReconcileInterval = 60 * time.Second
	}
	if cfg.VarLogContainersDir == "" {
		cfg.VarLogContainersDir = "/var/log/containers"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		cfg:    cfg,
		client: client,
		log:    logger,
		resolver: RootfsResolver{
			HostFSDir:          cfg.HostFSDir,
			HostProcDir:        cfg.HostProcDir,
			ContainerdStateDir: cfg.ContainerdStateDir,
			MaxProcScan:        4096,
		},
	}
}

func (m *Manager) Run(ctx context.Context) {
	_ = m.Reconcile(ctx)
	ticker := time.NewTicker(m.cfg.ReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Reconcile(ctx); err != nil {
				m.log.Warn("symlink reconcile failed", "error", err)
			}
		}
	}
}

func (m *Manager) Reconcile(ctx context.Context) error {
	pods, err := m.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + m.cfg.NodeName})
	if err != nil {
		return err
	}
	activePodDirs := map[string]bool{}
	var firstErr error
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !isActivePod(pod) {
			continue
		}
		podDirs, err := m.syncPod(ctx, pod)
		if err != nil {
			m.log.Warn("pod symlink sync failed", "namespace", pod.Namespace, "pod", pod.Name, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
		for _, dir := range podDirs {
			activePodDirs[dir] = true
		}
	}
	if err := m.cleanupOrphans(m.cfg.KlogDir, activePodDirs); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := m.cleanupOrphans(m.cfg.KlogStdioDir, activePodDirs); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (m *Manager) syncPod(ctx context.Context, pod *corev1.Pod) ([]string, error) {
	controllerType, controllerName := m.ResolveControllerIdentity(ctx, pod)
	podDirRoot := filepath.Join(m.cfg.KlogDir, control.SafePathSegment(pod.Namespace), controllerType, controllerName, control.SafePathSegment(pod.Name))
	podDirStdio := filepath.Join(m.cfg.KlogStdioDir, control.SafePathSegment(pod.Namespace), controllerType, controllerName, control.SafePathSegment(pod.Name))
	var firstErr error
	for _, status := range pod.Status.ContainerStatuses {
		containerID := normalizeContainerID(status.ContainerID)
		if containerID == "" {
			continue
		}
		root, strategy, err := m.resolver.Resolve(containerID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			linkPath := filepath.Join(podDirRoot, "containers", control.SafePathSegment(status.Name))
			if err := ensureSymlink(linkPath, root); err != nil && firstErr == nil {
				firstErr = err
			}
			m.log.Debug("rootfs symlink synced", "pod", pod.Name, "container", status.Name, "strategy", strategy)
		}
		if err := m.syncStdioLinks(pod, status.Name, podDirStdio); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return []string{podDirRoot, podDirStdio}, firstErr
}

func (m *Manager) syncStdioLinks(pod *corev1.Pod, containerName, podDirStdio string) error {
	stdioDir := filepath.Join(podDirStdio, "containers", control.SafePathSegment(containerName))
	pattern := filepath.Join(m.cfg.VarLogContainersDir, fmt.Sprintf("%s_%s_%s-*.log", pod.Name, pod.Namespace, containerName))
	targets, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stdioDir, 0755); err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, target := range targets {
		linkPath := filepath.Join(stdioDir, filepath.Base(target))
		keep[linkPath] = true
		if err := ensureSymlink(linkPath, target); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(stdioDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(stdioDir, entry.Name())
		if !keep[path] {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) ResolveControllerIdentity(ctx context.Context, pod *corev1.Pod) (string, string) {
	if len(pod.OwnerReferences) == 0 {
		return "pod", control.SafePathSegment(pod.Name)
	}
	owner := pod.OwnerReferences[0]
	switch owner.Kind {
	case "ReplicaSet":
		rs, err := m.client.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
		if err == nil {
			for _, rsOwner := range rs.OwnerReferences {
				if rsOwner.Kind == "Deployment" {
					return "deployment", control.SafePathSegment(rsOwner.Name)
				}
			}
		}
		return "replicaset", control.SafePathSegment(owner.Name)
	case "Job":
		job, err := m.client.BatchV1().Jobs(pod.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
		if err == nil {
			for _, jobOwner := range job.OwnerReferences {
				if jobOwner.Kind == "CronJob" {
					return "cronjob", control.SafePathSegment(jobOwner.Name)
				}
			}
		}
		return "job", control.SafePathSegment(owner.Name)
	default:
		return strings.ToLower(owner.Kind), control.SafePathSegment(owner.Name)
	}
}

func (m *Manager) cleanupOrphans(root string, active map[string]bool) error {
	if root == "" {
		return nil
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	var remove []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == root {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if len(strings.Split(rel, string(filepath.Separator))) == 4 && !active[path] {
			remove = append(remove, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, path := range remove {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		pruneEmptyParents(root, filepath.Dir(path))
	}
	return nil
}

func ensureSymlink(linkPath, target string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return err
	}
	if current, err := os.Readlink(linkPath); err == nil {
		if current == target {
			return nil
		}
		if err := os.Remove(linkPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		if err := os.Remove(linkPath); err != nil {
			return err
		}
	}
	return os.Symlink(target, linkPath)
}

func pruneEmptyParents(root, dir string) {
	root = filepath.Clean(root)
	for filepath.Clean(dir) != root && strings.HasPrefix(filepath.Clean(dir), root) {
		_ = os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

func isActivePod(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded
}

func normalizeContainerID(raw string) string {
	if raw == "" {
		return ""
	}
	if _, id, ok := strings.Cut(raw, "://"); ok {
		return id
	}
	return raw
}
