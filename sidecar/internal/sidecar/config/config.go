package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ControlServerURL             string
	AgentToken                   string
	ClusterID                    string
	NodeName                     string
	PodName                      string
	PodNamespace                 string
	NodeLabels                   string
	ConfigMode                   string
	WatchEnabled                 bool
	WatchTimeout                 time.Duration
	PollInterval                 time.Duration
	RunOnce                      bool
	AgentVersion                 string
	FilebeatVersion              string
	InputsDir                    string
	K8SProfile                   string
	ContainerFileMode            string
	KlogDir                      string
	KlogStdioDir                 string
	HostFSDir                    string
	HostProcDir                  string
	ContainerdStateDir           string
	StdioLogDirCandidates        []string
	ContainerdStateDirCandidates []string
	VarLogContainersDir          string
	ReconcileInterval            time.Duration
}

func Load() (Config, error) {
	watchEnabled, err := envBool("WATCH_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	watchTimeout, err := envDuration("WATCH_TIMEOUT", 25*time.Second)
	if err != nil {
		return Config{}, err
	}
	pollInterval, err := envDuration("POLL_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	runOnce, err := envBool("RUN_ONCE", false)
	if err != nil {
		return Config{}, err
	}
	reconcileInterval, err := envDuration("RECONCILE_INTERVAL", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	stdioCandidates, err := pathCandidates("STDIO_LOG_DIR_CANDIDATES", defaultStdioLogDirCandidates())
	if err != nil {
		return Config{}, err
	}
	containerdCandidates, err := pathCandidates("CONTAINERD_STATE_DIR_CANDIDATES", defaultContainerdStateDirCandidates())
	if err != nil {
		return Config{}, err
	}
	if override := strings.TrimSpace(os.Getenv("VAR_LOG_CONTAINERS_DIR")); override != "" {
		stdioCandidates = []string{override}
	}
	if override := strings.TrimSpace(os.Getenv("CONTAINERD_STATE_DIR")); override != "" {
		containerdCandidates = []string{override}
	}

	cfg := Config{
		ControlServerURL:             os.Getenv("CONTROL_SERVER_URL"),
		AgentToken:                   os.Getenv("AGENT_TOKEN"),
		ClusterID:                    os.Getenv("CLUSTER_ID"),
		NodeName:                     os.Getenv("NODE_NAME"),
		PodName:                      env("POD_NAME", "local-sidecar"),
		PodNamespace:                 env("POD_NAMESPACE", "default"),
		NodeLabels:                   os.Getenv("NODE_LABELS"),
		ConfigMode:                   env("CONFIG_MODE", "poll"),
		WatchEnabled:                 watchEnabled,
		WatchTimeout:                 watchTimeout,
		PollInterval:                 pollInterval,
		RunOnce:                      runOnce,
		AgentVersion:                 env("AGENT_VERSION", "dev"),
		FilebeatVersion:              env("FILEBEAT_VERSION", "unknown"),
		InputsDir:                    os.Getenv("INPUTS_DIR"),
		K8SProfile:                   env("K8S_PROFILE", "auto"),
		ContainerFileMode:            env("CONTAINER_FILE_MODE", "auto"),
		KlogDir:                      env("KLOG_DIR", "/var/log/klog"),
		KlogStdioDir:                 env("KLOG_STDIO_DIR", "/var/log/klog-stdio"),
		HostFSDir:                    env("HOSTFS_DIR", "/hostfs"),
		HostProcDir:                  env("HOSTPROC_DIR", "/hostproc"),
		ContainerdStateDir:           os.Getenv("CONTAINERD_STATE_DIR"),
		StdioLogDirCandidates:        stdioCandidates,
		ContainerdStateDirCandidates: containerdCandidates,
		VarLogContainersDir:          stdioCandidates[0],
		ReconcileInterval:            reconcileInterval,
	}
	if cfg.WatchEnabled {
		cfg.ConfigMode = "watch"
	}
	if cfg.ControlServerURL == "" {
		return cfg, fmt.Errorf("CONTROL_SERVER_URL is required")
	}
	if cfg.AgentToken == "" {
		return cfg, fmt.Errorf("AGENT_TOKEN is required")
	}
	if cfg.ClusterID == "" {
		return cfg, fmt.Errorf("CLUSTER_ID is required")
	}
	if cfg.NodeName == "" {
		return cfg, fmt.Errorf("NODE_NAME is required")
	}
	if cfg.InputsDir == "" {
		return cfg, fmt.Errorf("INPUTS_DIR is required")
	}
	if cfg.ConfigMode != "poll" && cfg.ConfigMode != "watch" {
		return cfg, fmt.Errorf("CONFIG_MODE must be poll or watch")
	}
	if !oneOf(cfg.K8SProfile, "auto", "generic", "ack", "eks", "gke", "aks", "tke") {
		return cfg, fmt.Errorf("K8S_PROFILE must be one of auto,generic,ack,eks,gke,aks,tke")
	}
	if !oneOf(cfg.ContainerFileMode, "auto", "disabled", "required") {
		return cfg, fmt.Errorf("CONTAINER_FILE_MODE must be one of auto,disabled,required")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return parsed, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return parsed, nil
}

func pathCandidates(key string, fallback []string) ([]string, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	values := splitCSV(raw)
	if len(values) == 0 {
		return nil, fmt.Errorf("%s must contain at least one path", key)
	}
	for _, value := range values {
		if !strings.HasPrefix(value, "/") {
			return nil, fmt.Errorf("%s contains non-absolute path %q", key, value)
		}
	}
	return values, nil
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

func defaultStdioLogDirCandidates() []string {
	return []string{"/var/log/containers", "/hostfs/var/log/containers"}
}

func defaultContainerdStateDirCandidates() []string {
	return []string{
		"/run/k3s/containerd/io.containerd.runtime.v2.task/k8s.io",
		"/run/containerd/io.containerd.runtime.v2.task/k8s.io",
		"/var/run/containerd/io.containerd.runtime.v2.task/k8s.io",
		"/data/container/state/io.containerd.runtime.v2.task/k8s.io",
	}
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
