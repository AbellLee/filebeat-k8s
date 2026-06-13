package agent

import "testing"

func TestParseNodeLabels(t *testing.T) {
	labels, err := ParseNodeLabels("nodepool=online,zone=hk-1")
	if err != nil {
		t.Fatal(err)
	}
	if labels["nodepool"] != "online" || labels["zone"] != "hk-1" {
		t.Fatalf("unexpected labels: %#v", labels)
	}
	labels, err = ParseNodeLabels(`{"nodepool":"online"}`)
	if err != nil {
		t.Fatal(err)
	}
	if labels["nodepool"] != "online" {
		t.Fatalf("unexpected json labels: %#v", labels)
	}
}
