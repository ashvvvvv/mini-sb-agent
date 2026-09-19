//go:build minimal && !with_hysteria2

package main

import (
	"github.com/sagernet/sing-box/adapter"
	"mini-sb-agent/panelapi"
)

func applyHysteria2Users(raw adapter.Inbound, tag string, users []panelapi.User, oldUsers map[int]panelapi.User) (bool, error) {
	return false, nil
}
