package panelapi

import "testing"

func TestTopologyRejectsUnsafeNodeID(t *testing.T) {
	topology := Topology{Nodes: []NodeSpec{{NodeID: "../../escape", NodeType: "hysteria", Outbound: OutboundSpec{Type: "direct"}}}}
	if err := topology.Validate(); err == nil {
		t.Fatal("expected unsafe node_id validation error")
	}
}

func TestOutboundTagsIncludeProtocolToAvoidCrossProtocolCollision(t *testing.T) {
	vless := NodeSpec{NodeID: "21", NodeType: "vless"}
	hy2 := NodeSpec{NodeID: "21", NodeType: "hysteria"}
	if vless.OutboundTag() == hy2.OutboundTag() {
		t.Fatalf("cross-protocol outbound tags collide: %q", vless.OutboundTag())
	}
	if vless.OutboundTag() != "node-vless-21-out" || hy2.OutboundTag() != "node-hy2-21-out" {
		t.Fatalf("unexpected tags: %q %q", vless.OutboundTag(), hy2.OutboundTag())
	}
}

func TestTopologyAllowsSameNumericIDAcrossProtocols(t *testing.T) {
	topology := Topology{Nodes: []NodeSpec{
		{NodeID: "21", NodeType: "vless", Outbound: OutboundSpec{Type: "direct"}},
		{NodeID: "21", NodeType: "hysteria", Outbound: OutboundSpec{Type: "direct"}},
	}}
	if err := topology.Validate(); err != nil {
		t.Fatalf("same ID in separate protocol namespaces should be valid: %v", err)
	}
}
