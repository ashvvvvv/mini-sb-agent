//go:build with_vless || !minimal

package main

import (
	"testing"

	"mini-sb-agent/panelapi"

	"github.com/sagernet/sing-box/adapter"
)

// The snapshot store must persist per-inbound user lists so a later sync can
// compute authentication deltas instead of re-adding everyone (which would
// disconnect live sessions every cycle).
func TestApplyBoxByInboundStoresPerInboundSnapshots(t *testing.T) {
	manager := NewUserManager(0)
	usersA := []panelapi.User{{ID: 1, UUID: "uuid-1", Password: "uuid-1", Name: "1"}}
	usersB := []panelapi.User{{ID: 2, UUID: "uuid-2", Password: "uuid-2", Name: "2"}}
	noInbounds := map[string]adapter.Inbound{}

	// No live inbounds yet: snapshots are still recorded per tag.
	err := manager.ApplyBoxByInbound(noInbounds, map[string][]panelapi.User{
		"vless-21-in": usersA,
		"vless-22-in": usersB,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapA := manager.InboundSnapshot("vless-21-in")
	snapB := manager.InboundSnapshot("vless-22-in")
	if len(snapA) != 1 || snapA[1].UUID != "uuid-1" {
		t.Fatalf("snapshot A=%#v", snapA)
	}
	if len(snapB) != 1 || snapB[2].UUID != "uuid-2" {
		t.Fatalf("snapshot B=%#v", snapB)
	}

	// A second identical sync must be a no-op delta (no churn).
	err = manager.ApplyBoxByInbound(noInbounds, map[string][]panelapi.User{
		"vless-21-in": usersA,
		"vless-22-in": usersB,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap := manager.InboundSnapshot("vless-21-in"); len(snap) != 1 || snap[1].UUID != "uuid-1" {
		t.Fatalf("snapshot A after resync=%#v", snap)
	}

	// Removing node B's user must clear its snapshot but keep A's.
	err = manager.ApplyBoxByInbound(noInbounds, map[string][]panelapi.User{
		"vless-21-in": usersA,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manager.InboundSnapshot("vless-22-in")) != 0 {
		t.Fatal("node B snapshot should be empty after its user is removed")
	}
	if len(manager.InboundSnapshot("vless-21-in")) != 1 {
		t.Fatal("node A snapshot should survive node B user removal")
	}
}
