package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

type settings struct {
	ControlServerURL string `json:"controlServerUrl"`
	AdminToken       string
}

type errorResponse struct {
	Error   string `json:"error"`
	Status  int    `json:"status"`
	Details string `json:"details,omitempty"`
}

func loadSettings(appSettings backend.AppInstanceSettings) (settings, error) {
	var cfg settings
	if len(appSettings.JSONData) > 0 {
		if err := json.Unmarshal(appSettings.JSONData, &cfg); err != nil {
			return settings{}, fmt.Errorf("invalid jsonData: %w", err)
		}
	}
	cfg.ControlServerURL = strings.TrimRight(strings.TrimSpace(cfg.ControlServerURL), "/")
	if token := strings.TrimSpace(appSettings.DecryptedSecureJSONData["adminToken"]); token != "" {
		cfg.AdminToken = token
	}
	if cfg.ControlServerURL != "" {
		if _, err := url.ParseRequestURI(cfg.ControlServerURL); err != nil {
			return settings{}, fmt.Errorf("invalid controlServerUrl: %w", err)
		}
	}
	return cfg, nil
}

func (a *App) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	if a.settings.ControlServerURL == "" {
		return sendError(sender, http.StatusBadRequest, "controlServerUrl is not configured", "")
	}
	upstreamPath, ok := controlServerPath(req.Path)
	if !ok {
		return sendError(sender, http.StatusNotFound, "unknown resource path", req.Path)
	}
	target := a.targetURL(upstreamPath, req.URL)
	body := bytes.NewReader(req.Body)
	out, err := http.NewRequestWithContext(ctx, req.Method, target, body)
	if err != nil {
		return sendError(sender, http.StatusBadRequest, "invalid upstream request", err.Error())
	}
	copyHeader(out.Header, req.GetHTTPHeaders(), "Accept")
	copyHeader(out.Header, req.GetHTTPHeaders(), "Content-Type")
	if a.settings.AdminToken != "" {
		out.Header.Set("Authorization", "Bearer "+a.settings.AdminToken)
	}
	if isWriteMethod(req.Method) {
		out.Header.Set("X-User", grafanaUser(req.PluginContext.User))
	}

	resp, err := a.client.Do(out)
	if err != nil {
		return sendError(sender, http.StatusBadGateway, "control-server request failed", err.Error())
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return sendError(sender, http.StatusBadGateway, "control-server response read failed", err.Error())
	}
	if resp.StatusCode >= 400 {
		return sendError(sender, resp.StatusCode, upstreamError(respBody), string(respBody))
	}
	return sender.Send(&backend.CallResourceResponse{
		Status:  resp.StatusCode,
		Headers: responseHeaders(resp.Header),
		Body:    respBody,
	})
}

func controlServerPath(raw string) (string, bool) {
	path := strings.TrimPrefix(strings.TrimSpace(raw), "/")
	if path == "" {
		return "", false
	}
	if path == "readyz" || path == "healthz" {
		return "/" + path, true
	}
	allowedPrefixes := []string{
		"policies",
		"agents",
		"cluster/options",
	}
	for _, prefix := range allowedPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return "/api/v1/" + path, true
		}
	}
	return "", false
}

func (a *App) targetURL(upstreamPath, resourceURL string) string {
	target := a.settings.ControlServerURL + upstreamPath
	if resourceURL == "" {
		return target
	}
	if u, err := url.Parse(resourceURL); err == nil && u.RawQuery != "" {
		target += "?" + u.RawQuery
	}
	return target
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func grafanaUser(user *backend.User) string {
	if user == nil {
		return "grafana"
	}
	if user.Login != "" {
		return "grafana:" + user.Login
	}
	if user.Email != "" {
		return "grafana:" + user.Email
	}
	if user.Name != "" {
		return "grafana:" + user.Name
	}
	return "grafana"
}

func copyHeader(dst, src http.Header, key string) {
	if value := src.Get(key); value != "" {
		dst.Set(key, value)
	}
}

func responseHeaders(src http.Header) map[string][]string {
	headers := map[string][]string{}
	if value := src.Get("Content-Type"); value != "" {
		headers["Content-Type"] = []string{value}
	}
	return headers
}

func upstreamError(body []byte) string {
	var raw struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &raw); err == nil && raw.Error != "" {
		return raw.Error
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
		return trimmed
	}
	return "control-server returned an error"
}

func sendError(sender backend.CallResourceResponseSender, status int, message, details string) error {
	body, err := json.Marshal(errorResponse{
		Error:   message,
		Status:  status,
		Details: details,
	})
	if err != nil {
		return err
	}
	return sender.Send(&backend.CallResourceResponse{
		Status:  status,
		Headers: map[string][]string{"Content-Type": []string{"application/json"}},
		Body:    body,
	})
}
