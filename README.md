# mini-sb-agent

mini-sb-agent is a lightweight sing-box node agent for fixed-user / small NAT VPS deployments.

## Protocol scope

The default build intentionally keeps the runtime protocol surface small:

- VLESS Reality/Vision inbound
- Hysteria2 inbound
- direct/block/dns outbound support for common node routing and DNS interception rules
- TUN inbound only when built with `-tags tun`

General-purpose proxy protocols such as VMess, Trojan, Shadowsocks, SOCKS, HTTP, and mixed inbound are intentionally not registered in the default node build.

## VLESS Reality/Vision users

Panel VLESS users are installed into sing-box with:

```text
flow = xtls-rprx-vision
```

This matches the intended VLESS Reality/Vision deployment mode. If a future deployment needs VLESS WebSocket or gRPC, make the VLESS flow configurable before using this agent for that node type.

## Hysteria2 password mapping

For lightweight dual-protocol nodes, one panel user is allowed to authenticate on both VLESS Reality/Vision and Hysteria2.

If the panel user endpoint returns `id`, `uuid`, and speed limits but does not return a Hysteria2 password, mini-sb-agent uses the user's UUID as the Hysteria2 password. It also uses the numeric panel user ID as the Hysteria2 user name when no name is provided.

Operationally this means:

- a user's VLESS UUID is also their HY2 password by default;
- VLESS and HY2 traffic can be billed back to the same numeric panel user ID;
- administrators should treat enabling both inbounds on a node as dual-protocol access for the same user set.

## Hysteria2 bandwidth and per-user limits

Hysteria2/QUIC should use sing-box's native Hysteria2 bandwidth controls for the node-wide Brutal bandwidth advertisement instead of sleeping in the UDP packet path. mini-sb-agent exposes optional CLI overrides for Hysteria2 inbounds:

```text
-hy2-up-mbps <mbps>
-hy2-down-mbps <mbps>
-hy2-ignore-client-bandwidth
```

Use these values for the node target capacity, not for an individual user's panel speed limit. For example, a node that should provide up to 500 Mbps overall should advertise `-hy2-up-mbps 500 -hy2-down-mbps 500`, while a panel user with `speed_limit=200` is still expected to be capped near 200 Mbps by mini-sb-agent's per-user limiter.

Operational milestone from the Germany LXC test node:

- Before handing node-wide HY2 bandwidth control back to sing-box/Hysteria2, Speedtest download stalls were certain or high-probability on HY2.
- With sing-box-layer HY2 bandwidth overrides and client bandwidth ignored, HY2 download stalls dropped to low-probability/occasional during repeated tests.
- The observed upload tail burst above the tested cap became rare. In that test round the active user had `speed_limit=200`, so it was not a 300 Mbps node-limit validation.
- The feature goal is HY2 parity/completeness for the agent: node-wide target capacity such as 500 Mbps should be advertised through HY2, while panel per-user limits remain enforced separately.
