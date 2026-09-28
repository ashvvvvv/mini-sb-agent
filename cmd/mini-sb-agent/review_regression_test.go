package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"mini-sb-agent/panelapi"

	"github.com/sagernet/sing-box/adapter"
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

func TestTopologyUsersCallbackKeepsNodeUserIsolation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("node_id") == "21" {
			_, _ = w.Write([]byte(`{"users":[{"id":1,"uuid":"u1"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"users":[{"id":2,"uuid":"u2"}]}`))
		}
	}))
	defer srv.Close()

	nodes := []panelapi.NodeSpec{{NodeID: "21", NodeType: "vless"}, {NodeID: "22", NodeType: "vless"}}
	m := NewUserManager(0)
	in := map[string]adapter.Inbound{
		nodes[0].InboundTag(): nil,
		nodes[1].InboundTag(): nil,
	}
	p := newTopologyPanel(srv.URL, "", nodes, func(byNode map[string][]panelapi.User) error {
		return m.ApplyBoxByInbound(in, groupNodeUsers(nodes, byNode))
	})
	merged, err := p.FetchUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// The Syncer calls Users with the merged list right after FetchUsers; in
	// topology mode the callback must only prune stats, because re-applying
	// the merged list would put node A's users onto node B.
	usersCallback := newPanelUsersCallback(true, m, &Hook{}, func() map[string]adapter.Inbound { return in })
	if err := usersCallback(merged); err != nil {
		t.Fatal(err)
	}
	for _, node := range nodes {
		if got := len(m.InboundSnapshot(node.InboundTag())); got != 1 {
			t.Fatalf("node %s has %d users; expected only its own user", node.NodeID, got)
		}
	}
}

func TestTopologySyncerRetriesOnlyFailedNodes(t *testing.T) {
	var mu sync.Mutex
	billed := map[string]int64{}
	failedNode := "22"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		node := r.URL.Query().Get("node_id")
		data, _ := io.ReadAll(r.Body)
		var payload map[int][]int64
		if err := json.Unmarshal(data, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if node == failedNode {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		mu.Lock()
		for _, v := range payload {
			billed[node] += v[0] + v[1]
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	nodes := []panelapi.NodeSpec{{NodeID: "21", NodeType: "vless"}, {NodeID: "22", NodeType: "vless"}}
	p := newTopologyPanel(srv.URL, "", nodes, nil)
	total := map[string]map[string][2]int64{
		"vless-21-in": {"1": {100, 0}},
		"vless-22-in": {"2": {100, 0}},
	}
	committed := map[string]map[string][2]int64{}
	s := &panelapi.Syncer{
		Panel: p,
		// SnapshotDelta semantics: only uncommitted traffic is returned.
		Snapshot: func() map[string]map[string][2]int64 {
			out := map[string]map[string][2]int64{}
			for tag, users := range total {
				for user, v := range users {
					c := committed[tag][user]
					up, down := v[0]-c[0], v[1]-c[1]
					if up > 0 || down > 0 {
						if out[tag] == nil {
							out[tag] = map[string][2]int64{}
						}
						out[tag][user] = [2]int64{up, down}
					}
				}
			}
			return out
		},
		Commit: func(got map[string]map[string][2]int64) {
			for tag, users := range got {
				if committed[tag] == nil {
					committed[tag] = map[string][2]int64{}
				}
				for user, v := range users {
					c := committed[tag][user]
					committed[tag][user] = [2]int64{c[0] + v[0], c[1] + v[1]}
				}
			}
		},
	}

	// Round 1: node 22 fails; node 21 is acknowledged and committed.
	s.FinalFlush(context.Background())
	if got := committed["vless-21-in"]["1"]; got != [2]int64{100, 0} {
		t.Fatalf("acknowledged node not committed: %#v", committed)
	}

	// Round 2: node 22 recovers; only its unconfirmed traffic is re-sent.
	failedNode = ""
	s.FinalFlush(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if billed["21"] != 100 {
		t.Fatalf("node 21 billed %d bytes for 100 bytes", billed["21"])
	}
	if billed["22"] != 100 {
		t.Fatalf("node 22 billed %d bytes for 100 bytes", billed["22"])
	}
}

func TestSpeedLimitChangesKeepLiveConnectionsUsable(t *testing.T) {
	if !userRateLimitBuildEnabled {
		return
	}
	m := NewUserManager(0)
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 1, UUID: "u1", SpeedLimit: 1}}); err != nil {
		t.Fatal(err)
	}
	_, lim := m.Limiters("u1")
	if lim == nil {
		t.Fatal("expected limiter for limited user")
	}

	// Limit removed: the limiter must be disabled, not closed, so existing
	// connections holding it keep working.
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 1, UUID: "u1", SpeedLimit: 0}}); err != nil {
		t.Fatal(err)
	}
	if err := lim.Wait(1); err != nil {
		t.Fatalf("existing connection fails after removing limit: %v", err)
	}

	// Limit re-enabled: the same limiter must limit again.
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 1, UUID: "u1", SpeedLimit: 1}}); err != nil {
		t.Fatal(err)
	}
	if got := lim.Rate(); got != mbpsToBytes(1) {
		t.Fatalf("rate = %d, want %d", got, mbpsToBytes(1))
	}
}

func TestUnlimitedUserGetsLimiterObservingLaterUpdates(t *testing.T) {
	if !userRateLimitBuildEnabled {
		return
	}
	m := NewUserManager(0)
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 1, UUID: "u1", SpeedLimit: 0}}); err != nil {
		t.Fatal(err)
	}
	_, lim := m.Limiters("u1")
	if lim == nil {
		t.Fatal("expected a disabled limiter for an unlimited user so connections opened now observe later updates")
	}
	if err := lim.Wait(1024); err != nil {
		t.Fatalf("disabled limiter must pass traffic: %v", err)
	}
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 1, UUID: "u1", SpeedLimit: 2}}); err != nil {
		t.Fatal(err)
	}
	if got := lim.Rate(); got != mbpsToBytes(2) {
		t.Fatalf("rate = %d, want %d", got, mbpsToBytes(2))
	}
}
