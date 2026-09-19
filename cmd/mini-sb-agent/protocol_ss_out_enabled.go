//go:build with_shadowsocks_outbound || !minimal

package main

import (
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/protocol/shadowsocks"
)

const shadowsocksOutboundBuildEnabled = true

func registerShadowsocksOutbound(registry *outbound.Registry) {
	shadowsocks.RegisterOutbound(registry)
}
