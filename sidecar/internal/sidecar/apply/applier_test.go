package apply

import (
	"os"
	"path/filepath"
	"testing"

	"filebeat-k8s/internal/control"
)

func TestApplyWritesFilesAndCleansOrphans(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fbctl-100-old.yml"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manual.yml"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	files := []control.ConfigFile{{Filename: "fbctl-100-payment.yml", Content: "hello"}}
	resp := control.DesiredConfigResponse{Changed: true, Checksum: control.ConfigSetChecksum(files), Files: files}
	if err := New(dir).Apply(resp); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "fbctl-100-payment.yml"))
	if err != nil || string(body) != "hello" {
		t.Fatalf("final file missing or wrong: %q %v", string(body), err)
	}
	if _, err := os.Stat(filepath.Join(dir, "fbctl-100-old.yml")); !os.IsNotExist(err) {
		t.Fatalf("orphan was not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "manual.yml")); err != nil {
		t.Fatalf("manual file should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, stateFilename)); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
}

func TestApplyRejectsChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	files := []control.ConfigFile{{Filename: "fbctl-100-payment.yml", Content: "hello"}}
	resp := control.DesiredConfigResponse{Changed: true, Checksum: "sha256:wrong", Files: files}
	if err := New(dir).Apply(resp); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestApplyRejectsBadFilename(t *testing.T) {
	dir := t.TempDir()
	files := []control.ConfigFile{{Filename: "fbctl-100-payment.yaml", Content: "hello"}}
	resp := control.DesiredConfigResponse{Changed: true, Checksum: control.ConfigSetChecksum(files), Files: files}
	if err := New(dir).Apply(resp); err == nil {
		t.Fatal("expected bad filename")
	}
}
