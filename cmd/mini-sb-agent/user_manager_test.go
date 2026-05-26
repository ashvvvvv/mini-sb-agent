package main

import (
	"testing"

	"mini-sb-agent/panelapi"
)

func TestUserManagerResolvesAliasesAndActiveIDs(t *testing.T) {
	m := NewUserManager(0)
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 7, UUID: "uuid-7", Password: "pw-7", Name: "name-7"}}); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"uuid-7", "pw-7", "name-7"} {
		if got := m.Resolve(secret); got != "7" {
			t.Fatalf("Resolve(%q)=%q, want 7", secret, got)
		}
	}
	active := m.ActiveIDs()
	if _, ok := active["7"]; !ok || len(active) != 1 {
		t.Fatalf("active ids mismatch: %#v", active)
	}
}

func TestUserManagerHotDeleteRemovesAliasesAndLimiter(t *testing.T) {
	m := NewUserManager(10)
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 7, UUID: "uuid-7", Password: "pw-7", SpeedLimit: 3}}); err != nil {
		t.Fatal(err)
	}
	_, userLimiter := m.Limiters("uuid-7")
	if userRateLimitBuildEnabled && userLimiter == nil {
		t.Fatal("expected user limiter before delete")
	}
	if err := m.ApplyBox(nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := m.Resolve("uuid-7"); got != "uuid-7" {
		t.Fatalf("deleted alias still resolves to %q", got)
	}
	if active := m.ActiveIDs(); len(active) != 0 {
		t.Fatalf("expected no active ids, got %#v", active)
	}
	_, userLimiter = m.Limiters("uuid-7")
	if userLimiter != nil {
		t.Fatal("deleted user limiter still exists")
	}
}
