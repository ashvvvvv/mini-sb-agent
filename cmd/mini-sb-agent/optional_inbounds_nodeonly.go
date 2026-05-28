//go:build !tun

package main

import "github.com/sagernet/sing-box/adapter/inbound"

func registerOptionalInbounds(registry *inbound.Registry) {}
