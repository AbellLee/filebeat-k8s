package config

import (
	"testing"
	"time"
)

func TestLoadDefaultsAndCandidates(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.K8SProfile != "auto" || cfg.ContainerFileMode != "auto" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
	if cfg.PollInterval != 30*time.Second || cfg.WatchTimeout != 25*time.Second {
		t.Fatalf("unexpected durations: poll=%s watch=%s", cfg.PollInterval, cfg.WatchTimeout)
	}
	if got := cfg.StdioLogDirCandidates[0]; got != "/var/log/containers" {
		t.Fatalf("unexpected stdio candidate %q", got)
	}
}

func TestLoadRejectsInvalidEnv(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "bool", key: "WATCH_ENABLED", value: "sometimes"},
		{name: "duration", key: "POLL_INTERVAL", value: "0s"},
		{name: "profile", key: "K8S_PROFILE", value: "minikube"},
		{name: "container file mode", key: "CONTAINER_FILE_MODE", value: "force"},
		{name: "stdio path", key: "STDIO_LOG_DIR_CANDIDATES", value: "relative/path"},
		{name: "containerd path", key: "CONTAINERD_STATE_DIR_CANDIDATES", value: "relative/path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatalf("expected error for %s=%s", tc.key, tc.value)
			}
		})
	}
}

func TestLoadHonorsLegacyPathOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("VAR_LOG_CONTAINERS_DIR", "/custom/containers")
	t.Setenv("CONTAINERD_STATE_DIR", "/custom/containerd")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.VarLogContainersDir; got != "/custom/containers" {
		t.Fatalf("unexpected var log dir %q", got)
	}
	if got := cfg.ContainerdStateDirCandidates[0]; got != "/custom/containerd" {
		t.Fatalf("unexpected containerd dir %q", got)
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"WATCH_ENABLED",
		"WATCH_TIMEOUT",
		"POLL_INTERVAL",
		"RUN_ONCE",
		"RECONCILE_INTERVAL",
		"STDIO_LOG_DIR_CANDIDATES",
		"CONTAINERD_STATE_DIR_CANDIDATES",
		"VAR_LOG_CONTAINERS_DIR",
		"CONTAINERD_STATE_DIR",
		"K8S_PROFILE",
		"CONTAINER_FILE_MODE",
		"CONFIG_MODE",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("CONTROL_SERVER_URL", "http://control-server:8080")
	t.Setenv("AGENT_TOKEN", "token")
	t.Setenv("CLUSTER_ID", "dev")
	t.Setenv("NODE_NAME", "node-1")
	t.Setenv("INPUTS_DIR", "/tmp/inputs")
}
