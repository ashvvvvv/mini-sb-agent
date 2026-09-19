package main

import (
	"testing"

	"mini-sb-agent/panelapi"
)

func TestUserChangesAreEmptyForUnchangedInboundUsers(t *testing.T) {
	old := map[int]panelapi.User{1: {ID: 1, UUID: "uuid-1", Password: "pw-1", Name: "1", SpeedLimit: 10}}
	next := []panelapi.User{{ID: 1, UUID: "uuid-1", Password: "pw-1", Name: "1", SpeedLimit: 10}}
	removed, added := userChanges(old, next)
	if len(removed) != 0 || len(added) != 0 {
		t.Fatalf("unchanged users generated churn: removed=%#v added=%#v", removed, added)
	}
}

func TestUserChangesIgnoreSpeedOnlyChangesForAuthentication(t *testing.T) {
	old := map[int]panelapi.User{1: {ID: 1, UUID: "uuid-1", Password: "pw-1", Name: "1", SpeedLimit: 10}}
	next := []panelapi.User{{ID: 1, UUID: "uuid-1", Password: "pw-1", Name: "1", SpeedLimit: 20}}
	removed, added := userChanges(old, next)
	if len(removed) != 0 || len(added) != 0 {
		t.Fatalf("speed-only update generated authentication churn: removed=%#v added=%#v", removed, added)
	}
}

func TestUserChangesReplaceChangedCredentials(t *testing.T) {
	old := map[int]panelapi.User{1: {ID: 1, UUID: "old", Password: "old-pw", Name: "1"}}
	next := []panelapi.User{{ID: 1, UUID: "new", Password: "new-pw", Name: "1"}}
	removed, added := userChanges(old, next)
	if len(removed) != 1 || removed[0].UUID != "old" || len(added) != 1 || added[0].UUID != "new" {
		t.Fatalf("credential replacement mismatch: removed=%#v added=%#v", removed, added)
	}
}

func TestPerInboundDeltasDoNotCrossDeleteSharedPanelUser(t *testing.T) {
	manager := NewUserManager(0)
	sharedA := panelapi.User{ID: 7, UUID: "uuid-a", Password: "pw-a", Name: "7"}
	sharedB := panelapi.User{ID: 7, UUID: "uuid-b", Password: "pw-b", Name: "7"}
	manager.byInbound["vless-1-in"] = usersByID([]panelapi.User{sharedA})
	manager.byInbound["vless-2-in"] = usersByID([]panelapi.User{sharedB})

	removed, added := userChanges(manager.byInbound["vless-2-in"], []panelapi.User{sharedB})
	if len(removed) != 0 || len(added) != 0 {
		t.Fatalf("unchanged second inbound was affected by first inbound identity: removed=%#v added=%#v", removed, added)
	}
}
