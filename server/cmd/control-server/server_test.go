package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMySQLDSNFromURL(t *testing.T) {
	dsn, err := mysqlDSN("mysql://filebeat:secret@localhost:53306/filebeat_ops?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	want := "filebeat:secret@tcp(localhost:53306)/filebeat_ops?parseTime=true"
	if dsn != want {
		t.Fatalf("dsn mismatch\nwant: %s\n got: %s", want, dsn)
	}
}

func TestParseWatchTimeoutCapsToMax(t *testing.T) {
	got := parseWatchTimeout("2m", 30*time.Second)
	if got != 30*time.Second {
		t.Fatalf("expected cap to max, got %s", got)
	}
}

func TestDecodePolicyDefaultsEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"name":"payment","cluster_id":"dev","namespace":"payment","controller_type":"deployment","controller_name":"payment-api","container_name":"app","log_type":"container_stdio"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies", strings.NewReader(body))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	p, err := decodePolicy(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Enabled || p.Priority != 100 || p.ID != "payment" {
		t.Fatalf("defaults not applied: %#v", p)
	}
}

func TestRenderPolicyPreview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name         string
		body         string
		expectedPath string
		wantParsers  bool
		wantSymlinks bool
		expectedID   string
	}{
		{
			name:         "container_stdio",
			body:         `{"name":"payment","cluster_id":"dev","namespace":"payment","controller_type":"deployment","controller_name":"payment-api","container_name":"app","log_type":"container_stdio","input_config":{"scan_frequency":"10s","exclude_files":["\\.gz$"]}}`,
			expectedPath: "/var/log/klog-stdio/payment/deployment/payment-api/*/containers/app/*.log",
			wantParsers:  true,
			wantSymlinks: true,
			expectedID:   "payment",
		},
		{
			name:         "container_file",
			body:         `{"name":"payment-file","cluster_id":"dev","namespace":"payment","controller_type":"deployment","controller_name":"payment-api","container_name":"app","log_type":"container_file","log_path":"/app/logs/access.log","input_config":{"scan_frequency":"10s","exclude_files":["\\.gz$"]}}`,
			expectedPath: "/var/log/klog/payment/deployment/payment-api/*/containers/app/app/logs/access.log",
			wantSymlinks: true,
			expectedID:   "payment-file",
		},
		{
			name:         "host_file",
			body:         `{"name":"node-message","cluster_id":"dev","log_type":"host_file","log_path":"/var/log/messages","input_config":{"scan_frequency":"10s","exclude_files":["\\.gz$"]}}`,
			expectedPath: "/var/log/messages",
			expectedID:   "node-message",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &server{}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/render-preview", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			s.renderPolicyPreview(c)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			var got struct {
				RenderedConfig   string `json:"rendered_config"`
				RenderedChecksum string `json:"rendered_checksum"`
				Policy           struct {
					ID          string         `json:"id"`
					Enabled     bool           `json:"enabled"`
					Priority    int            `json:"priority"`
					InputConfig map[string]any `json:"input_config"`
				} `json:"policy"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Policy.ID != tc.expectedID || !got.Policy.Enabled || got.Policy.Priority != 100 {
				t.Fatalf("defaults not returned: %#v", got.Policy)
			}
			if !strings.Contains(got.RenderedConfig, tc.expectedPath) {
				t.Fatalf("rendered config missing expected path:\n%s", got.RenderedConfig)
			}
			if !strings.Contains(got.RenderedConfig, "scan_frequency: 10s") || !strings.Contains(got.RenderedConfig, "exclude_files:") {
				t.Fatalf("rendered config missing input_config fields:\n%s", got.RenderedConfig)
			}
			if strings.Contains(got.RenderedConfig, "parsers:") != tc.wantParsers {
				t.Fatalf("parsers presence mismatch:\n%s", got.RenderedConfig)
			}
			if strings.Contains(got.RenderedConfig, "prospector.scanner.symlinks") != tc.wantSymlinks {
				t.Fatalf("symlinks presence mismatch:\n%s", got.RenderedConfig)
			}
			if got.Policy.InputConfig["scan_frequency"] != "10s" {
				t.Fatalf("input_config not returned: %#v", got.Policy.InputConfig)
			}
			if !strings.HasPrefix(got.RenderedChecksum, "sha256:") {
				t.Fatalf("rendered checksum not set: %q", got.RenderedChecksum)
			}
		})
	}
}

func TestRenderPolicyPreviewRejectsReservedInputConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &server{}
	body := `{"name":"bad","cluster_id":"dev","namespace":"default","controller_type":"deployment","controller_name":"nginx","container_name":"nginx","log_type":"container_stdio","input_config":{"paths":["/tmp/*.log"]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/render-preview", strings.NewReader(body))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	s.renderPolicyPreview(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "reserved field") {
		t.Fatalf("expected reserved field error, got %s", w.Body.String())
	}
}

func TestRenderPolicyPreviewRejectsNonObjectInputConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &server{}
	body := `{"name":"bad","cluster_id":"dev","namespace":"default","controller_type":"deployment","controller_name":"nginx","container_name":"nginx","log_type":"container_stdio","input_config":["scan_frequency"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/render-preview", strings.NewReader(body))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	s.renderPolicyPreview(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRenderPolicyPreviewRejectsInvalidPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &server{}
	body := `{"name":"bad","cluster_id":"dev","log_type":"host_file","log_path":"/etc/passwd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/render-preview", strings.NewReader(body))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	s.renderPolicyPreview(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestScanAgentNormalizesCapabilities(t *testing.T) {
	now := time.Now()
	agent, err := scanAgent(mockRow{
		"dev:node-1",
		"dev",
		"node-1",
		"agent-pod",
		"filebeat",
		"dev",
		"9.4.1",
		"sha256:abc",
		`{"nodepool":"online"}`,
		`{"profile":"eks","runtime":"containerd","stdio":{"status":"ok","detected_path":"/var/log/containers"},"container_file":{"status":"degraded","reason":"rootfs not found"}}`,
		sql.NullTime{Time: now, Valid: true},
		"success",
		"applied",
		"sha256:abc",
		now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Capabilities.Profile != "eks" || agent.Capabilities.Runtime != "containerd" {
		t.Fatalf("unexpected capabilities: %#v", agent.Capabilities)
	}
	if agent.Capabilities.Stdio.Status != "ok" || agent.Capabilities.ContainerFile.Status != "degraded" {
		t.Fatalf("unexpected capability details: %#v", agent.Capabilities)
	}
}

func TestScanAgentDefaultsMissingCapabilities(t *testing.T) {
	now := time.Now()
	agent, err := scanAgent(mockRow{
		"dev:node-1",
		"dev",
		"node-1",
		"",
		"",
		"",
		"",
		"",
		`{}`,
		`{}`,
		sql.NullTime{},
		"",
		"",
		"",
		now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Capabilities.Profile != "unknown" || agent.Capabilities.Stdio.Status != "unknown" || agent.Capabilities.ContainerFile.Status != "unknown" {
		t.Fatalf("missing capabilities not normalized: %#v", agent.Capabilities)
	}
}

type mockRow []any

func (r mockRow) Scan(dest ...any) error {
	for i, value := range r {
		switch target := dest[i].(type) {
		case *string:
			*target = value.(string)
		case *sql.NullTime:
			*target = value.(sql.NullTime)
		case *time.Time:
			*target = value.(time.Time)
		default:
			panic("unsupported scan target")
		}
	}
	return nil
}
