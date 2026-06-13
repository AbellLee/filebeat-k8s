package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

type mockCallResourceResponseSender struct {
	response *backend.CallResourceResponse
}

func (s *mockCallResourceResponseSender) Send(response *backend.CallResourceResponse) error {
	s.response = response
	return nil
}

func TestCallResourceRequiresControlServerURL(t *testing.T) {
	app := &App{}
	var sender mockCallResourceResponseSender

	err := app.CallResource(context.Background(), &backend.CallResourceRequest{
		Method: http.MethodGet,
		Path:   "policies",
	}, &sender)

	if err != nil {
		t.Fatal(err)
	}
	if sender.response == nil || sender.response.Status != http.StatusBadRequest {
		t.Fatalf("expected 400 response, got %#v", sender.response)
	}
}

func TestCallResourceProxiesKnownPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/policies" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if r.URL.RawQuery != "cluster_id=dev" {
			t.Fatalf("unexpected query %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"payment-app"}]`))
	}))
	defer upstream.Close()
	app := &App{settings: settings{ControlServerURL: upstream.URL}, client: upstream.Client()}
	var sender mockCallResourceResponseSender

	err := app.CallResource(context.Background(), &backend.CallResourceRequest{
		Method: http.MethodGet,
		Path:   "policies",
		URL:    "/api/plugins/filebeat-k8s-app/resources/policies?cluster_id=dev",
	}, &sender)

	if err != nil {
		t.Fatal(err)
	}
	if sender.response == nil || sender.response.Status != http.StatusOK {
		t.Fatalf("expected 200 response, got %#v", sender.response)
	}
	if !bytes.Contains(sender.response.Body, []byte("payment-app")) {
		t.Fatalf("unexpected body %s", sender.response.Body)
	}
}

func TestCallResourceForwardsWriteUserAndToken(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-User"); got != "grafana:abell" {
			t.Fatalf("unexpected X-User %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected Authorization %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"payment-app"}`))
	}))
	defer upstream.Close()
	app := &App{settings: settings{ControlServerURL: upstream.URL, AdminToken: "secret"}, client: upstream.Client()}
	var sender mockCallResourceResponseSender

	err := app.CallResource(context.Background(), &backend.CallResourceRequest{
		Method: http.MethodPost,
		Path:   "policies",
		Body:   []byte(`{"name":"payment app"}`),
		PluginContext: backend.PluginContext{
			User: &backend.User{Login: "abell"},
		},
		Headers: map[string][]string{"Content-Type": []string{"application/json"}},
	}, &sender)

	if err != nil {
		t.Fatal(err)
	}
	if sender.response == nil || sender.response.Status != http.StatusOK {
		t.Fatalf("expected 200 response, got %#v", sender.response)
	}
}

func TestCallResourceNormalizesUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid policy"}`))
	}))
	defer upstream.Close()
	app := &App{settings: settings{ControlServerURL: upstream.URL}, client: upstream.Client()}
	var sender mockCallResourceResponseSender

	err := app.CallResource(context.Background(), &backend.CallResourceRequest{
		Method: http.MethodPost,
		Path:   "policies/render-preview",
	}, &sender)

	if err != nil {
		t.Fatal(err)
	}
	if sender.response == nil || sender.response.Status != http.StatusBadRequest {
		t.Fatalf("expected 400 response, got %#v", sender.response)
	}
	var body errorResponse
	if err := json.Unmarshal(sender.response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "invalid policy" || body.Status != http.StatusBadRequest {
		t.Fatalf("unexpected error body: %#v", body)
	}
}
