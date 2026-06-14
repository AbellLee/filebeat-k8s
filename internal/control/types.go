package control

import "time"

const (
	LogTypeContainerStdio = "container_stdio"
	LogTypeContainerFile  = "container_file"
	LogTypeHostFile       = "host_file"
)

type Policy struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	ClusterID         string            `json:"cluster_id"`
	Namespace         string            `json:"namespace,omitempty"`
	ControllerType    string            `json:"controller_type,omitempty"`
	ControllerName    string            `json:"controller_name,omitempty"`
	PodSelector       string            `json:"pod_selector,omitempty"`
	PodName           string            `json:"pod_name,omitempty"`
	ContainerName     string            `json:"container_name,omitempty"`
	NodeSelector      string            `json:"node_selector,omitempty"`
	LogType           string            `json:"log_type"`
	LogPath           string            `json:"log_path,omitempty"`
	Enabled           bool              `json:"enabled"`
	Priority          int               `json:"priority"`
	CurrentRevision   int               `json:"current_revision"`
	CustomFields      map[string]string `json:"custom_fields,omitempty"`
	InputConfig       map[string]any    `json:"input_config,omitempty"`
	CreatedAt         time.Time         `json:"created_at,omitempty"`
	UpdatedAt         time.Time         `json:"updated_at,omitempty"`
	RenderedConfig    string            `json:"rendered_config,omitempty"`
	RenderedChecksum  string            `json:"rendered_checksum,omitempty"`
	ControllerScopeID string            `json:"controller_scope_id,omitempty"`
}

type PolicyRevision struct {
	PolicyID       string    `json:"policy_id"`
	Revision       int       `json:"revision"`
	RenderedConfig string    `json:"rendered_config"`
	Checksum       string    `json:"checksum"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

type ConfigFile struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type DesiredConfigResponse struct {
	Changed  bool         `json:"changed"`
	Checksum string       `json:"checksum"`
	Files    []ConfigFile `json:"files,omitempty"`
}

type AgentRegisterRequest struct {
	ID                    string            `json:"id,omitempty"`
	ClusterID             string            `json:"cluster_id"`
	NodeName              string            `json:"node_name"`
	PodName               string            `json:"pod_name,omitempty"`
	Namespace             string            `json:"namespace,omitempty"`
	AgentVersion          string            `json:"agent_version,omitempty"`
	FilebeatVersion       string            `json:"filebeat_version,omitempty"`
	CurrentConfigChecksum string            `json:"current_config_checksum,omitempty"`
	NodeLabels            map[string]string `json:"node_labels,omitempty"`
	Capabilities          AgentCapabilities `json:"capabilities,omitempty"`
}

type AgentHeartbeatRequest struct {
	ID                    string            `json:"id"`
	ClusterID             string            `json:"cluster_id,omitempty"`
	NodeName              string            `json:"node_name,omitempty"`
	CurrentConfigChecksum string            `json:"current_config_checksum,omitempty"`
	Capabilities          AgentCapabilities `json:"capabilities,omitempty"`
}

type AgentApplyResultRequest struct {
	AgentID  string `json:"agent_id"`
	Checksum string `json:"checksum"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}

type Agent struct {
	ID                    string            `json:"id"`
	ClusterID             string            `json:"cluster_id"`
	NodeName              string            `json:"node_name"`
	PodName               string            `json:"pod_name,omitempty"`
	Namespace             string            `json:"namespace,omitempty"`
	AgentVersion          string            `json:"agent_version,omitempty"`
	FilebeatVersion       string            `json:"filebeat_version,omitempty"`
	CurrentConfigChecksum string            `json:"current_config_checksum,omitempty"`
	NodeLabels            map[string]string `json:"node_labels,omitempty"`
	Capabilities          AgentCapabilities `json:"capabilities"`
	LastHeartbeatAt       *time.Time        `json:"last_heartbeat_at,omitempty"`
	LastApplyStatus       string            `json:"last_apply_status,omitempty"`
	LastApplyMessage      string            `json:"last_apply_message,omitempty"`
	LastApplyChecksum     string            `json:"last_apply_checksum,omitempty"`
	UpdatedAt             time.Time         `json:"updated_at,omitempty"`
}

func AgentID(clusterID, nodeName string) string {
	if clusterID == "" || nodeName == "" {
		return ""
	}
	return clusterID + ":" + nodeName
}

const (
	CapabilityStatusOK          = "ok"
	CapabilityStatusDegraded    = "degraded"
	CapabilityStatusUnsupported = "unsupported"
	CapabilityStatusUnknown     = "unknown"
)

type AgentCapabilities struct {
	Profile       string           `json:"profile"`
	Runtime       string           `json:"runtime"`
	Stdio         CapabilityDetail `json:"stdio"`
	ContainerFile CapabilityDetail `json:"container_file"`
}

type CapabilityDetail struct {
	Status        string   `json:"status"`
	Reason        string   `json:"reason,omitempty"`
	DetectedPath  string   `json:"detected_path,omitempty"`
	DetectedPaths []string `json:"detected_paths,omitempty"`
}

func NormalizeAgentCapabilities(capabilities AgentCapabilities) AgentCapabilities {
	if capabilities.Profile == "" {
		capabilities.Profile = CapabilityStatusUnknown
	}
	if capabilities.Runtime == "" {
		capabilities.Runtime = CapabilityStatusUnknown
	}
	capabilities.Stdio = normalizeCapabilityDetail(capabilities.Stdio)
	capabilities.ContainerFile = normalizeCapabilityDetail(capabilities.ContainerFile)
	return capabilities
}

func normalizeCapabilityDetail(detail CapabilityDetail) CapabilityDetail {
	if detail.Status == "" {
		detail.Status = CapabilityStatusUnknown
	}
	return detail
}
