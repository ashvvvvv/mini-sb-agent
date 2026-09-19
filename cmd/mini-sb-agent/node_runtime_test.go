package main

import (
	"testing"

	"mini-sb-agent/panelapi"
)

func TestGroupNodeUsersKeepsSameProtocolNodesIndependent(t *testing.T) {
	nodes := []panelapi.NodeSpec{
		{NodeID: "21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}},
		{NodeID: "22", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}},
	}
	users := map[string][]panelapi.User{
		nodes[0].Key(): {{ID: 1, UUID: "uuid-1"}},
		nodes[1].Key(): {{ID: 2, UUID: "uuid-2"}},
	}
	got := groupNodeUsers(nodes, users)
	if len(got[nodes[0].InboundTag()]) != 1 || got[nodes[0].InboundTag()][0].ID != 1 {
		t.Fatalf("node 21 users=%#v", got[nodes[0].InboundTag()])
	}
	if len(got[nodes[1].InboundTag()]) != 1 || got[nodes[1].InboundTag()][0].ID != 2 {
		t.Fatalf("node 22 users=%#v", got[nodes[1].InboundTag()])
	}
}
