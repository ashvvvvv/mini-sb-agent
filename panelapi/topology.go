package panelapi

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var tagUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type Topology struct {
	Nodes []NodeSpec `json:"nodes"`
}

type NodeSpec struct {
	NodeID   string       `json:"node_id"`
	NodeType string       `json:"node_type"`
	Tag      string       `json:"tag,omitempty"`
	Outbound OutboundSpec `json:"outbound"`
}

type OutboundSpec struct {
	Type       string `json:"type"`
	Server     string `json:"server,omitempty"`
	ServerPort int    `json:"server_port,omitempty"`
	Method     string `json:"method,omitempty"`
	Password   string `json:"password,omitempty"`
	Plugin     string `json:"plugin,omitempty"`
	PluginOpts string `json:"plugin_opts,omitempty"`
}

func LoadTopology(path string) (Topology, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Topology{}, err
	}
	var topology Topology
	if err := json.Unmarshal(data, &topology); err != nil {
		return Topology{}, err
	}
	if err := topology.Validate(); err != nil {
		return Topology{}, err
	}
	return topology, nil
}

func normalizeNodeType(nodeType string) string {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "vless", "vless-reality", "reality":
		return "vless"
	case "hy2", "hysteria", "hysteria2":
		return "hysteria"
	default:
		return strings.ToLower(strings.TrimSpace(nodeType))
	}
}

func safeTagPart(value string) string {
	value = tagUnsafe.ReplaceAllString(strings.TrimSpace(value), "-")
	return strings.Trim(value, "-")
}

func validateNodeID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid or unsafe node_id: %q", id)
	}
	return nil
}

func (n NodeSpec) Key() string {
	return normalizeNodeType(n.NodeType) + ":" + strings.TrimSpace(n.NodeID)
}

func (n NodeSpec) InboundTag() string {
	if tag := safeTagPart(n.Tag); tag != "" {
		return tag
	}
	protocol := normalizeNodeType(n.NodeType)
	if protocol == "hysteria" {
		protocol = "hy2"
	}
	return protocol + "-" + safeTagPart(n.NodeID) + "-in"
}

func (n NodeSpec) OutboundTag() string {
	protocol := normalizeNodeType(n.NodeType)
	if protocol == "hysteria" {
		protocol = "hy2"
	}
	return "node-" + protocol + "-" + safeTagPart(n.NodeID) + "-out"
}

func (t Topology) Validate() error {
	if len(t.Nodes) == 0 {
		return fmt.Errorf("topology contains no nodes")
	}
	seenKeys := make(map[string]struct{}, len(t.Nodes))
	seenTags := make(map[string]struct{}, len(t.Nodes))
	seenOutboundTags := make(map[string]struct{}, len(t.Nodes))
	for i, node := range t.Nodes {
		if err := validateNodeID(node.NodeID); err != nil {
			return fmt.Errorf("nodes[%d]: %w", i, err)
		}
		switch normalizeNodeType(node.NodeType) {
		case "vless", "hysteria":
		default:
			return fmt.Errorf("nodes[%d].node_type %q is unsupported", i, node.NodeType)
		}
		if _, exists := seenKeys[node.Key()]; exists {
			return fmt.Errorf("duplicate node key %q", node.Key())
		}
		seenKeys[node.Key()] = struct{}{}
		if _, exists := seenTags[node.InboundTag()]; exists {
			return fmt.Errorf("duplicate inbound tag %q", node.InboundTag())
		}
		seenTags[node.InboundTag()] = struct{}{}
		if _, exists := seenOutboundTags[node.OutboundTag()]; exists {
			return fmt.Errorf("duplicate outbound tag %q", node.OutboundTag())
		}
		seenOutboundTags[node.OutboundTag()] = struct{}{}
		switch strings.ToLower(strings.TrimSpace(node.Outbound.Type)) {
		case "direct":
		case "shadowsocks", "ss":
			if node.Outbound.Server == "" || node.Outbound.ServerPort <= 0 || node.Outbound.ServerPort > 65535 || node.Outbound.Method == "" || node.Outbound.Password == "" {
				return fmt.Errorf("nodes[%d] shadowsocks outbound requires server, valid server_port, method and password", i)
			}
		default:
			return fmt.Errorf("nodes[%d] outbound type %q is unsupported", i, node.Outbound.Type)
		}
	}
	return nil
}

func (n NodeSpec) PanelNodeType() string {
	if normalizeNodeType(n.NodeType) == "hysteria" {
		return "hysteria"
	}
	return "vless"
}

func (n NodeSpec) NumericID() (int, bool) {
	id, err := strconv.Atoi(n.NodeID)
	return id, err == nil
}
