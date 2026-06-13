package symlink

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RootfsResolver struct {
	HostFSDir          string
	HostProcDir        string
	ContainerdStateDir string
	MaxProcScan        int
}

func (r RootfsResolver) Resolve(containerID string) (string, string, error) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return "", "", fmt.Errorf("container id is empty")
	}
	if r.MaxProcScan <= 0 {
		r.MaxProcScan = 4096
	}
	prefixes := r.containerdPrefixes()
	for _, prefix := range prefixes {
		if target, ok := r.rootFromInitPID(prefix, containerID); ok {
			return target, "init_pid", nil
		}
	}
	if target, ok := r.rootFromCgroup(containerID); ok {
		return target, "cgroup", nil
	}
	for _, prefix := range prefixes {
		target := filepath.Join(prefix, containerID, "rootfs")
		if exists(target) {
			return target, "bundle_rootfs", nil
		}
	}
	return "", "", fmt.Errorf("rootfs not found for container %s", shortID(containerID))
}

func (r RootfsResolver) rootFromInitPID(prefix, containerID string) (string, bool) {
	body, err := os.ReadFile(filepath.Join(prefix, containerID, "init.pid"))
	if err != nil {
		return "", false
	}
	pid := strings.TrimSpace(string(body))
	if pid == "" {
		return "", false
	}
	root := filepath.Join(r.hostProcDir(), pid, "root")
	return root, exists(root)
}

func (r RootfsResolver) rootFromCgroup(containerID string) (string, bool) {
	entries, err := os.ReadDir(r.hostProcDir())
	if err != nil {
		return "", false
	}
	short := shortID(containerID)
	scanned := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "" || name[0] < '0' || name[0] > '9' {
			continue
		}
		scanned++
		if scanned > r.MaxProcScan {
			break
		}
		cgroupPath := filepath.Join(r.hostProcDir(), name, "cgroup")
		body, err := os.ReadFile(cgroupPath)
		if err != nil {
			continue
		}
		text := string(body)
		if strings.Contains(text, containerID) || (len(short) >= 12 && strings.Contains(text, short)) {
			root := filepath.Join(r.hostProcDir(), name, "root")
			if exists(root) {
				return root, true
			}
		}
	}
	return "", false
}

func (r RootfsResolver) containerdPrefixes() []string {
	if r.ContainerdStateDir != "" {
		return []string{r.hostPath(r.ContainerdStateDir)}
	}
	candidates := []string{
		"/run/k3s/containerd/io.containerd.runtime.v2.task/k8s.io",
		"/run/containerd/io.containerd.runtime.v2.task/k8s.io",
		"/var/run/containerd/io.containerd.runtime.v2.task/k8s.io",
		"/data/container/state/io.containerd.runtime.v2.task/k8s.io",
	}
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, r.hostPath(c))
	}
	return out
}

func (r RootfsResolver) hostPath(p string) string {
	if filepath.VolumeName(p) != "" {
		return filepath.Clean(p)
	}
	if strings.HasPrefix(filepath.Clean(p), filepath.Clean(r.hostFSDir())) {
		return filepath.Clean(p)
	}
	return filepath.Join(r.hostFSDir(), strings.TrimPrefix(filepath.FromSlash(p), string(filepath.Separator)))
}

func (r RootfsResolver) hostFSDir() string {
	if r.HostFSDir == "" {
		return "/hostfs"
	}
	return r.HostFSDir
}

func (r RootfsResolver) hostProcDir() string {
	if r.HostProcDir == "" {
		return "/hostproc"
	}
	return r.HostProcDir
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
