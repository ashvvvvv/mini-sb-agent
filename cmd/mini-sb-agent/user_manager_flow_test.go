package main

import (
	"testing"

	"mini-sb-agent/panelapi"
)

func TestVLESSUserFromPanelUserAddsVisionFlow(t *testing.T) {
	got := vlessUserFromPanelUser(panelapi.User{ID: 1, UUID: "65ef6ea4-e719-497f-9fd1-e1fab9c0384d"})
	if got.Name != "65ef6ea4-e719-497f-9fd1-e1fab9c0384d" || got.UUID != "65ef6ea4-e719-497f-9fd1-e1fab9c0384d" {
		t.Fatalf("unexpected VLESS user identity: %#v", got)
	}
	if got.Flow != "xtls-rprx-vision" {
		t.Fatalf("VLESS flow = %q, want xtls-rprx-vision", got.Flow)
	}
}
