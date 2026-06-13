package control

import (
	"fmt"
	"sort"
	"strings"
)

func ParseSelector(selector string) (map[string]string, error) {
	out := map[string]string{}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return out, nil
	}
	for _, part := range strings.Split(selector, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("invalid selector segment %q", part)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			return nil, fmt.Errorf("invalid selector segment %q", part)
		}
		out[k] = v
	}
	return out, nil
}

func MatchSelector(selector string, labels map[string]string) (bool, error) {
	want, err := ParseSelector(selector)
	if err != nil {
		return false, err
	}
	for k, v := range want {
		if labels[k] != v {
			return false, nil
		}
	}
	return true, nil
}

func SelectorFromLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, ",")
}
