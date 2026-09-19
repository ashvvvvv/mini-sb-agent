//go:build with_vless || !minimal

package main

import (
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/protocol/vless"
)

const vlessBuildEnabled = true

func registerVLESSInbound(registry *inbound.Registry) {
	vless.RegisterInbound(registry)
}
