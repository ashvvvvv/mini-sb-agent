package main

import (
	"testing"

	"mini-sb-agent/panelapi"
)

func TestUserManagerReusesLimiterOnSpeedChange(t *testing.T) {
	m := NewUserManager(0)
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 7, UUID: "uuid-7", SpeedLimit: 3}}); err != nil {
		t.Fatal(err)
	}
	_, before := m.Limiters("uuid-7")
	if userRateLimitBuildEnabled && before == nil {
		t.Fatal("expected limiter before speed change")
	}
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 7, UUID: "uuid-7", SpeedLimit: 5}}); err != nil {
		t.Fatal(err)
	}
	_, after := m.Limiters("uuid-7")
	if userRateLimitBuildEnabled {
		if after == nil {
			t.Fatal("expected limiter after speed change")
		}
		if before != after {
			t.Fatal("speed change replaced limiter; old connections would keep stale limits")
		}
		if got, want := after.Rate(), mbpsToBytes(5); got != want {
			t.Fatalf("limiter rate=%d, want %d", got, want)
		}
	}
}

func TestUserManagerClosesLimiterOnDelete(t *testing.T) {
	m := NewUserManager(0)
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 7, UUID: "uuid-7", SpeedLimit: 3}}); err != nil {
		t.Fatal(err)
	}
	_, limiter := m.Limiters("uuid-7")
	if userRateLimitBuildEnabled && limiter == nil {
		t.Fatal("expected limiter before delete")
	}
	if err := m.ApplyBox(nil, nil); err != nil {
		t.Fatal(err)
	}
	if userRateLimitBuildEnabled && !limiter.Closed() {
		t.Fatal("deleted user limiter is still open; old connections may continue")
	}
}
