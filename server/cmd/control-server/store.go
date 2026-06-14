package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"filebeat-k8s/internal/control"
)

type database struct {
	db *sql.DB
}

var errNotFound = errors.New("not found")

func (d database) createPolicy(ctx context.Context, p control.Policy, createdBy string) (control.Policy, error) {
	rendered, err := control.RenderPolicy(p)
	if err != nil {
		return control.Policy{}, err
	}
	p.CurrentRevision = 1
	checksum := control.ContentChecksum(rendered)
	fields, err := json.Marshal(p.CustomFields)
	if err != nil {
		return control.Policy{}, err
	}
	inputConfig, err := json.Marshal(p.InputConfig)
	if err != nil {
		return control.Policy{}, err
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return control.Policy{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO policies
		(id,name,cluster_id,namespace,controller_type,controller_name,pod_selector,pod_name,container_name,node_selector,log_type,log_path,enabled,priority,current_revision,custom_fields,input_config)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.ClusterID, p.Namespace, p.ControllerType, p.ControllerName, p.PodSelector, p.PodName, p.ContainerName, p.NodeSelector,
		p.LogType, p.LogPath, p.Enabled, p.Priority, p.CurrentRevision, string(fields), string(inputConfig))
	if err != nil {
		return control.Policy{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO policy_revisions (policy_id,revision,rendered_config,checksum,created_by) VALUES (?,?,?,?,?)`,
		p.ID, p.CurrentRevision, rendered, checksum, createdBy)
	if err != nil {
		return control.Policy{}, err
	}
	if err := tx.Commit(); err != nil {
		return control.Policy{}, err
	}
	p.RenderedConfig = rendered
	p.RenderedChecksum = checksum
	return p, nil
}

func (d database) updatePolicy(ctx context.Context, id string, p control.Policy, createdBy string) (control.Policy, error) {
	current, err := d.getPolicy(ctx, id)
	if err != nil {
		return control.Policy{}, err
	}
	p.ID = id
	p.CurrentRevision = current.CurrentRevision + 1
	rendered, err := control.RenderPolicy(p)
	if err != nil {
		return control.Policy{}, err
	}
	checksum := control.ContentChecksum(rendered)
	fields, err := json.Marshal(p.CustomFields)
	if err != nil {
		return control.Policy{}, err
	}
	inputConfig, err := json.Marshal(p.InputConfig)
	if err != nil {
		return control.Policy{}, err
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return control.Policy{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE policies SET
		name=?, cluster_id=?, namespace=?, controller_type=?, controller_name=?, pod_selector=?, pod_name=?, container_name=?,
		node_selector=?, log_type=?, log_path=?, enabled=?, priority=?, current_revision=?, custom_fields=?, input_config=?
		WHERE id=?`,
		p.Name, p.ClusterID, p.Namespace, p.ControllerType, p.ControllerName, p.PodSelector, p.PodName, p.ContainerName,
		p.NodeSelector, p.LogType, p.LogPath, p.Enabled, p.Priority, p.CurrentRevision, string(fields), string(inputConfig), p.ID)
	if err != nil {
		return control.Policy{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO policy_revisions (policy_id,revision,rendered_config,checksum,created_by) VALUES (?,?,?,?,?)`,
		p.ID, p.CurrentRevision, rendered, checksum, createdBy)
	if err != nil {
		return control.Policy{}, err
	}
	if err := tx.Commit(); err != nil {
		return control.Policy{}, err
	}
	p.RenderedConfig = rendered
	p.RenderedChecksum = checksum
	return p, nil
}

func (d database) deletePolicy(ctx context.Context, id string) error {
	res, err := d.db.ExecContext(ctx, `DELETE FROM policies WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errNotFound
	}
	return nil
}

func (d database) listPolicies(ctx context.Context) ([]control.Policy, error) {
	rows, err := d.db.QueryContext(ctx, selectPolicySQL+` ORDER BY p.cluster_id, p.priority, p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []control.Policy{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d database) getPolicy(ctx context.Context, id string) (control.Policy, error) {
	row := d.db.QueryRowContext(ctx, selectPolicySQL+` WHERE p.id=?`, id)
	p, err := scanPolicy(row)
	if errors.Is(err, sql.ErrNoRows) {
		return control.Policy{}, errNotFound
	}
	return p, err
}

func (d database) listRevisions(ctx context.Context, policyID string) ([]control.PolicyRevision, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT policy_id,revision,rendered_config,checksum,created_by,created_at
		FROM policy_revisions WHERE policy_id=? ORDER BY revision DESC`, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []control.PolicyRevision{}
	for rows.Next() {
		var r control.PolicyRevision
		if err := rows.Scan(&r.PolicyID, &r.Revision, &r.RenderedConfig, &r.Checksum, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d database) rollbackPolicy(ctx context.Context, policyID string, revision int, createdBy string) (control.PolicyRevision, error) {
	var rendered string
	if err := d.db.QueryRowContext(ctx, `SELECT rendered_config FROM policy_revisions WHERE policy_id=? AND revision=?`, policyID, revision).Scan(&rendered); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return control.PolicyRevision{}, errNotFound
		}
		return control.PolicyRevision{}, err
	}
	current, err := d.getPolicy(ctx, policyID)
	if err != nil {
		return control.PolicyRevision{}, err
	}
	nextRevision := current.CurrentRevision + 1
	checksum := control.ContentChecksum(rendered)
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return control.PolicyRevision{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO policy_revisions (policy_id,revision,rendered_config,checksum,created_by) VALUES (?,?,?,?,?)`,
		policyID, nextRevision, rendered, checksum, createdBy)
	if err != nil {
		return control.PolicyRevision{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE policies SET current_revision=? WHERE id=?`, nextRevision, policyID)
	if err != nil {
		return control.PolicyRevision{}, err
	}
	if err := tx.Commit(); err != nil {
		return control.PolicyRevision{}, err
	}
	return control.PolicyRevision{PolicyID: policyID, Revision: nextRevision, RenderedConfig: rendered, Checksum: checksum, CreatedBy: createdBy, CreatedAt: time.Now()}, nil
}

func (d database) upsertAgentRegistration(ctx context.Context, req control.AgentRegisterRequest) (control.Agent, error) {
	if req.ID == "" {
		req.ID = control.AgentID(req.ClusterID, req.NodeName)
	}
	labels, err := json.Marshal(req.NodeLabels)
	if err != nil {
		return control.Agent{}, err
	}
	capabilities, err := json.Marshal(control.NormalizeAgentCapabilities(req.Capabilities))
	if err != nil {
		return control.Agent{}, err
	}
	_, err = d.db.ExecContext(ctx, `INSERT INTO agents
		(id,cluster_id,node_name,pod_name,namespace,agent_version,filebeat_version,current_config_checksum,node_labels,capabilities,last_heartbeat_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,NOW())
		ON DUPLICATE KEY UPDATE
		cluster_id=VALUES(cluster_id), node_name=VALUES(node_name), pod_name=VALUES(pod_name), namespace=VALUES(namespace),
		agent_version=VALUES(agent_version), filebeat_version=VALUES(filebeat_version),
		current_config_checksum=VALUES(current_config_checksum), node_labels=VALUES(node_labels), capabilities=VALUES(capabilities), last_heartbeat_at=NOW()`,
		req.ID, req.ClusterID, req.NodeName, req.PodName, req.Namespace, req.AgentVersion, req.FilebeatVersion, req.CurrentConfigChecksum, string(labels), string(capabilities))
	if err != nil {
		return control.Agent{}, err
	}
	return d.getAgent(ctx, req.ID)
}

func (d database) heartbeat(ctx context.Context, req control.AgentHeartbeatRequest) error {
	if req.ID == "" {
		req.ID = control.AgentID(req.ClusterID, req.NodeName)
	}
	capabilities, err := json.Marshal(control.NormalizeAgentCapabilities(req.Capabilities))
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, `INSERT INTO agents (id,cluster_id,node_name,current_config_checksum,capabilities,last_heartbeat_at)
		VALUES (?,?,?,?,?,NOW())
		ON DUPLICATE KEY UPDATE current_config_checksum=VALUES(current_config_checksum), capabilities=VALUES(capabilities), last_heartbeat_at=NOW()`,
		req.ID, req.ClusterID, req.NodeName, req.CurrentConfigChecksum, string(capabilities))
	return err
}

func (d database) recordApplyResult(ctx context.Context, req control.AgentApplyResultRequest) error {
	_, err := d.db.ExecContext(ctx, `INSERT INTO agent_apply_results (agent_id,checksum,status,message) VALUES (?,?,?,?)`,
		req.AgentID, req.Checksum, req.Status, req.Message)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, `UPDATE agents SET last_apply_status=?, last_apply_message=?, last_apply_checksum=? WHERE id=?`,
		req.Status, req.Message, req.Checksum, req.AgentID)
	return err
}

func (d database) listAgents(ctx context.Context) ([]control.Agent, error) {
	rows, err := d.db.QueryContext(ctx, selectAgentSQL+` ORDER BY cluster_id,node_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []control.Agent{}
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, agent)
	}
	return out, rows.Err()
}

func (d database) getAgent(ctx context.Context, id string) (control.Agent, error) {
	row := d.db.QueryRowContext(ctx, selectAgentSQL+` WHERE id=?`, id)
	agent, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return control.Agent{}, errNotFound
	}
	return agent, err
}

func (d database) desiredFiles(ctx context.Context, agentID, clusterID string) ([]control.ConfigFile, string, error) {
	labels := map[string]string{}
	if agentID != "" {
		agent, err := d.getAgent(ctx, agentID)
		if err != nil {
			return nil, "", err
		}
		clusterID = agent.ClusterID
		labels = agent.NodeLabels
	}
	if clusterID == "" {
		return nil, "", fmt.Errorf("agent_id or cluster_id is required")
	}
	rows, err := d.db.QueryContext(ctx, `SELECT p.id,p.priority,p.node_selector,r.rendered_config
		FROM policies p
		JOIN policy_revisions r ON r.policy_id=p.id AND r.revision=p.current_revision
		WHERE p.enabled=TRUE AND p.cluster_id=?
		ORDER BY p.priority,p.id`, clusterID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var files []control.ConfigFile
	for rows.Next() {
		var id, selector, rendered string
		var priority int
		if err := rows.Scan(&id, &priority, &selector, &rendered); err != nil {
			return nil, "", err
		}
		match, err := control.MatchSelector(selector, labels)
		if err != nil {
			return nil, "", err
		}
		if !match {
			continue
		}
		files = append(files, control.ConfigFile{Filename: control.ManagedFilename(priority, id), Content: rendered})
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return files, control.ConfigSetChecksum(files), nil
}

const selectPolicySQL = `SELECT
	p.id,p.name,p.cluster_id,p.namespace,p.controller_type,p.controller_name,p.pod_selector,p.pod_name,p.container_name,
	p.node_selector,p.log_type,p.log_path,p.enabled,p.priority,p.current_revision,COALESCE(CAST(p.custom_fields AS CHAR),'{}'),
	COALESCE(CAST(p.input_config AS CHAR),'{}'),p.created_at,p.updated_at,COALESCE(r.rendered_config,''),COALESCE(r.checksum,'')
	FROM policies p
	LEFT JOIN policy_revisions r ON r.policy_id=p.id AND r.revision=p.current_revision`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPolicy(row rowScanner) (control.Policy, error) {
	var p control.Policy
	var fields string
	var inputConfig string
	if err := row.Scan(&p.ID, &p.Name, &p.ClusterID, &p.Namespace, &p.ControllerType, &p.ControllerName, &p.PodSelector, &p.PodName,
		&p.ContainerName, &p.NodeSelector, &p.LogType, &p.LogPath, &p.Enabled, &p.Priority, &p.CurrentRevision, &fields, &inputConfig, &p.CreatedAt, &p.UpdatedAt,
		&p.RenderedConfig, &p.RenderedChecksum); err != nil {
		return control.Policy{}, err
	}
	p.CustomFields = map[string]string{}
	_ = json.Unmarshal([]byte(fields), &p.CustomFields)
	p.InputConfig = map[string]any{}
	_ = json.Unmarshal([]byte(inputConfig), &p.InputConfig)
	return p, nil
}

const selectAgentSQL = `SELECT id,cluster_id,node_name,pod_name,namespace,agent_version,filebeat_version,current_config_checksum,COALESCE(CAST(node_labels AS CHAR),'{}'),COALESCE(CAST(capabilities AS CHAR),'{}'),last_heartbeat_at,last_apply_status,COALESCE(last_apply_message,''),last_apply_checksum,updated_at FROM agents`

func scanAgent(row rowScanner) (control.Agent, error) {
	var a control.Agent
	var labels string
	var capabilities string
	var heartbeat sql.NullTime
	if err := row.Scan(&a.ID, &a.ClusterID, &a.NodeName, &a.PodName, &a.Namespace, &a.AgentVersion, &a.FilebeatVersion,
		&a.CurrentConfigChecksum, &labels, &capabilities, &heartbeat, &a.LastApplyStatus, &a.LastApplyMessage, &a.LastApplyChecksum, &a.UpdatedAt); err != nil {
		return control.Agent{}, err
	}
	a.NodeLabels = map[string]string{}
	_ = json.Unmarshal([]byte(labels), &a.NodeLabels)
	_ = json.Unmarshal([]byte(capabilities), &a.Capabilities)
	a.Capabilities = control.NormalizeAgentCapabilities(a.Capabilities)
	if heartbeat.Valid {
		a.LastHeartbeatAt = &heartbeat.Time
	}
	return a, nil
}
