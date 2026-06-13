package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"filebeat-k8s/internal/control"

	"github.com/gin-gonic/gin"
)

func (s *server) createPolicy(c *gin.Context) {
	p, err := decodePolicy(c, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	out, err := s.db.createPolicy(c.Request.Context(), p, createdBy(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (s *server) renderPolicyPreview(c *gin.Context) {
	p, err := decodePolicy(c, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rendered, err := control.RenderPolicy(p)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p.RenderedConfig = rendered
	p.RenderedChecksum = control.ContentChecksum(rendered)
	c.JSON(http.StatusOK, gin.H{
		"policy":            p,
		"rendered_config":   rendered,
		"rendered_checksum": p.RenderedChecksum,
	})
}

func (s *server) listPolicies(c *gin.Context) {
	policies, err := s.db.listPolicies(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, policies)
}

func (s *server) getPolicy(c *gin.Context) {
	policy, err := s.db.getPolicy(c.Request.Context(), c.Param("id"))
	if err != nil {
		statusError(c, err)
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (s *server) updatePolicy(c *gin.Context) {
	current, err := s.db.getPolicy(c.Request.Context(), c.Param("id"))
	if err != nil {
		statusError(c, err)
		return
	}
	p, err := decodePolicy(c, &current)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	out, err := s.db.updatePolicy(c.Request.Context(), c.Param("id"), p, createdBy(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (s *server) deletePolicy(c *gin.Context) {
	if err := s.db.deletePolicy(c.Request.Context(), c.Param("id")); err != nil {
		statusError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (s *server) listRevisions(c *gin.Context) {
	revisions, err := s.db.listRevisions(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, revisions)
}

func (s *server) rollbackPolicy(c *gin.Context) {
	var req struct {
		Revision int `json:"revision"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Revision <= 0 {
		if q := c.Query("revision"); q != "" {
			req.Revision, _ = strconv.Atoi(q)
		}
	}
	if req.Revision <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "revision is required"})
		return
	}
	revision, err := s.db.rollbackPolicy(c.Request.Context(), c.Param("id"), req.Revision, createdBy(c))
	if err != nil {
		statusError(c, err)
		return
	}
	c.JSON(http.StatusOK, revision)
}

func (s *server) policyByIDAlias(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "id", Value: id})
	switch c.Request.Method {
	case http.MethodGet:
		s.getPolicy(c)
	case http.MethodPut:
		s.updatePolicy(c)
	case http.MethodDelete:
		s.deletePolicy(c)
	}
}

func (s *server) policyRevisionsAlias(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "id", Value: id})
	s.listRevisions(c)
}

func (s *server) policyRollbackAlias(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "id", Value: id})
	s.rollbackPolicy(c)
}

func (s *server) listAgents(c *gin.Context) {
	agents, err := s.db.listAgents(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, agents)
}

func (s *server) getAgent(c *gin.Context) {
	agent, err := s.db.getAgent(c.Request.Context(), c.Param("id"))
	if err != nil {
		statusError(c, err)
		return
	}
	c.JSON(http.StatusOK, agent)
}

func (s *server) registerAgent(c *gin.Context) {
	var req control.AgentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ID == "" {
		req.ID = control.AgentID(req.ClusterID, req.NodeName)
	}
	if req.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_id and node_name are required"})
		return
	}
	agent, err := s.db.upsertAgentRegistration(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": agent.ID})
}

func (s *server) heartbeatAgent(c *gin.Context) {
	var req control.AgentHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ID == "" {
		req.ID = control.AgentID(req.ClusterID, req.NodeName)
	}
	if req.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id or cluster_id/node_name is required"})
		return
	}
	if err := s.db.heartbeat(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *server) getAgentConfig(c *gin.Context) {
	s.respondDesiredConfig(c, c.Query("agent_id"), c.Query("cluster_id"), c.Query("checksum"))
}

func (s *server) watchAgentConfig(c *gin.Context) {
	agentID := c.Query("agent_id")
	clusterID := c.Query("cluster_id")
	clientChecksum := c.Query("checksum")
	timeout := parseWatchTimeout(c.Query("timeout"), s.cfg.WatchMaxTimeout)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.cfg.WatchPollInterval)
	defer ticker.Stop()

	for {
		files, checksum, err := s.db.desiredFiles(c.Request.Context(), agentID, clusterID)
		if err != nil {
			statusError(c, err)
			return
		}
		if checksum != clientChecksum {
			c.JSON(http.StatusOK, control.DesiredConfigResponse{Changed: true, Checksum: checksum, Files: files})
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-deadline.C:
			c.JSON(http.StatusOK, control.DesiredConfigResponse{Changed: false, Checksum: checksum})
			return
		case <-ticker.C:
		}
	}
}

func (s *server) respondDesiredConfig(c *gin.Context, agentID, clusterID, clientChecksum string) {
	files, checksum, err := s.db.desiredFiles(c.Request.Context(), agentID, clusterID)
	if err != nil {
		statusError(c, err)
		return
	}
	if checksum == clientChecksum {
		c.JSON(http.StatusOK, control.DesiredConfigResponse{Changed: false, Checksum: checksum})
		return
	}
	c.JSON(http.StatusOK, control.DesiredConfigResponse{Changed: true, Checksum: checksum, Files: files})
}

func (s *server) applyResult(c *gin.Context) {
	var req control.AgentApplyResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.AgentID == "" || req.Status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_id and status are required"})
		return
	}
	if err := s.db.recordApplyResult(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func decodePolicy(c *gin.Context, existing *control.Policy) (control.Policy, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return control.Policy{}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return control.Policy{}, err
	}
	p := control.Policy{}
	if existing != nil {
		p = *existing
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return control.Policy{}, err
	}
	if existing == nil {
		if _, ok := raw["enabled"]; !ok {
			p.Enabled = true
		}
	} else if _, ok := raw["enabled"]; !ok {
		p.Enabled = existing.Enabled
	}
	control.ApplyPolicyDefaults(&p)
	if err := control.ValidatePolicy(p); err != nil {
		return control.Policy{}, err
	}
	return p, nil
}

func parseWatchTimeout(raw string, max time.Duration) time.Duration {
	if raw == "" {
		if max < 25*time.Second {
			return max
		}
		return 25 * time.Second
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 25 * time.Second
	}
	if d > max {
		return max
	}
	return d
}

func statusError(c *gin.Context, err error) {
	if errors.Is(err, errNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
