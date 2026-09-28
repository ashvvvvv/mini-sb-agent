# 源码审查报告

审查日期：2026-09-28。仓库：https://github.com/ashvvvvv/mini-sb-agent 。分支：master。提交：2b57186b2d510d524dc139e0a2f5303cfb5afaec。

本次重点检查客户端用户同步、限速、流量上报、退出流程，以及内置面板鉴权和记账。sing-box fork 仅检查相关调用路径与补丁说明，不代表对整个上游内核的完整安全审计。未修改业务源码、提交或推送代码。

## 发现

### 1. [P1] 拓扑同步回调把节点独立用户重新合并到所有入站

位置：cmd/mini-sb-agent/main.go:477–482；topology_panel.go:39–44。

topologyPanel.FetchUsers 先通过 ApplyBoxByInbound 按节点更新用户，然后返回合并列表。Syncer.syncOnce 接着调用通用 Users 回调，后者执行 ApplyBox，把合并列表再次应用到所有入站。节点 A 专属用户因此可以被加入节点 B，失去节点访问隔离；下一轮还会先删除再添加这些跨节点用户。

复现：两个节点分别只有用户 1 和用户 2，执行 FetchUsers 和 main 中相同的回调顺序后，节点 21 的快照从 1 个用户变成 2 个。复现使用占位入站验证用户快照与调用顺序，未进行真实代理认证测试。

建议：拓扑模式仅执行分节点应用，后续回调只处理统计清理；共享列表 ApplyBox 仅用于旧模式。

### 2. [P1] 多节点上报部分成功后重试导致重复扣量

位置：cmd/mini-sb-agent/topology_panel.go:47–53；panelapi/sync.go:78–82。panelapi/multi.go 有同类问题。

PushTraffic 顺序上报多个节点；A 成功、B 失败时整个调用返回错误，Syncer 不提交任何计数。下一轮会再次上报 A 已确认的流量。若 B 持续失败，A 会持续重复入账。

复现：A 有 100 字节，B 首次返回 500；恢复 B 后重试同一份增量，A 合计收到 200 字节。

建议：按节点维护快照和确认进度，成功后只提交该节点计数，重试仅覆盖失败节点。网络断开导致服务端是否入账不确定的场景，仍需面板支持稳定批次 ID 的幂等协议。

### 3. [P2] 取消限速使已有连接持有的限速器永久关闭

位置：cmd/mini-sb-agent/user_manager.go:200–217；main.go:68–72。

用户从正数限速改为 0 时，updateLimiterLocked 调用 Close 并删除限速器。已有连接仍引用旧对象，其后续读写返回 ErrLimiterClosed，违反改限速不断连的行为。反向从 0 改为正数也存在问题：原来没有限速包装的已有连接不会自动获得新限速器。

复现：将限速从 1 Mbps 改为 0 后，旧限速器 Wait(1) 返回 rate limiter closed。反向路径是静态调用链结论。

建议：为活跃用户保留稳定的限速器引用，通过 SetRate(0) 禁用限速，仅在删除用户时关闭；初始不限速连接也应能观察后续更新。

### 4. [P1] 内置面板允许 agent 给未分配节点写入流量

位置：panelserver/store.go:116–139。

ApplyTraffic 只检查 agent 存在以及字段格式，不检查 item.NodeID 是否属于该 agent 的拓扑。持有任意有效 agent token 的调用方可以给其他节点、任意正数用户 ID 写入流量，污染其他节点的账目。此问题适用于内置 mini-sb-panel，并非对外部 Xboard 服务端的判断。

复现：agent 仅分配节点 21，提交 node_id=other-agents-node 的批次仍被接受并记账。

建议：在写入前验证全部条目的节点归属，明确节点的完整身份和可报告用户范围，拒绝未授权条目。

### 5. [P2] 面板拒绝的批次仍然部分入账，重试继续累加

位置：panelserver/store.go:129–139。

ApplyTraffic 在同一循环中边校验边修改 usage。若第一条有效、第二条上传量为负数，方法返回错误，但第一条已入账，批次 ID 又未标记成功。因此客户端收到拒绝仍产生账目变更，重复提交同一坏批次会反复累加有效前缀。

复现：第一条上传 100，第二条上传 -1，调用报错后第一条 usage 仍为 100。

建议：先验证整个批次和授权，再在锁内一次性提交计数及幂等记录，确保拒绝不产生副作用。

### 6. [P2] 主进程未等待退出时的最后一次流量上报

位置：cmd/mini-sb-agent/main.go:486–493；panelapi/sync.go:27–31。

收到 SIGTERM 后 main 与 Syncer 同时被唤醒。Syncer 尝试用独立上下文进行最后一次上报，但 main 只等待 b.Close 随即返回，没有等待同步 goroutine。只要上报比关闭内核慢，进程就会终止上报；定时周期以来的未确认流量可能丢失。此项为静态调用链确认，未进行 Linux 信号退出端到端复现。

建议：先停止接入并完成数据路径关闭，然后做最终快照和上报；用完成信号等待最终上报结束，并设置明确超时。

## 验证与边界

- Windows amd64，Go 1.26.4；项目声明 Go 1.25。
- 原有 go test ./... 全部通过，包含 agent、counter、panelapi、panelserver；panel 命令没有测试文件。
- go vet ./... 通过。
- 新增 5 个临时最小复现，均暴露预期缺陷；这些失败是验证发现，不是原有测试失败。
- Docker Linux 引擎未运行，未运行 Linux 专属测试、真实 VLESS/HY2 流量测试和六构建矩阵；默认跳过的运行时内存测试不计为已验证。
- 根模块 go test ./... 不覆盖嵌套的 fork/sing-box 模块全部测试。

复现测试已保存为 agent_repro_test.go.txt 和 panel_repro_test.go.txt，不加入正常测试集。重现时分别复制到 cmd/mini-sb-agent/review_temp_test.go 和 panelserver/review_temp_test.go，运行：

```powershell
go test ./panelserver ./cmd/mini-sb-agent -run '^TestReview(TopologySyncIsolation|PartialTrafficRetry|RemovingSpeedLimitKeepsConnectionUsable|RejectedTrafficIsAtomic|TrafficNodeOwnership)$' -count=1
```

实际关键输出：

```text
node 21 now has 2 users; expected only its own user
node 21 billed 200 bytes for 100 bytes
existing connection fails after removing limit: rate limiter closed
agent can book traffic to a node outside its assigned topology
rejected batch still booked 100 bytes
```
