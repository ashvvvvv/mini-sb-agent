package panelapi

import (
	"encoding/json"
	"testing"
)

func TestDecodeTopologySupportsMultipleSameProtocolNodesWithIndependentOutbounds(t *testing.T) {
	data := []byte(`{
  "nodes": [
    {"node_id":"21","node_type":"vless","outbound":{"type":"shadowsocks","server":"198.51.100.10","server_port":8388,"method":"2022-blake3-aes-128-gcm","password":"pw1"}},
    {"node_id":"22","node_type":"vless","outbound":{"type":"shadowsocks","server":"198.51.100.20","server_port":8388,"method":"aes-128-gcm","password":"pw2"}},
    {"node_id":"23","node_type":"hysteria","outbound":{"type":"direct"}}
  ]
}`)
	var topology Topology
	if err := json.Unmarshal(data, &topology); err != nil {
		t.Fatal(err)
	}
	if err := topology.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(topology.Nodes) != 3 {
		t.Fatalf("nodes=%d, want 3", len(topology.Nodes))
	}
	if got := topology.Nodes[0].InboundTag(); got != "vless-21-in" {
		t.Fatalf("inbound tag=%q", got)
	}
	if got := topology.Nodes[0].OutboundTag(); got != "node-vless-21-out" {
		t.Fatalf("outbound tag=%q", got)
	}
	if topology.Nodes[0].Outbound.Server != "198.51.100.10" || topology.Nodes[1].Outbound.Server != "198.51.100.20" {
		t.Fatalf("outbounds were not kept per node: %#v", topology.Nodes)
	}
}

func TestTopologyRejectsDuplicateNodeKeys(t *testing.T) {
	topology := Topology{Nodes: []NodeSpec{
		{NodeID: "21", NodeType: "vless", Outbound: OutboundSpec{Type: "direct"}},
		{NodeID: "21", NodeType: "vless", Outbound: OutboundSpec{Type: "direct"}},
	}}
	if err := topology.Validate(); err == nil {
		t.Fatal("expected duplicate node key validation error")
	}
}

func TestTopologyRejectsUnsupportedOutbound(t *testing.T) {
	topology := Topology{Nodes: []NodeSpec{{NodeID: "21", NodeType: "vless", Outbound: OutboundSpec{Type: "socks"}}}}
	if err := topology.Validate(); err == nil {
		t.Fatal("expected unsupported outbound validation error")
	}
}
