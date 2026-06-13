package main

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL            string
	Port                   string
	AgentToken             string
	WatchPollInterval      time.Duration
	WatchMaxTimeout        time.Duration
	OperatorEnabled        bool
	OperatorClusterID      string
	OperatorNamespace      string
	OperatorResyncInterval time.Duration
}

func loadConfig() Config {
	return Config{
		DatabaseURL:            env("DATABASE_URL", "mysql://filebeat:filebeat@localhost:3306/filebeat_ops?parseTime=true"),
		Port:                   env("PORT", "8080"),
		AgentToken:             env("AGENT_TOKEN", "dev-agent-token"),
		WatchPollInterval:      envDuration("WATCH_POLL_INTERVAL", 2*time.Second),
		WatchMaxTimeout:        envDuration("WATCH_MAX_TIMEOUT", 60*time.Second),
		OperatorEnabled:        envBool("OPERATOR_ENABLED", false),
		OperatorClusterID:      env("OPERATOR_CLUSTER_ID", "dev"),
		OperatorNamespace:      os.Getenv("OPERATOR_NAMESPACE"),
		OperatorResyncInterval: envDuration("OPERATOR_RESYNC_INTERVAL", 30*time.Second),
	}
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
