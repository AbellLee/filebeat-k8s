package control

import (
	"fmt"
	"path"
	"strings"
)

var supportedControllerTypes = map[string]bool{
	"deployment":  true,
	"statefulset": true,
	"daemonset":   true,
	"job":         true,
	"cronjob":     true,
	"pod":         true,
	"replicaset":  true,
}

func ApplyPolicyDefaults(p *Policy) {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.ClusterID = strings.TrimSpace(p.ClusterID)
	p.Namespace = strings.TrimSpace(p.Namespace)
	p.ControllerType = strings.ToLower(strings.TrimSpace(p.ControllerType))
	p.ControllerName = strings.TrimSpace(p.ControllerName)
	p.ContainerName = strings.TrimSpace(p.ContainerName)
	p.LogType = strings.TrimSpace(p.LogType)
	p.LogPath = strings.TrimSpace(p.LogPath)
	p.PodSelector = strings.TrimSpace(p.PodSelector)
	p.PodName = strings.TrimSpace(p.PodName)
	p.NodeSelector = strings.TrimSpace(p.NodeSelector)
	if p.ID == "" {
		p.ID = SafeName(p.Name)
	} else {
		p.ID = SafeName(p.ID)
	}
	if p.LogType == "" {
		p.LogType = LogTypeContainerStdio
	}
	if p.Priority == 0 {
		p.Priority = 100
	}
	if p.CustomFields == nil {
		p.CustomFields = map[string]string{}
	}
	if p.InputConfig == nil {
		p.InputConfig = map[string]any{}
	}
}

func ValidatePolicy(p Policy) error {
	if p.ID == "" {
		return fmt.Errorf("id is required")
	}
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if p.ClusterID == "" {
		return fmt.Errorf("cluster_id is required")
	}
	if _, err := ParseSelector(p.NodeSelector); err != nil {
		return fmt.Errorf("invalid node_selector: %w", err)
	}
	if err := ValidateInputConfig(p.InputConfig); err != nil {
		return err
	}
	switch p.LogType {
	case LogTypeContainerStdio:
		return validateContainerScope(p, false)
	case LogTypeContainerFile:
		if err := validateContainerScope(p, true); err != nil {
			return err
		}
		if !strings.HasPrefix(p.LogPath, "/") {
			return fmt.Errorf("container_file log_path must be an absolute container path")
		}
		clean := path.Clean(p.LogPath)
		if clean == "/" || strings.Contains(clean, "..") || hasDeniedContainerPrefix(clean) {
			return fmt.Errorf("container_file log_path is not allowed: %s", p.LogPath)
		}
		return nil
	case LogTypeHostFile:
		if !isAllowedHostPath(p.LogPath) {
			return fmt.Errorf("host_file log_path is not allowed: %s", p.LogPath)
		}
		return nil
	default:
		return fmt.Errorf("unsupported log_type %q", p.LogType)
	}
}

var reservedInputConfigKeys = map[string]struct{}{
	"type":                        {},
	"id":                          {},
	"enabled":                     {},
	"paths":                       {},
	"parsers":                     {},
	"processors":                  {},
	"prospector.scanner.symlinks": {},
}

func ValidateInputConfig(config map[string]any) error {
	for key := range config {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			return fmt.Errorf("input_config contains empty key")
		}
		if _, reserved := reservedInputConfigKeys[trimmed]; reserved {
			return fmt.Errorf("input_config cannot override reserved field %q", trimmed)
		}
	}
	return nil
}

func validateContainerScope(p Policy, requireLogPath bool) error {
	if p.Namespace == "" {
		return fmt.Errorf("namespace is required for %s", p.LogType)
	}
	if p.ControllerType == "" || !supportedControllerTypes[p.ControllerType] {
		return fmt.Errorf("controller_type must be one of deployment,statefulset,daemonset,job,cronjob,pod,replicaset")
	}
	if p.ControllerName == "" {
		return fmt.Errorf("controller_name is required for %s", p.LogType)
	}
	if p.ContainerName == "" {
		return fmt.Errorf("container_name is required for %s", p.LogType)
	}
	if requireLogPath && p.LogPath == "" {
		return fmt.Errorf("log_path is required for %s", p.LogType)
	}
	return nil
}

func isAllowedHostPath(p string) bool {
	clean := path.Clean(p)
	if !strings.HasPrefix(clean, "/") || clean == "/" || strings.Contains(clean, "..") {
		return false
	}
	allowed := []string{"/var/log/", "/opt/logs/", "/data/logs/"}
	for _, prefix := range allowed {
		if strings.HasPrefix(clean+"/", prefix) || strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

func hasDeniedContainerPrefix(p string) bool {
	denied := []string{"/etc/", "/proc/", "/sys/", "/dev/", "/run/secrets/"}
	withSlash := p
	if !strings.HasSuffix(withSlash, "/") {
		withSlash += "/"
	}
	for _, prefix := range denied {
		if strings.HasPrefix(withSlash, prefix) {
			return true
		}
	}
	return false
}
