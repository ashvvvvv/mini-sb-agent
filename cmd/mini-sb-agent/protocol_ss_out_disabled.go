//go:build minimal && !with_shadowsocks_outbound

package main

import "github.com/sagernet/sing-box/adapter/outbound"

const shadowsocksOutboundBuildEnabled = false

func registerShadowsocksOutbound(registry *outbound.Registry) {}
