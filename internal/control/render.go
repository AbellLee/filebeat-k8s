package control

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func RenderPolicy(p Policy) (string, error) {
	if err := ValidatePolicy(p); err != nil {
		return "", err
	}

	input := mappingNode()
	addMapEntry(input, "type", stringNode("filestream", false))
	addMapEntry(input, "id", stringNode("fbctl-"+SafeName(p.ID), true))
	addMapEntry(input, "enabled", boolNode(true))

	pathValue, symlinks := RenderedPath(p)
	addMapEntry(input, "paths", sequenceNode(stringNode(pathValue, true)))
	if symlinks {
		addMapEntry(input, "prospector.scanner.symlinks", boolNode(true))
	}
	if p.LogType == LogTypeContainerStdio {
		parser := mappingNode()
		addMapEntry(parser, "container", mappingNode())
		addMapEntry(input, "parsers", sequenceNode(parser))
	}

	fields := renderedFields(p)
	if len(fields) > 0 {
		fieldsNode := mappingNode()
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			addMapEntry(fieldsNode, key, stringNode(fields[key], true))
		}

		addFields := mappingNode()
		addMapEntry(addFields, "target", stringNode("", true))
		addMapEntry(addFields, "fields", fieldsNode)

		processor := mappingNode()
		addMapEntry(processor, "add_fields", addFields)
		addMapEntry(input, "processors", sequenceNode(processor))
	}

	inputKeys := make([]string, 0, len(p.InputConfig))
	for key := range p.InputConfig {
		inputKeys = append(inputKeys, key)
	}
	sort.Strings(inputKeys)
	for _, rawKey := range inputKeys {
		key := strings.TrimSpace(rawKey)
		if _, reserved := reservedInputConfigKeys[key]; reserved {
			return "", fmt.Errorf("input_config cannot override reserved field %q", key)
		}
		node, err := yamlNodeFromValue(p.InputConfig[rawKey])
		if err != nil {
			return "", fmt.Errorf("invalid input_config.%s: %w", key, err)
		}
		addMapEntry(input, key, node)
	}

	root := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Tag:     "!!seq",
		Content: []*yaml.Node{input},
	}
	out, err := yaml.Marshal(root)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func RenderedPath(p Policy) (string, bool) {
	switch p.LogType {
	case LogTypeContainerStdio:
		return fmt.Sprintf("/var/log/klog-stdio/%s/%s/%s/*/containers/%s/*.log",
			SafePathSegment(p.Namespace),
			SafePathSegment(p.ControllerType),
			SafePathSegment(p.ControllerName),
			SafePathSegment(p.ContainerName),
		), true
	case LogTypeContainerFile:
		cleanLogPath := strings.TrimPrefix(p.LogPath, "/")
		return fmt.Sprintf("/var/log/klog/%s/%s/%s/*/containers/%s/%s",
			SafePathSegment(p.Namespace),
			SafePathSegment(p.ControllerType),
			SafePathSegment(p.ControllerName),
			SafePathSegment(p.ContainerName),
			cleanLogPath,
		), true
	default:
		return p.LogPath, false
	}
}

func renderedFields(p Policy) map[string]string {
	fields := map[string]string{}
	for key, value := range p.CustomFields {
		fields[key] = value
	}
	fields["cluster_id"] = p.ClusterID
	fields["log_type"] = p.LogType
	if p.Namespace != "" {
		fields["kubernetes.namespace"] = p.Namespace
	}
	if p.ControllerType != "" {
		fields["kubernetes.controller.type"] = p.ControllerType
	}
	if p.ControllerName != "" {
		fields["kubernetes.controller.name"] = p.ControllerName
	}
	if p.ContainerName != "" {
		fields["kubernetes.container.name"] = p.ContainerName
	}
	return fields
}

func mappingNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func sequenceNode(items ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items}
}

func addMapEntry(node *yaml.Node, key string, value *yaml.Node) {
	node.Content = append(node.Content, keyNode(key), value)
}

func keyNode(value string) *yaml.Node {
	node := stringNode(value, false)
	if strings.ContainsAny(value, ".:- ") {
		node.Style = yaml.DoubleQuotedStyle
	}
	return node
}

func stringNode(value string, quoted bool) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if quoted {
		node.Style = yaml.DoubleQuotedStyle
	}
	return node
}

func boolNode(value bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(value)}
}

func intNode(value int64) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(value, 10)}
}

func floatNode(value float64) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(value, 'f', -1, 64)}
}

func nullNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
}

func yamlNodeFromValue(value any) (*yaml.Node, error) {
	switch typed := value.(type) {
	case nil:
		return nullNode(), nil
	case string:
		return stringNode(typed, false), nil
	case bool:
		return boolNode(typed), nil
	case int:
		return intNode(int64(typed)), nil
	case int64:
		return intNode(typed), nil
	case float64:
		return floatNode(typed), nil
	case []any:
		items := make([]*yaml.Node, 0, len(typed))
		for _, item := range typed {
			node, err := yamlNodeFromValue(item)
			if err != nil {
				return nil, err
			}
			items = append(items, node)
		}
		return sequenceNode(items...), nil
	case []string:
		items := make([]*yaml.Node, 0, len(typed))
		for _, item := range typed {
			items = append(items, stringNode(item, false))
		}
		return sequenceNode(items...), nil
	case map[string]any:
		return yamlNodeFromMap(typed)
	case map[string]string:
		values := make(map[string]any, len(typed))
		for key, item := range typed {
			values[key] = item
		}
		return yamlNodeFromMap(values)
	default:
		var node yaml.Node
		if err := node.Encode(value); err != nil {
			return nil, err
		}
		return &node, nil
	}
}

func yamlNodeFromMap(values map[string]any) (*yaml.Node, error) {
	node := mappingNode()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		valueNode, err := yamlNodeFromValue(values[key])
		if err != nil {
			return nil, err
		}
		addMapEntry(node, key, valueNode)
	}
	return node, nil
}
