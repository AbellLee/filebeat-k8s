export type LogType = 'container_stdio' | 'container_file' | 'host_file';

export interface Policy {
  id: string;
  name: string;
  cluster_id: string;
  namespace?: string;
  controller_type?: string;
  controller_name?: string;
  pod_selector?: string;
  pod_name?: string;
  container_name?: string;
  node_selector?: string;
  log_type: LogType | string;
  log_path?: string;
  enabled: boolean;
  priority: number;
  current_revision: number;
  custom_fields?: Record<string, string>;
  input_config?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
  rendered_config?: string;
  rendered_checksum?: string;
  controller_scope_id?: string;
}

export interface PolicyRevision {
  policy_id: string;
  revision: number;
  rendered_config: string;
  checksum: string;
  created_by: string;
  created_at: string;
}

export interface Agent {
  id: string;
  cluster_id: string;
  node_name: string;
  pod_name?: string;
  namespace?: string;
  agent_version?: string;
  filebeat_version?: string;
  current_config_checksum?: string;
  node_labels?: Record<string, string>;
  capabilities?: AgentCapabilities;
  last_heartbeat_at?: string;
  last_apply_status?: string;
  last_apply_message?: string;
  last_apply_checksum?: string;
  updated_at?: string;
}

export type CapabilityStatus = 'ok' | 'degraded' | 'unsupported' | 'unknown' | string;

export interface CapabilityDetail {
  status: CapabilityStatus;
  reason?: string;
  detected_path?: string;
  detected_paths?: string[];
}

export interface AgentCapabilities {
  profile: string;
  runtime: string;
  stdio: CapabilityDetail;
  container_file: CapabilityDetail;
}

export interface WorkloadOption {
  namespace: string;
  controller_type: string;
  name: string;
}

export interface PodOption {
  namespace: string;
  name: string;
  node_name: string;
  controller_type?: string;
  controller_name?: string;
}

export interface ContainerOption {
  namespace: string;
  pod: string;
  name: string;
  controller_type?: string;
  controller_name?: string;
}

export interface ClusterOptions {
  namespaces: string[];
  workloads: WorkloadOption[];
  pods: PodOption[];
  containers: ContainerOption[];
  node_labels: Record<string, string[]>;
  degraded?: boolean;
  message?: string;
}

export interface RenderPreviewResponse {
  policy: Policy;
  rendered_config: string;
  rendered_checksum: string;
}

export interface ApiError {
  error: string;
  status: number;
  details?: string;
}

export interface AppPluginSettings {
  controlServerUrl?: string;
}
