package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ControlServerURL    string
	AgentToken          string
	ClusterID           string
	NodeName            string
	PodName             string
	PodNamespace        string
	NodeLabels          string
	ConfigMode          string
	WatchEnabled        bool
	WatchTimeout        time.Duration
	PollInterval        time.Duration
	RunOnce             bool
	AgentVersion        string
	FilebeatVersion     string
	InputsDir           string
	KlogDir             string
	KlogStdioDir        string
	HostFSDir           string
	HostProcDir         string
	ContainerdStateDir  string
	VarLogContainersDir string
	ReconcileInterval   time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		ControlServerURL:    os.Getenv("CONTROL_SERVER_URL"),
		AgentToken:          os.Getenv("AGENT_TOKEN"),
		ClusterID:           os.Getenv("CLUSTER_ID"),
		NodeName:            os.Getenv("NODE_NAME"),
		PodName:             env("POD_NAME", "local-sidecar"),
		PodNamespace:        env("POD_NAMESPACE", "default"),
		NodeLabels:          os.Getenv("NODE_LABELS"),
		ConfigMode:          env("CONFIG_MODE", "poll"),
		WatchEnabled:        envBool("WATCH_ENABLED", false),
		WatchTimeout:        envDuration("WATCH_TIMEOUT", 25*time.Second),
		PollInterval:        envDuration("POLL_INTERVAL", 30*time.Second),
		RunOnce:             envBool("RUN_ONCE", false),
		AgentVersion:        env("AGENT_VERSION", "dev"),
		FilebeatVersion:     env("FILEBEAT_VERSION", "unknown"),
		InputsDir:           os.Getenv("INPUTS_DIR"),
		KlogDir:             env("KLOG_DIR", "/var/log/klog"),
		KlogStdioDir:        env("KLOG_STDIO_DIR", "/var/log/klog-stdio"),
		HostFSDir:           env("HOSTFS_DIR", "/hostfs"),
		HostProcDir:         env("HOSTPROC_DIR", "/hostproc"),
		ContainerdStateDir:  os.Getenv("CONTAINERD_STATE_DIR"),
		VarLogContainersDir: env("VAR_LOG_CONTAINERS_DIR", "/var/log/containers"),
		ReconcileInterval:   envDuration("RECONCILE_INTERVAL", 60*time.Second),
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
	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}
