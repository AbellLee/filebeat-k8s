package control

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigSetChecksumOrderIndependent(t *testing.T) {
	a := []ConfigFile{{Filename: "fbctl-100-a.yml", Content: "a"}, {Filename: "fbctl-200-b.yml", Content: "b"}}
	b := []ConfigFile{{Filename: "fbctl-200-b.yml", Content: "b"}, {Filename: "fbctl-100-a.yml", Content: "a"}}
	if ConfigSetChecksum(a) != ConfigSetChecksum(b) {
		t.Fatal("checksum should be order independent")
	}
}

func TestValidateManagedFilename(t *testing.T) {
	if err := ValidateManagedFilename("fbctl-100-payment-app.yml"); err != nil {
		t.Fatalf("valid filename rejected: %v", err)
	}
	for _, name := range []string{"../fbctl-100-a.yml", "fbctl-a.yml", "other-100-a.yml", "fbctl-100-a.yaml"} {
		if err := ValidateManagedFilename(name); err == nil {
			t.Fatalf("invalid filename accepted: %s", name)
		}
	}
}

func TestMatchSelector(t *testing.T) {
	ok, err := MatchSelector("nodepool=online,zone=hk-1", map[string]string{"nodepool": "online", "zone": "hk-1"})
	if err != nil || !ok {
		t.Fatalf("selector should match, ok=%v err=%v", ok, err)
	}
	ok, err = MatchSelector("nodepool=offline", map[string]string{"nodepool": "online"})
	if err != nil || ok {
		t.Fatalf("selector should not match, ok=%v err=%v", ok, err)
	}
}

func TestRenderContainerPaths(t *testing.T) {
	p := Policy{
		ID:             "payment-app",
		Name:           "payment",
		ClusterID:      "dev",
		Namespace:      "payment",
		ControllerType: "deployment",
		ControllerName: "payment-api",
		ContainerName:  "app",
		LogType:        LogTypeContainerStdio,
		Enabled:        true,
		Priority:       100,
	}
	out, err := RenderPolicy(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "/var/log/klog-stdio/payment/deployment/payment-api/*/containers/app/*.log") {
		t.Fatalf("unexpected stdio render:\n%s", out)
	}

	p.LogType = LogTypeContainerFile
	p.LogPath = "/app/logs/*.log"
	out, err = RenderPolicy(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "/var/log/klog/payment/deployment/payment-api/*/containers/app/app/logs/*.log") {
		t.Fatalf("unexpected container file render:\n%s", out)
	}
}

func TestRenderInputConfigAddsTopLevelFields(t *testing.T) {
	p := Policy{
		ID:             "payment-app",
		Name:           "payment",
		ClusterID:      "dev",
		Namespace:      "payment",
		ControllerType: "deployment",
		ControllerName: "payment-api",
		ContainerName:  "app",
		LogType:        LogTypeContainerStdio,
		Enabled:        true,
		Priority:       100,
		InputConfig: map[string]any{
			"scan_frequency":         "10s",
			"exclude_files":          []any{`\.gz$`},
			"harvester_limit":        5,
			"recursive_glob.enabled": true,
		},
	}
	out, err := RenderPolicy(p)
	if err != nil {
		t.Fatal(err)
	}
	var rendered []map[string]any
	if err := yaml.Unmarshal([]byte(out), &rendered); err != nil {
		t.Fatalf("rendered YAML should parse: %v\n%s", err, out)
	}
	if len(rendered) != 1 {
		t.Fatalf("expected one input, got %d", len(rendered))
	}
	input := rendered[0]
	if input["scan_frequency"] != "10s" {
		t.Fatalf("scan_frequency missing: %#v", input)
	}
	if fmt.Sprint(input["harvester_limit"]) != "5" {
		t.Fatalf("harvester_limit missing: %#v", input)
	}
	excludeFiles, ok := input["exclude_files"].([]any)
	if !ok || len(excludeFiles) != 1 || excludeFiles[0] != `\.gz$` {
		t.Fatalf("exclude_files missing: %#v", input["exclude_files"])
	}
	if input["recursive_glob.enabled"] != true {
		t.Fatalf("recursive_glob.enabled missing: %#v", input)
	}
	if _, ok := input["paths"]; !ok {
		t.Fatalf("system paths field should still be present: %#v", input)
	}
}

func TestRenderInputConfigRejectsReservedFields(t *testing.T) {
	p := Policy{
		ID:             "payment-app",
		Name:           "payment",
		ClusterID:      "dev",
		Namespace:      "payment",
		ControllerType: "deployment",
		ControllerName: "payment-api",
		ContainerName:  "app",
		LogType:        LogTypeContainerStdio,
		InputConfig: map[string]any{
			"processors": []any{},
		},
	}
	if _, err := RenderPolicy(p); err == nil || !strings.Contains(err.Error(), "reserved field") {
		t.Fatalf("expected reserved field error, got %v", err)
	}
}
