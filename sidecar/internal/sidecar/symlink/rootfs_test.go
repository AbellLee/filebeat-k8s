package symlink

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootfsResolverUsesInitPIDFirst(t *testing.T) {
	dir := t.TempDir()
	id := "abcdef1234567890"
	state := filepath.Join(dir, "state")
	hostproc := filepath.Join(dir, "proc")
	if err := os.MkdirAll(filepath.Join(state, id), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, id, "init.pid"), []byte("123"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(hostproc, "123", "root"), 0755); err != nil {
		t.Fatal(err)
	}
	resolver := RootfsResolver{HostProcDir: hostproc, ContainerdStateDir: state}
	root, strategy, err := resolver.Resolve(id)
	if err != nil {
		t.Fatal(err)
	}
	if strategy != "init_pid" || root != filepath.Join(hostproc, "123", "root") {
		t.Fatalf("unexpected resolution: %s %s", strategy, root)
	}
}

func TestRootfsResolverFallsBackToCgroup(t *testing.T) {
	dir := t.TempDir()
	id := "abcdef1234567890"
	hostproc := filepath.Join(dir, "proc")
	if err := os.MkdirAll(filepath.Join(hostproc, "234", "root"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostproc, "234", "cgroup"), []byte("cri-containerd-"+id+".scope"), 0644); err != nil {
		t.Fatal(err)
	}
	resolver := RootfsResolver{HostProcDir: hostproc, ContainerdStateDir: filepath.Join(dir, "missing")}
	root, strategy, err := resolver.Resolve(id)
	if err != nil {
		t.Fatal(err)
	}
	if strategy != "cgroup" || root != filepath.Join(hostproc, "234", "root") {
		t.Fatalf("unexpected resolution: %s %s", strategy, root)
	}
}
