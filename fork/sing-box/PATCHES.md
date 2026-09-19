# sing-box fork 补丁说明

本目录是 `github.com/wyx2685/sing-box_mod v1.13.0-alpha.5.0.20251202212447-8d054dcd8bfe`
的本地裁剪副本，通过仓库根 `go.mod` 的 `replace` 指向。目的：把 agent 二进制里
永远不会执行、但会拉入链接和触碰内存页的代码路径剪掉，压低 RSS。

基线上游：wyx2685/sing-box_mod（其本身 fork 自 SagerNet/sing-box）。
同步上游时请按本文件逐条重放补丁。

## 补丁清单

> **必须按序应用**：补丁 8（adapter HTTPStartContext 桩化）依赖补丁 7
> （远程规则集 fetch 删除）先行——否则 `HTTPClient()` 仍有调用方。

### 1. debug_http.go — 调试 HTTP 服务器桩化
`applyDebugListenOption` 改为空实现。原版在 `box.New` 里无条件可达，把
`net/http/pprof`、`go-chi/chi`、`http.Server` 全部钉进二进制。
agent 从不配置 debug listen。**效果：pprof(45 符号) + chi(57 符号) 死亡。**

### 2. transport/v2ray/ — v2ray 传输分发器桩化
`transport.go` 的 `NewServerTransport/NewClientTransport` 只保留空 transport
（返回 nil,nil，与原版一致），其余类型报错；删除 `grpc.go`、`quic.go`，
`grpc_lite.go` 改为报错桩。原版分发把 http/websocket/httpupgrade/quic/grpc-lite
全部拉入，其中 grpc-lite 钉死 `golang.org/x/net/http2`。
agent 生成的节点配置从不设置 transport。**效果：全部 v2ray 传输变体死亡。**
注意：`transport/v2rayhttp`、`v2raywebsocket`、`v2rayhttpupgrade`、
`v2raygrpclite`、`v2raygrpc`、`v2rayquic` 等兄弟包**保留在源码树中**
（桩化的分发器不再 import 它们，故不链接进二进制）——重放时无需删除，
删了也不影响构建产物。

### 3. protocol/hysteria2/inbound.go — masquerade 桩化
masquerade 分支（file/proxy/string 三种）改为直接报错。原版把
`http.FileServer`、`httputil.ReverseProxy` 钉进二进制。agent 生成的 hy2
配置从不设置 masquerade（nil handler 传给 service，行为不变）。

### 4. common/tls/reality_client.go — 客户端 fallback 桩化
`realityClientFallback`（reality 客户端校验失败后的伪装浏览行为）改为直接
关连接。它是 `http2.Transport` + `http.Client` 在 common/tls 包内的锚点之一。
agent 是服务端，从不走 reality 客户端。

### 5. common/tls/utls_client.go — 常量内联
`http2.NextProtoTLS` 替换为字面量 `"h2"`，去掉对 http2 包的 import。

### 6. dns/transport/ — 删除 DoH/DoT-QUIC/DHCP/fakeip 传输
删除 `https.go`、`https_transport.go`、`quic/`、`dhcp/`、`fakeip/`。
保留 `udp.go`、`tcp.go`、`tls.go`（DoT，local 传输在 Linux 依赖它）、`local/`、`hosts/`。
DoH 把 `net/http` 客户端机件和 `cookiejar` 钉进包级 import。
**注意：配置里使用 DoH/HTTP3 DNS 服务器的用户会启动失败（fail-fast）。**

### 7. route/rule/rule_set_remote.go — 远程规则集快速失败（2026-09-18 二轮）
`StartContext` 直接报错 "remote rule-sets are not compiled into this build"，
`PostStart` 变 no-op，删除 `fetch/updateOnce/loopUpdate` 及其 net/http、
crypto/tls、ntp 等 import。原版 `fetch` 用 `http.Client`（含
`ForceAttemptHTTP2`，是 http2 的客户端锚点之一）。
**注意：配置里使用 `type: remote` 规则集会启动失败（fail-fast）。**

### 8. adapter/router.go — HTTPStartContext 桩化（2026-09-18 二轮）
`HTTPStartContext` 去掉 `httpClientCache` 字段与 `HTTPClient()` 方法
（唯一调用方是补丁 7 已删除的 fetch），`Close()` 变 no-op，移除
net/http、crypto/tls、ntp 等 import。类型保留以满足 Router 接口签名。

### 9. transport/simple-obfs/http.go — 手写伪装请求（2026-09-18 二轮）
`HTTPObfs.Write` 的首包伪装请求从 `http.NewRequest` 改为手写
HTTP/1.1 文本。拔掉 simple-obfs 的 net/http 锚点。
**注意：SS 出站 `plugin: obfs-http` 仍可用，仅实现方式改变。**

独立审计（2026-09-18，子智能体全量 diff + 字节级实测）补充两个边界：
- 首包载荷 >4KB 时写调用分段不同（上游按 4096 拆两次写，本版一次写），
  但字节流相同，TCP 按 MSS 分段后线上无差异
- 首包载荷为空时本版会多发 `Content-Length: 0`（上游省略）——该路径
  不可达：SS 出站首包必含 salt+目标地址，永远非空
  （sing-shadowsocks2 各 method 族的 writeRequest 保证）

## 已知保留（无法再砍的原因）

- `golang.org/x/net/http2`（~705 符号）：被 `sing-mux` 的 H2 mux（vless/hy2
  连接复用，功能必需）和 `quic-go/qpack`（HTTP/3 头压缩表，hy2 必需）锚定。
- `net/http` 残余（~984 符号，二轮后从 1723 降下来）：被 hy2 的 HTTP/3
  伪装响应锚定——hysteria2 协议规范要求对未认证的 HTTP/3 请求作答
  （`sing-quic` 默认 `http.NotFoundHandler` + `quic-go/http3` 服务器）。
  fork sing-quic 无收益：伪装路径反正保住 net/http 的类型与消息处理机件。

## 二轮效果（2026-09-18）

- 二进制：26.16MB（一轮）→ 25.56MB；net/http 符号 1584 → 984
- Docker 生产同构实测：空闲 19.3MB（持平）、满载峰值 24.2MB（持平）、
  负载后恢复 22.4MB（一轮为 23.7，改善 1.3MB）
- 吞吐两轮采样 1240/2181MB，在既知 ±20% 环境噪声带内，补丁不触数据路径

## 验证

- `go test ./...` 全绿（除既有的 Windows /proc 平台用例）
- 六构建矩阵编译通过
- Docker 生产同构 4 节点（vless/hy2 × direct/SS）真实流量压测：
  四路径 200、吞吐与 CPU 效率不低于未打补丁基线
