package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"filebeat-k8s/internal/control"
)

type Client struct {
	baseURL      string
	token        string
	watchTimeout time.Duration
	httpClient   *http.Client
}

func New(baseURL, token string, watchTimeout time.Duration) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		token:        token,
		watchTimeout: watchTimeout,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Register(ctx context.Context, req control.AgentRegisterRequest) error {
	var out struct {
		ID string `json:"id"`
	}
	return c.post(ctx, "/api/v1/agent/register", req, &out)
}

func (c *Client) Heartbeat(ctx context.Context, req control.AgentHeartbeatRequest) error {
	var out map[string]string
	return c.post(ctx, "/api/v1/agent/heartbeat", req, &out)
}

func (c *Client) ReportApplyResult(ctx context.Context, req control.AgentApplyResultRequest) error {
	var out map[string]string
	return c.post(ctx, "/api/v1/agent/apply-result", req, &out)
}

func (c *Client) PullConfig(ctx context.Context, mode, agentID, clusterID, checksum string) (control.DesiredConfigResponse, error) {
	if mode == "watch" {
		resp, err := c.watchConfig(ctx, agentID, clusterID, checksum)
		if err == nil {
			return resp, nil
		}
	}
	return c.pollConfig(ctx, agentID, clusterID, checksum)
}

func (c *Client) pollConfig(ctx context.Context, agentID, clusterID, checksum string) (control.DesiredConfigResponse, error) {
	values := url.Values{}
	values.Set("checksum", checksum)
	if agentID != "" {
		values.Set("agent_id", agentID)
	} else {
		values.Set("cluster_id", clusterID)
	}
	var out control.DesiredConfigResponse
	err := c.get(ctx, "/api/v1/agent/config?"+values.Encode(), &out, 30*time.Second)
	return out, err
}

func (c *Client) watchConfig(ctx context.Context, agentID, clusterID, checksum string) (control.DesiredConfigResponse, error) {
	values := url.Values{}
	values.Set("checksum", checksum)
	values.Set("timeout", c.watchTimeout.String())
	if agentID != "" {
		values.Set("agent_id", agentID)
	} else {
		values.Set("cluster_id", clusterID)
	}
	var out control.DesiredConfigResponse
	err := c.get(ctx, "/api/v1/agent/watch?"+values.Encode(), &out, c.watchTimeout+10*time.Second)
	return out, err
}

func (c *Client) get(ctx context.Context, path string, out any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	c.auth(req)
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
	return c.do(req, out)
}

func (c *Client) auth(req *http.Request) {
	req.Header.Set("X-Agent-Token", c.token)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s returned %d: %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}
