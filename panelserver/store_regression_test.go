package panelserver

import (
	"testing"

	"mini-sb-agent/panelapi"
)

func testStoreWithTopology(t *testing.T) *Store {
	t.Helper()
	s := NewStore("")
	if err := s.PutAgent(Agent{ID: "a", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutTopology("a", panelapi.Topology{Nodes: []panelapi.NodeSpec{
		{NodeID: "21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}},
	}}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestApplyTrafficRejectsNodeOutsideAgentTopology(t *testing.T) {
	s := testStoreWithTopology(t)
	_, err := s.ApplyTraffic("a", panelapi.TrafficPushRequest{
		BatchID: "b",
		Items:   []panelapi.TrafficItem{{NodeID: "other-agents-node", NodeType: "vless", UserID: 1, Upload: 100}},
	})
	if err == nil {
		t.Fatal("agent can book traffic to a node outside its assigned topology")
	}
	if got := s.Usage("a", "vless", "other-agents-node", 1).Upload; got != 0 {
		t.Fatalf("foreign node usage = %d, want 0", got)
	}
}

func TestApplyTrafficRejectsAgentWithoutTopology(t *testing.T) {
	s := NewStore("")
	if err := s.PutAgent(Agent{ID: "a", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.ApplyTraffic("a", panelapi.TrafficPushRequest{
		BatchID: "b",
		Items:   []panelapi.TrafficItem{{NodeID: "21", NodeType: "vless", UserID: 1, Upload: 100}},
	})
	if err == nil {
		t.Fatal("agent without an assigned topology must not book traffic")
	}
}

func TestApplyTrafficRejectedBatchLeavesNoPartialSideEffects(t *testing.T) {
	s := testStoreWithTopology(t)
	payload := panelapi.TrafficPushRequest{
		BatchID: "b",
		Items: []panelapi.TrafficItem{
			{NodeID: "21", NodeType: "vless", UserID: 1, Upload: 100},
			{NodeID: "21", NodeType: "vless", UserID: 2, Upload: -1},
		},
	}
	if _, err := s.ApplyTraffic("a", payload); err == nil {
		t.Fatal("expected rejection")
	}
	if got := s.Usage("a", "vless", "21", 1).Upload; got != 0 {
		t.Fatalf("rejected batch still booked %d bytes", got)
	}
	// The rejected batch must not be recorded as processed: a corrected
	// retry with the same batch id has to apply, not be swallowed.
	applied, err := s.ApplyTraffic("a", panelapi.TrafficPushRequest{
		BatchID: "b",
		Items:   []panelapi.TrafficItem{{NodeID: "21", NodeType: "vless", UserID: 1, Upload: 100}},
	})
	if err != nil || !applied {
		t.Fatalf("retry after rejection: applied=%v err=%v", applied, err)
	}
	if got := s.Usage("a", "vless", "21", 1).Upload; got != 100 {
		t.Fatalf("usage after retry = %d, want 100", got)
	}
}

func TestApplyTrafficIsolatesAgentsWithSameNodeIdentity(t *testing.T) {
	s := NewStore("")
	for _, agentID := range []string{"a1", "a2"} {
		if err := s.PutAgent(Agent{ID: agentID, Secret: "secret-" + agentID}); err != nil {
			t.Fatal(err)
		}
		if err := s.PutTopology(agentID, panelapi.Topology{Nodes: []panelapi.NodeSpec{
			{NodeID: "21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}},
		}}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ApplyTraffic(agentID, panelapi.TrafficPushRequest{
			BatchID: "batch-" + agentID,
			Items:   []panelapi.TrafficItem{{NodeID: "21", NodeType: "vless", UserID: 7, Upload: 100}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Same (node_type, node_id, user_id) under two agents: each account sees
	// only its own traffic.
	if got := s.Usage("a1", "vless", "21", 7).Upload; got != 100 {
		t.Fatalf("agent a1 usage = %d, want 100", got)
	}
	if got := s.Usage("a2", "vless", "21", 7).Upload; got != 100 {
		t.Fatalf("agent a2 usage = %d, want 100 (accounts must not share a key)", got)
	}
}

func TestApplyTrafficSeparatesProtocolsSharingNodeID(t *testing.T) {
	s := NewStore("")
	if err := s.PutAgent(Agent{ID: "a", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutTopology("a", panelapi.Topology{Nodes: []panelapi.NodeSpec{
		{NodeID: "21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}},
		{NodeID: "21", NodeType: "hysteria", Outbound: panelapi.OutboundSpec{Type: "direct"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyTraffic("a", panelapi.TrafficPushRequest{
		BatchID: "b1",
		Items:   []panelapi.TrafficItem{{NodeID: "21", NodeType: "vless", UserID: 1, Upload: 100}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyTraffic("a", panelapi.TrafficPushRequest{
		BatchID: "b2",
		Items:   []panelapi.TrafficItem{{NodeID: "21", NodeType: "hysteria", UserID: 1, Upload: 50}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := s.Usage("a", "vless", "21", 1).Upload; got != 100 {
		t.Fatalf("vless:21 usage = %d, want 100", got)
	}
	if got := s.Usage("a", "hysteria", "21", 1).Upload; got != 50 {
		t.Fatalf("hysteria:21 usage = %d, want 50", got)
	}
}

func TestApplyTrafficRejectsMissingOrWrongNodeType(t *testing.T) {
	s := testStoreWithTopology(t)
	for name, item := range map[string]panelapi.TrafficItem{
		"missing node_type": {NodeID: "21", UserID: 1, Upload: 100},
		"wrong node_type":   {NodeID: "21", NodeType: "trojan", UserID: 1, Upload: 100},
	} {
		payload := panelapi.TrafficPushRequest{
			BatchID: "batch-" + name,
			Items: []panelapi.TrafficItem{
				{NodeID: "21", NodeType: "vless", UserID: 1, Upload: 100},
				item,
			},
		}
		if _, err := s.ApplyTraffic("a", payload); err == nil {
			t.Fatalf("%s: expected rejection", name)
		}
		if got := s.Usage("a", "vless", "21", 1).Upload; got != 0 {
			t.Fatalf("%s: rejected batch still booked %d bytes", name, got)
		}
	}
}

func TestApplyTrafficColonIdentitiesDoNotCollide(t *testing.T) {
	// (agent, node) pairs whose ":"-joined usage keys would be identical:
	// "edge" + "vless:hysteria:21" == "edge:vless" + "hysteria:21".
	s := NewStore("")
	nodes := map[string]panelapi.NodeSpec{
		"edge":       {NodeID: "hysteria:21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}},
		"edge:vless": {NodeID: "21", NodeType: "hysteria", Outbound: panelapi.OutboundSpec{Type: "direct"}},
	}
	for agentID, node := range nodes {
		if err := s.PutAgent(Agent{ID: agentID, Secret: "secret-" + agentID}); err != nil {
			t.Fatal(err)
		}
		if err := s.PutTopology(agentID, panelapi.Topology{Nodes: []panelapi.NodeSpec{node}}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ApplyTraffic(agentID, panelapi.TrafficPushRequest{
			BatchID: "batch-" + agentID,
			Items:   []panelapi.TrafficItem{{NodeID: node.NodeID, NodeType: node.NodeType, UserID: 1, Upload: 100}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.Usage("edge", "vless", "hysteria:21", 1).Upload; got != 100 {
		t.Fatalf(`agent "edge" usage = %d, want 100 (colon identity collided)`, got)
	}
	if got := s.Usage("edge:vless", "hysteria", "21", 1).Upload; got != 100 {
		t.Fatalf(`agent "edge:vless" usage = %d, want 100 (colon identity collided)`, got)
	}
}

func TestApplyTrafficColonBatchIDsDoNotCollide(t *testing.T) {
	// (agent, batch) pairs whose ":"-joined keys would be identical:
	// "a" + "b:c" == "a:b" + "c". With a joined-string key the second push
	// would be swallowed as a duplicate of the first.
	s := NewStore("")
	batches := map[string]string{"a": "b:c", "a:b": "c"}
	for agentID := range batches {
		if err := s.PutAgent(Agent{ID: agentID, Secret: "secret-" + agentID}); err != nil {
			t.Fatal(err)
		}
		if err := s.PutTopology(agentID, panelapi.Topology{Nodes: []panelapi.NodeSpec{
			{NodeID: "21", NodeType: "vless", Outbound: panelapi.OutboundSpec{Type: "direct"}},
		}}); err != nil {
			t.Fatal(err)
		}
		applied, err := s.ApplyTraffic(agentID, panelapi.TrafficPushRequest{
			BatchID: batches[agentID],
			Items:   []panelapi.TrafficItem{{NodeID: "21", NodeType: "vless", UserID: 1, Upload: 100}},
		})
		if err != nil || !applied {
			t.Fatalf("agent %q batch %q: applied=%v err=%v (colon batch identity collided)", agentID, batches[agentID], applied, err)
		}
	}
	for agentID := range batches {
		if got := s.Usage(agentID, "vless", "21", 1).Upload; got != 100 {
			t.Fatalf("agent %q usage = %d, want 100", agentID, got)
		}
	}
}
