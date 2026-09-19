//go:build minimal && !with_vless

package main

import "github.com/sagernet/sing-box/adapter/inbound"

const vlessBuildEnabled = false

func registerVLESSInbound(registry *inbound.Registry) {}
