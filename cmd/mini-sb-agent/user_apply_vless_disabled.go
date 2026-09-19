//go:build minimal && !with_vless

package main

import (
	"github.com/sagernet/sing-box/adapter"
	"mini-sb-agent/panelapi"
)

func applyVLESSUsers(raw adapter.Inbound, tag string, users []panelapi.User, oldUsers map[int]panelapi.User) (bool, error) {
	return false, nil
}
