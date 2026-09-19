//go:build with_hysteria2 || !minimal

package main

import (
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/protocol/hysteria2"
)

const hysteria2BuildEnabled = true

func registerHysteria2Inbound(registry *inbound.Registry) {
	hysteria2.RegisterInbound(registry)
}
