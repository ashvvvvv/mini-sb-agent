package main

import (
	"strings"
	"testing"

	"mini-sb-agent/panelapi"
)

func TestMergeUsersRejectsConflictingSameIDAcrossNodes(t *testing.T) {
	users := map[string][]panelapi.User{
		"vless:1": {{ID: 7, UUID: "uuid-7", SpeedLimit: 10}},
		"vless:2": {{ID: 7, UUID: "uuid-7", SpeedLimit: 20}},
	}
	if _, err := mergeUsersByNode(users); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("expected conflicting user error, got %v", err)
	}
}

func TestMergeUsersAllowsIdenticalSameIDAcrossNodes(t *testing.T) {
	user := panelapi.User{ID: 7, UUID: "uuid-7", Password: "uuid-7", Name: "7", SpeedLimit: 10}
	users := map[string][]panelapi.User{"vless:1": {user}, "vless:2": {user}}
	merged, err := mergeUsersByNode(users)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 || merged[0] != user {
		t.Fatalf("unexpected merged users: %#v", merged)
	}
}
