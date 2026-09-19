//go:build minimal && !with_vless && test

package main

import (
	"github.com/sagernet/sing-box/option"
	"mini-sb-agent/panelapi"
)

func vlessUserFromPanelUser(user panelapi.User) option.VLESSUser {
	return option.VLESSUser{Name: user.UUID, UUID: user.UUID, Flow: "xtls-rprx-vision"}
}
