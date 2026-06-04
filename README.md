# mini-sb-agent

`mini-sb-agent` 是一个专为 **Alpine Linux** 环境设计的轻量级 `sing-box` 节点管理客户端，适用于内存受限的 NAT / 小内存 VPS，支持通过 Xboard 面板实现多节点与多用户的管理、一键导入 Clash 等代理应用程序。

> **免责声明**：本项目基于官方 `sing-box` 精简改良。本项目不对任何安全性与可靠性负责，使用本项目即表示您默认同意此条款。

---

## 项目背景与设计初衷

在 128MB 甚至 256MB 内存的 NAT 服务器上使用传统客户端（如 V2bX）时，常会遇到以下痛点：
1. **进程开销过大**：默认打包了过多不常用协议，导致常态空载物理内存（RSS）过高。
2. **高并发测速引发 OOM**：在进行多线程测速或大并发流量吞吐时，系统底层的 TCP 缓冲区（Socket Buffer）膨胀，代理程序与 TCP 缓冲区共同作用，导致进程被内核 OOM Killer 强杀。

`mini-sb-agent` 针对上述痛点进行了极致的精简与优化，以压缩代理程序内存占用：

* **极简协议栈**：编译期剔除 VMess、Trojan、Shadowsocks 等协议，仅保留 Hysteria 2 与 VLESS Reality。
* **单进程低开销**：Hysteria 2 与 VLESS 运行于同一个进程内，常态空载物理内存（RSS）仅约 **16.9 MB**。
* **无用依赖削减**：删除精简了无用依赖库，使用二进制编译，以最大程度压缩内存占用。
* **自动 GC 限制**：默认设置 `GOMEMLIMIT=40MiB`、`GOGC=70`、限制 `GOMAXPROCS=1`，防止 Go 虚拟机激进申请内存。

---

## 核心与高级功能介绍

* **轻量级双协议**：提供基础 VLESS Reality (`xtls-rprx-vision` 流控) 与 Hysteria 2 支持。
* **流量统计**：支持单个用户流量统计与节点总体流量统计。
* **用户限速**：支持单个用户精准限速。
* **跨协议节点级总体限速**：支持 `-node-rate-mbps` 参数。该参数设置一个**全局共享的上传/下载 Token 桶**。不论是 **VLESS Reality** 还是 **Hysteria 2**，所有进出本节点的连接将**共享并竞争此节点限速额度**。当多协议用户并发跑满该节点带宽时，它们会自动在此限速器中排队，不会突破该机器的总限速。
* **用户名单热更新**：默认每 60 秒异步轮询面板更新用户及限速，无需重启进程，实现无感热变更。
* **防 ACK 饥饿的双向限速**：限速器的上传与下载通道采用完全独立的 Token 桶（令牌桶）隔离。这能防止单方向的测速或高速下载榨干 ACK 控制报文，确保极限下载负载下上传响应依旧顺畅。
* **双协议合并同步**：支持 `-panel-hy2-node-id` 参数。可在同一个客户端进程内同时对接 Xboard 的 VLESS 节点和 Hysteria 2 节点，合并拉取用户数据，完美实现双节点单进程部署。
* **本地静态回滚机制**：支持 `-users` 参数传入本地 JSON 用户映射文件。在面板宕机或离线环境下，能够自动退避至本地用户库进行限速与认证。
* **本地监控 API**：内置极简轻量级服务接口（支持 Unix Domain Socket 和 TCP），通过访问 `/stats?delta=1` 或 `/stats?reset=1` 可以高效率提取流量增量与监控快照。
* **自动证书管理**：Hysteria 2 默认生成并使用自签名证书（需在面板端开启“允许不安全”）。
* **TUN 模块可选**：默认不注册 TUN 虚拟网卡，可根据需要在编译/安装阶段手动开启。

---

## 技术细节规范 (Technical Specs)

### 1. 条件编译 TUN 设备支持 (Conditional TUN Module Build)
如果您需要激活 TUN 虚拟网卡接口以接管主机的全局 network 路由（透明网关配置），必须在编译时加入 `tun` 编译标签：

```bash
go build -tags tun -o mini-sb-agent ./cmd/mini-sb-agent
```

如果未使用该标签（默认编译），TUN 协议接口将不会被编译打包，以此排除大量的底层网络依赖，实现最高的二进制精简度和最小的内存开销。

### 2. 日志限制 (Logging)
为了避免小内存节点因为大量 `TRACE` / `DEBUG` / `INFO` 日志刷写导致 I/O 暴涨或 CPU 开销，项目默认日志级别设定为 **`warn`**。如果需要开启详细日志，可在 `config.json` 中添加：

```json
{
  "log": {
    "level": "info",
    "timestamp": true
  }
}
```

### 3. 节点带宽与总体限速参数 (Bandwidth Controls)
支持配置 Hysteria 2 与 VLESS Reality 的速率上限。代理支持以下命令行参数：

```text
-hy2-up-mbps <mbps>
-hy2-down-mbps <mbps>
-hy2-ignore-client-bandwidth
-node-rate-mbps <mbps>
```

* `-hy2-up-mbps` / `-hy2-down-mbps`：设置节点向 Hysteria 2 客户端广播的 Hysteria 原生 Brutal 拥塞控制带宽参数。
* `-node-rate-mbps`：全局节点级双向总体速率限制。**VLESS Reality 与 Hysteria 2 底层数据无差别地共享此 Token 桶流量**，对并发多协议的整站总带宽设置硬性上限，保护小鸡不被宿主机限速或封锁。
* *注：单个用户的限速仍会通过 `speed_limit` 在应用层单独被精准限制。*

### 4. Hysteria 2 密码自动映射 (Password Mapping)
`mini-sb-agent` 会自动将该用户的 **VLESS UUID 作为其 Hysteria 2 的连接密码**。用户的面板 ID 将作为其 Hysteria 2 的用户名，方便将两种协议的流量统一统计上报。

---

## 安装与部署

### 1. 面板端配置
在 Xboard 中配置节点，记录您的 **面板网址**、**通讯密钥** 和 **节点 ID**。

<details>
<summary>🖼️ <b>点击展开查看图片：Xboard 面板配置截图参考</b></summary>
<br>

* **配置参考图 1（系统配置面板）：**
<img src="https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/temp-assets-upload-branch/assets/img_1.jpg" width="600">

* **配置参考图 2（VLess 节点编辑）：**
<img src="https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/temp-assets-upload-branch/assets/img_2.jpg" width="600">

* **配置参考图 3（Hysteria 节点编辑）：**
<img src="https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/temp-assets-upload-branch/assets/img_3.jpg" width="600">

</details>

### 2. 一键安装脚本
在您的 Alpine 小鸡上运行以下命令，按提示输入配置信息即可：

```bash
curl -fsSL [https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/master/install.sh](https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/master/install.sh) | sh
```

---

## NAT 小鸡 TCP 缓冲区调参指南

在大并发连接下（如多线程测速或高速下载），爆内存的罪魁祸首通常是 **TCP 读写缓冲区 (Socket Buffer)**。按 BDP（带宽时延积）公式计算的缓冲区对于小内存机器来说过于庞大且不符合实际使用情况。建议直接将最大缓冲区限制压低，在损失极少吞吐的前提下保证机器绝对不爆内存（建议小于 5MB）。

### 永久生效配置方法

```bash
# 1. 清理 sysctl.conf 中原有的相关配置（防止冲突）
sed -i '/net.ipv4.tcp_rmem/d' /etc/sysctl.conf
sed -i '/net.ipv4.tcp_wmem/d' /etc/sysctl.conf

# 2. 写入新限制并应用（此处参数根据机器实际情况修改，一般仅需修改最大值。以下以 1.6MB 限制为例）
echo 'net.ipv4.tcp_rmem = 4096 87380 1677722' >> /etc/sysctl.conf
echo 'net.ipv4.tcp_wmem = 4096 16384 1677722' >> /etc/sysctl.conf

# 3. 重新加载内核参数使配置生效
sysctl -p
```
## 极限性能实测数据展示

### 测试环境配置
* **测试节点**：Lowsla 家，德国法兰克福（晚高峰测试，这家机器不错，没收广告费，单纯觉得不错，推荐优惠码：`AFF-346-37JKBI2I`）
* **硬件规格**：0.15 核 CPU (AMD EPYC 9655) / 256MB 内存
* **系统环境**：Alpine Linux 3.21，部署 Hysteria 2 + VLESS Reality 双协议
* **客户端网络**：安徽联通 5G 移动网络测速
* **代理程序参数**：`GOMEMLIMIT=40MiB`, `GOGC=70`, `GOMAXPROCS=1`
* **内核缓冲区设置 (限制 1.6MB)**：
  * `net.ipv4.tcp_rmem = 4096 87380 1677722`
  * `net.ipv4.tcp_wmem = 4096 16384 1677722`
* **相关链接**：[IP 质量检测报告 (NodeQuality)](https://nodequality.com/r/Y7RHwI4JtYpGbtnt5BEoUqs4HIVwPzPB) *(注：邻居行为导致 IP 信誉近期略有波动)*

---

### 常态资源占用
* **空载占用 (VmRSS)**：**17,312 KB (约 16.9 MB)**
* **评价**：内存占用显著低于主流 V2bX 与官方 Sing-Box 完整版。

### 极限并发压测 (Speedtest)
*(测试期间使用监控脚本每 2 秒高频采样数据，在小内存极其苛刻的条件下，进程保持稳定，无 OOM。)*

1. **第一波压测**
   * **TCP 连接数峰值**：537 个
   * **系统 Socket 内存 (Cgroup Sock Mem)**：189 MB (198,889,472 bytes)
   * **Cgroup 内存总量**：233 MB (244,355,072 bytes)
   * **Mini-SB RSS 占用**：稳定在 **35 MB**
   
   ![第一波测速结果](https://www.speedtest.net/result/a/11658313877.png)

2. **第二波压测**
   * **TCP 连接数峰值**：494 个
   * **系统 Socket 内存**：199 MB (209,104,896 bytes)
   * **Cgroup 内存总量**：243 MB (255,594,496 bytes)
   * **Mini-SB RSS 占用**：稳定在 **36 MB**
   
   ![第二波测速结果](https://www.speedtest.net/result/a/11658328624.png)

3. **第三波压测**
   * **TCP 连接数峰值**：461 个
   * **系统 Socket 内存**：196 MB
   * **Cgroup 内存总量**：240 MB
   * **Mini-SB RSS 占用**：稳定在 **37 MB**
   
   ![第三波测速结果](https://www.speedtest.net/result/a/11658331342.png)

### 流媒体测试 (YouTube 4K，采用 VLESS 协议)
*(注：对于流媒体播放，Hysteria 2 基于 UDP 的拥塞控制会更加流畅，但为测试底层稳定性，此处采样 VLESS 的真实数据。)*

* **整体体验**：4K 视频毫无压力，拖动进度条平均响应延迟仅 **0.3 - 0.5s**。

**网络带宽监控采样片段：**

* **第一阶段**
  * **TCP 连接数**：375 - 431 个
  * **瞬时网速 (下行 / RX)**：
    * `12:36:41`: 14.04 MB/s (约 112 Mbps)
    * `12:36:43`: 12.81 MB/s (约 102 Mbps)
    * `12:36:45`: 19.83 MB/s (约 158 Mbps)
    * `12:36:47`: 27.61 MB/s (约 220 Mbps，此次测速峰值)

* **第二阶段**
  * **TCP 连接数**：258 - 285 个
  * **瞬时网速 (下行 / RX)**：
    * `12:37:05`: 25.83 MB/s (约 206 Mbps)
    * `12:37:07`: 23.77 MB/s (约 190 Mbps)

* **第三阶段**
  * **TCP 连接数**：始终在 179 - 180 个
  * **瞬时网速 (下行 / RX)**：
    * `12:38:47`: 20.35 MB/s (约 162 Mbps)
    * `12:38:49`: 19.02 MB/s (约 152 Mbps)
    * `12:38:51`: 14.97 MB/s (约 120 Mbps)
