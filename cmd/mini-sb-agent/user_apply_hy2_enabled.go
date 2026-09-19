//go:build with_hysteria2 || !minimal

package main

import (
	"fmt"

	"mini-sb-agent/panelapi"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/hysteria2"
)

// applyHysteria2Users applies only the authentication delta for one HY2
// inbound; unchanged users keep their live sessions. Returns handled=false
// when raw is not a Hysteria2 inbound.
func applyHysteria2Users(raw adapter.Inbound, tag string, users []panelapi.User, oldUsers map[int]panelapi.User) (bool, error) {
	in, ok := raw.(*hysteria2.Inbound)
	if !ok {
		return false, nil
	}
	removed, added := userChanges(oldUsers, users)
	if len(removed) > 0 {
		remove := make([]string, 0, len(removed))
		for _, old := range removed {
			if old.Password != "" {
				remove = append(remove, old.Password)
			}
		}
		if len(remove) > 0 {
			if err := in.DelUsers(remove); err != nil {
				return true, fmt.Errorf("delete hysteria2 users from %s: %w", tag, err)
			}
		}
	}
	if len(added) > 0 {
		desired := make([]option.Hysteria2User, 0, len(added))
		desiredIDs := make([]int, 0, len(added))
		for _, user := range added {
			if user.Password == "" {
				continue
			}
			name := user.Name
			if name == "" {
				name = user.Password
			}
			desired = append(desired, option.Hysteria2User{Name: name, Password: user.Password})
			desiredIDs = append(desiredIDs, user.ID)
		}
		if len(desired) > 0 {
			if err := in.AddUsers(desired, desiredIDs); err != nil {
				return true, fmt.Errorf("add hysteria2 users to %s: %w", tag, err)
			}
		}
	}
	return true, nil
}
