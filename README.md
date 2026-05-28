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
