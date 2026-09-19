package main

import (
	"testing"

	"mini-sb-agent/panelapi"
)

func TestTopologyAllowsSameNodeIDAcrossProtocols(t *testing.T) {
	// Outbound tags embed the protocol, so vless:21 and hysteria:21 must NOT
	// collide (regression: tags once collided when protocols shared an ID).
	topology := panelapi.Topology{Nodes: []panelapi.NodeSpec{
		{NodeID: "21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}},
		{NodeID: "21", NodeType: "hysteria", Outbound: panelapi.OutboundSpec{Type: "direct"}},
	}}
	if err := topology.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestShadowsocksAliasGeneratesCanonicalOutboundType(t *testing.T) {
	node := panelapi.NodeSpec{NodeID: "1", NodeType: "vless", Outbound: panelapi.OutboundSpec{
		Type: "ss", Server: "127.0.0.1", ServerPort: 8388, Method: "aes-128-gcm", Password: "pw",
	}}
	out := outboundFromNodeSpec(node)
	if got := out["type"]; got != "shadowsocks" {
		t.Fatalf("outbound type=%v, want shadowsocks", got)
	}
}
