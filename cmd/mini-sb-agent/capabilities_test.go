package main

import (
	"strings"
	"testing"

	"mini-sb-agent/panelapi"
)

func TestBuildCapabilitiesHaveStableBuildID(t *testing.T) {
	caps := currentBuildCapabilities()
	if caps.BuildID == "" {
		t.Fatal("build id is empty")
	}
	if caps.DirectOutbound != true {
		t.Fatal("direct outbound must always be available")
	}
	if !strings.Contains(caps.BuildID, "direct") {
		t.Fatalf("build id %q must describe direct capability", caps.BuildID)
	}
}

func TestValidateTopologyCapabilitiesRejectsMissingProtocol(t *testing.T) {
	topology := panelapi.Topology{Nodes: []panelapi.NodeSpec{{
		NodeID: "1", NodeType: "hysteria", Outbound: panelapi.OutboundSpec{Type: "direct"},
	}}}
	caps := BuildCapabilities{VLESSInbound: true, DirectOutbound: true}
	if err := validateTopologyCapabilities(topology, caps); err == nil || !strings.Contains(err.Error(), "hysteria2") {
		t.Fatalf("expected missing hysteria2 error, got %v", err)
	}
}

func TestValidateTopologyCapabilitiesRejectsMissingShadowsocks(t *testing.T) {
	topology := panelapi.Topology{Nodes: []panelapi.NodeSpec{{
		NodeID: "1", NodeType: "vless", Outbound: panelapi.OutboundSpec{
			Type: "shadowsocks", Server: "127.0.0.1", ServerPort: 8388, Method: "aes-128-gcm", Password: "pw",
		},
	}}}
	caps := BuildCapabilities{VLESSInbound: true, DirectOutbound: true}
	if err := validateTopologyCapabilities(topology, caps); err == nil || !strings.Contains(err.Error(), "shadowsocks") {
		t.Fatalf("expected missing shadowsocks error, got %v", err)
	}
}
