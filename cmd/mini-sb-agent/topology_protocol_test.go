package main

import (
	"testing"

	"mini-sb-agent/panelapi"
)

func TestValidateNodeProtocolRejectsTopologyPanelMismatch(t *testing.T) {
	node := panelapi.NodeSpec{NodeID: "21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}}
	cfg := panelapi.NodeConfig{Protocol: "hysteria", Version: 2, ServerPort: 443}
	if err := validateNodeProtocol(node, cfg); err == nil {
		t.Fatal("expected topology/panel protocol mismatch error")
	}
}

func TestValidateNodeProtocolAcceptsHysteriaAliases(t *testing.T) {
	node := panelapi.NodeSpec{NodeID: "21", NodeType: "hy2", Outbound: panelapi.OutboundSpec{Type: "direct"}}
	cfg := panelapi.NodeConfig{Protocol: "hysteria2", Version: 2, ServerPort: 443}
	if err := validateNodeProtocol(node, cfg); err != nil {
		t.Fatalf("expected aliases to match: %v", err)
	}
}
