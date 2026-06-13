package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWatchFallsBackToPoll(t *testing.T) {
	var pollCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/watch" {
			http.Error(w, "nope", http.StatusInternalServerError)
			return
		}
		if r.URL.Path == "/api/v1/agent/config" {
			pollCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"changed": false, "checksum": "sha256:x"})
			return
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
	}))
	defer server.Close()
	c := New(server.URL, "token", 10*time.Millisecond)
	resp, err := c.PullConfig(context.Background(), "watch", "dev:node-a", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !pollCalled || resp.Checksum != "sha256:x" {
		t.Fatalf("fallback did not happen: called=%v resp=%#v", pollCalled, resp)
	}
}
