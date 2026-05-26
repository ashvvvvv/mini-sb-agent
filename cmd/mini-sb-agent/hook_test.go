package main

import (
	"testing"

	"mini-sb-agent/panelapi"
)

func TestHookResolvesUserAliasesToNumericID(t *testing.T) {
	um := NewUserManager(0)
	if err := um.ApplyBox(nil, []panelapi.User{{ID: 7, UUID: "uuid-7", Password: "pw-7", Name: "name-7"}}); err != nil {
		t.Fatal(err)
	}
	h := &Hook{users: um}
	for _, in := range []string{"uuid-7", "pw-7", "name-7"} {
		if got := h.ResolveUser(in); got != "7" {
			t.Fatalf("ResolveUser(%q)=%q, want 7", in, got)
		}
	}
	if got := h.ResolveUser("8"); got != "8" {
		t.Fatalf("numeric user should pass through, got %q", got)
	}
}
