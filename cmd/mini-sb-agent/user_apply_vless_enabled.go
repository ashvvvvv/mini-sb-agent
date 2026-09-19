//go:build with_vless || !minimal

package main

import (
	"fmt"

	"mini-sb-agent/panelapi"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/vless"
)

func vlessUserFromPanelUser(user panelapi.User) option.VLESSUser {
	return option.VLESSUser{Name: user.UUID, UUID: user.UUID, Flow: "xtls-rprx-vision"}
}

// applyVLESSUsers applies only the authentication delta for one VLESS inbound.
// Users whose credentials are unchanged (including speed-only changes) keep
// their live sessions; only added/removed/re-credentialed users are touched.
// Returns handled=false when raw is not a VLESS inbound.
func applyVLESSUsers(raw adapter.Inbound, tag string, users []panelapi.User, oldUsers map[int]panelapi.User) (bool, error) {
	in, ok := raw.(*vless.Inbound)
	if !ok {
		return false, nil
	}
	removed, added := userChanges(oldUsers, users)
	if len(removed) > 0 {
		remove := make([]string, 0, len(removed))
		for _, old := range removed {
			if old.UUID != "" {
				remove = append(remove, old.UUID)
			}
		}
		if len(remove) > 0 {
			if err := in.DelUsers(remove); err != nil {
				return true, fmt.Errorf("delete vless users from %s: %w", tag, err)
			}
		}
	}
	if len(added) > 0 {
		desired := make([]option.VLESSUser, 0, len(added))
		for _, user := range added {
			if user.UUID != "" {
				desired = append(desired, vlessUserFromPanelUser(user))
			}
		}
		if len(desired) > 0 {
			if err := in.AddUsers(desired); err != nil {
				return true, fmt.Errorf("add vless users to %s: %w", tag, err)
			}
		}
	}
	return true, nil
}
