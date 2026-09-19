# mini-sb-agent

轻量级 sing-box 内核节点管理客户端，面向内存受限的 NAT VPS / 小内存机器。
单进程即可运行多协议、多节点拓扑。

## 特性

- **多节点拓扑**：单进程运行任意数量的 VLESS Reality / Hysteria2 入站（支持同类型多节点）
- **每节点独立出站**：direct 直出或远端 Shadowsocks，按节点自由组合
- **节点隔离**：用户、流量统计、出站按节点隔离，面板按节点入账
- **用户热更新**：面板增删用户 / 改限速不断连（delta 同步）
- **双层限速**：面板用户级限速（热更新）+ 进程级总限速，上传 / 下载方向独立
- **面板兼容**：Xboard UniProxy 协议——自动拉取节点配置与用户列表、定时上报流量
- **内存优化**：按拓扑裁剪构建 + 周期内存归还，空闲 RSS 约 19-25MB（视拓扑而定）
- **fail-fast**：能力与配置校验在建实例之前，错误启动即报；路由 final=block，未匹配流量一律阻断

## 内存与体积（实测）

同负载对比（vless + hy2 双节点、直出、面板同步开启、双方均不加内存参数）：

| 版本 | 二进制 | 空闲 RSS |
|---|---|---|
| v0.1.2（旧稳定版） | 26.9MB | 21.1MB |
| v0.2.0 全量构建 | 25.6MB | 20.2MB（-4.4%） |
| v0.2.0 裁剪变体 | 24.4MB | 19.5MB（-7.8%） |

> 注：上表二进制为本地同参数构建（保证 RSS 对比口径一致）；release 资产经
> `-s -w` 裁剪，体积见下表。v0.2.0 安装器默认内置内存参数
> （GOGC / GOMEMLIMIT / madvdontneed / 周期归还），实际占用优于上表，
> 且负载结束后 RSS 可回落。

六构建矩阵（amd64 release 资产体积）：

| 变体 | 体积 | 适用拓扑 |
|---|---|---|
| hy2-direct | 12.7MB | 仅 hy2，直出 |
| hy2-ss | 13.9MB | 仅 hy2，含 SS 出站 |
| vless-direct | 15.2MB | 仅 vless，直出 |
| vless-ss | 16.0MB | 仅 vless，含 SS 出站 |
| vless-hy2-direct | 16.8MB | 双协议，直出 |
| vless-hy2-ss（=全量） | 17.6MB | 双协议，含 SS 出站 |

安装器会读取 topology.json 自动选择最小可用构建，资产缺失时回退全量。

## 安装

### 一键交互（推荐）

```bash
curl -fsSL https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/master/install.sh | sh
```

交互流程：面板地址 → token → 单个 / 多个节点 → 面板 node ID（**协议自动探测**，
无需手动选择）→ 多节点自动生成 topology.json → 自动选构建 → 安装并启动。

### 非交互

```bash
PANEL="--panel-url https://board.example.com --panel-token 节点密钥"

# 单节点 VLESS Reality
sh install.sh $PANEL --node-mode vless --vless-node-id 1 --yes

# 单节点 HY2
sh install.sh $PANEL --node-mode hy2 --hy2-node-id 2 --yes

# 双节点（vless + hy2）
sh install.sh $PANEL --node-mode both --vless-node-id 1 --hy2-node-id 2 --yes

# 多节点拓扑（本地文件或 URL）
sh install.sh $PANEL --topology-file ./topology.json --yes
sh install.sh $PANEL --topology-url https://example.com/topology.json --yes
```

## 拓扑配置

topology.json 只声明"有哪些节点 + 每个节点的流量去哪"（完整示例见
[topology.example.json](topology.example.json)）：

```json
{
  "nodes": [
    { "node_id": "21", "node_type": "vless", "outbound": { "type": "direct" } },
    { "node_id": "22", "node_type": "vless",
      "outbound": { "type": "shadowsocks", "server": "落地IP", "server_port": 8388,
                    "method": "2022-blake3-aes-128-gcm", "password": "base64密钥" } },
    { "node_id": "23", "node_type": "hysteria", "outbound": { "type": "direct" } }
  ]
}
```

- `node_id` / `node_type`：面板上的节点 ID 与协议（vless / hysteria）；
  监听端口、Reality 私钥、用户列表均由面板自动拉取，hy2 证书自动生成
- `outbound`：`direct` 直出，或 `shadowsocks` 四件套
  （server / server_port / method / password，须与落地机 SS 服务端一致）

改出站 / 加减节点：编辑 topology.json 后 `systemctl restart mini-sb-agent`
（重启断连几秒）。面板用户列表的增删改不需要重启，每分钟自动同步。

## 限速

两层限速可叠加（实际速率取较小者），上传 / 下载方向各自独立——
下载打满不影响上传 ACK：

- **用户级**：面板用户列表的 `speed_limit`（Mbps），同步时自动应用，
  改限速不断连（热更新）。同一用户在多个节点间共享同一限速值
  （即"该用户总共这么快"，而非每节点各一份）
- **进程级**：`--node-rate-mbps N`（安装参数，0 关闭），
  限制整个进程的总带宽；多节点拓扑下为全部节点共享的总量

TCP 连接按令牌桶节流；hy2 / UDP 采用即时判定，不干扰 QUIC 拥塞控制。
用户级限速所有构建默认启用（`-tags nouserlimit` 可编译裁掉）。

## 安装后

```
/opt/mini-sb-agent/                        程序、env、topology.json、卸载脚本
/etc/systemd/system/mini-sb-agent.service
```

```bash
systemctl status mini-sb-agent
curl --unix-socket /run/mini-sb-agent/stats.sock http://x/stats    # 流量统计
curl --unix-socket /run/mini-sb-agent/stats.sock http://x/health   # 健康检查
/opt/mini-sb-agent/mini-sb-agent capabilities                       # 查看构建能力
/opt/mini-sb-agent/uninstall.sh                                    # 卸载
```

内存调优参数（GOGC / GOMEMLIMIT / GODEBUG / 周期归还）安装器已内置，
无需手动配置；如需微调，编辑 `/opt/mini-sb-agent/env` 后重启服务。

## 从源码构建

```bash
git clone https://github.com/ashvvvvv/mini-sb-agent
cd mini-sb-agent
go build -tags with_utls -o mini-sb-agent ./cmd/mini-sb-agent
```

依赖包含本地裁剪的 sing-box fork（`fork/sing-box/`，经 `go.mod` replace 引入）。
六构建矩阵的构建命令见 `scripts/build.sh`。

## 兼容性

- 面板：Xboard（UniProxy 协议）
- Go 1.25+，Linux amd64 / arm64
- 入站：VLESS Reality、Hysteria2；出站：direct、Shadowsocks

## 免责声明

本项目基于官方 sing-box 精简改良，仅供学习研究。使用本项目产生的任何
安全性与可靠性后果由使用者自行承担。
