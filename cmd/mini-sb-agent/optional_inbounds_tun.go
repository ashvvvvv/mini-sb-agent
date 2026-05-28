//go:build tun

package main

import (
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/protocol/tun"
)

func registerOptionalInbounds(registry *inbound.Registry) {
	tun.RegisterInbound(registry)
}
