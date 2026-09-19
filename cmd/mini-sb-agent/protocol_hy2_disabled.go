//go:build minimal && !with_hysteria2

package main

import "github.com/sagernet/sing-box/adapter/inbound"

const hysteria2BuildEnabled = false

func registerHysteria2Inbound(registry *inbound.Registry) {}
