# mini-sb-agent

`mini-sb-agent` is a lightweight `sing-box` node management client specifically tailored for **Alpine Linux** environments. It is designed for resource-constrained NAT / low-memory VPS instances, supporting multi-node and multi-user management via the Xboard panel, and one-click configuration imports for proxy apps like Clash.

> **Disclaimer**: This project is a streamlined and modified version based on the official `sing-box` core. The project owners assume no responsibility for any security or reliability issues. Using this software implies your consent to this disclaimer.

---

## Background & Core Philosophy

Running traditional node clients (e.g., V2bX) on NAT servers with only 128MB or 256MB of RAM often reveals the following pain points:
1. **Bloated Memory Overhead**: Packaging too many unused protocols by default results in a high idle physical memory footprint (RSS).
2. **OOM Risks Under High Concurrency**: During multi-threaded speed tests or high-concurrency traffic throughput, the system's underlying TCP Socket Buffers inflate. The combined memory footprint of the proxy application and the TCP buffers triggers the kernel's Out-Of-Memory (OOM) Killer, terminating the proxy process.

`mini-sb-agent` undergoes extreme simplification and tuning to minimize the proxy engine's RAM usage:

* **Minimalist Protocol Stack**: VMess, Trojan, Shadowsocks, and other protocols are stripped out at compile time, retaining only Hysteria 2 and VLESS Reality.
* **Single-Process Lightweight Setup**: Hysteria 2 and VLESS run within a single process. The idle physical memory (RSS) is reduced to only **~16.9 MB**.
* **Unused Dependency Pruning**: Bloated dependencies are pruned, and the binary is compiled specifically to maximize RAM conservation.
* **Aggressive Go GC Tuning**: Defaults to `GOMEMLIMIT=40MiB`, `GOGC=70`, and restricts `GOMAXPROCS=1` to prevent the Go runtime from aggressively requesting virtual memory.

---

## Core & Advanced Features

* **Lightweight Dual Protocols**: Built-in support for VLESS Reality (using `xtls-rprx-vision` flow control) and Hysteria 2.
* **Traffic Accounting**: Individual user traffic statistics combined with global node-wide traffic usage reporting.
* **Per-User Speed Limits**: Precise, isolated rate-limiting per user.
* **Cross-Protocol Node-Level Bandwidth Cap**: Enabled via the `-node-rate-mbps` parameter. This configures a **globally shared upload/download Token Bucket**. Regardless of whether clients connect via **VLESS Reality** or **Hysteria 2**, all traffic entering/leaving the node **shares and competes for this node-wide speed limit**. When multi-protocol users concurrently saturate the node's bandwidth, they queue up gracefully in this rate-limiter, ensuring the VPS never exceeds the overall machine bandwidth limit.
* **Hot-Reloadable User List**: Periodically queries the panel (default every 60 seconds) to update users and speed limits asynchronously without restarting the process.
* **Bi-directional Limiter (Prevents ACK Starvation)**: Rate-limiting upload and download paths use separate Token Buckets. This prevents heavy download traffic or unilateral speed tests from starving outgoing ACK packets, keeping the upstream pipeline responsive under full downstream load.
* **Dual-Node Synchronization**: Supports the `-panel-hy2-node-id` parameter. This allows the client to fetch VLESS and Hysteria 2 user lists simultaneously from the Xboard panel within a single process, enabling a perfect dual-protocol deployment on a single machine.
* **Static Local Fallback**: Supports loading a local JSON user database via the `-users` flag. In the event of panel downtime or network isolation, the agent automatically falls back to this local user directory for rate-limiting and authentication.
* **Local Metrics API**: Built-in ultra-lightweight monitoring interface (supporting Unix Domain Sockets and TCP). Querying `/stats?delta=1` or `/stats?reset=1` returns traffic increments and statistics snapshots with minimal CPU footprint.
* **Automated Certificate Management**: Generates and manages self-signed certificates for Hysteria 2 by default (requires "Allow Insecure" to be enabled on the panel).
* **Optional TUN Module**: The virtual TUN network interface is not registered by default, but it can be manually enabled during compilation or installation if needed.

---

## Technical Specifications

### 1. Conditional TUN Device Support
If you need to activate the TUN interface to manage host-level transparent gateway routing, you must build the binary with the `tun` build tag:

```bash
go build -tags tun -o mini-sb-agent ./cmd/mini-sb-agent
```

If compiled without this tag (default build), the TUN driver code is excluded, removing low-level networking dependencies to yield a smaller binary size and lower runtime memory overhead.

### 2. Logging Controls
To prevent high I/O spikes and CPU load caused by writing large volumes of `TRACE` / `DEBUG` / `INFO` logs on low-end servers, the default log level is set to **`warn`**. To override this, define the log settings in your `config.json`:

```json
{
  "log": {
    "level": "info",
    "timestamp": true
  }
}
```

### 3. Bandwidth and Node-Rate Overrides
You can customize the maximum speed limits of Hysteria 2 and VLESS Reality. The agent supports the following CLI arguments:

```text
-hy2-up-mbps <mbps>
-hy2-down-mbps <mbps>
-hy2-ignore-client-bandwidth
-node-rate-mbps <mbps>
```

* `-hy2-up-mbps` / `-hy2-down-mbps`: Configures the native Hysteria 2 Brutal congestion control parameters broadcasted to Hysteria 2 clients.
* `-node-rate-mbps`: A hard cap on global node-level bi-directional speed. **VLESS Reality and Hysteria 2 share this Token Bucket**, putting a strict limit on the overall bandwidth to prevent host-level throttling or server suspension.
* *Note: Individual user speed limits configured in the panel will still be strictly enforced on the application layer via `speed_limit`.*

### 4. Hysteria 2 Password Auto-Mapping
`mini-sb-agent` automatically maps the user's **VLESS UUID as their Hysteria 2 connection password**. The user's panel ID is mapped as the Hysteria 2 username, allowing traffic from both protocols to be accurately unified and reported under a single ID.

---

## Installation & Deployment

### 1. Panel Configuration
Configure your node in Xboard and take note of your **Panel URL**, **Communication Key (API Key)**, and **Node ID**.

<details>
<summary>🖼️ <b>Click to expand screenshots: Xboard Node Configuration Guide</b></summary>
<br>

* **Step 1: System Settings Panel**
<img src="https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/temp-assets-upload-branch/assets/img_1.jpg" width="600">

* **Step 2: Editing the VLESS Node**
<img src="https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/temp-assets-upload-branch/assets/img_2.jpg" width="600">

* **Step 3: Editing the Hysteria 2 Node**
<img src="https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/temp-assets-upload-branch/assets/img_3.jpg" width="600">

</details>

### 2. One-Click Installation Script
Execute the following command on your Alpine VPS to run the setup script and input your configurations:

```bash
curl -fsSL https://raw.githubusercontent.com/ashvvvvv/mini-sb-agent/master/install.sh | sh
```

---

## TCP Socket Buffer Tuning for Low-Memory VPS Nodes

Under massive concurrent connections (e.g., multi-threaded speed tests or high-throughput downloads), the root cause of OOM crashes is often the **TCP Socket Read/Write Buffer (Socket Buffer)**. Buffers computed automatically by BDP (Bandwidth-Delay Product) formulas can be too large for low-memory environments.

Limiting the maximum buffer size significantly ensures the proxy remains stable without OOM events, at the cost of a negligible drop in peak throughput (recommended max limit: < 5MB).

### Permanent Configuration Guide

```bash
# 1. Clean existing TCP buffer settings in sysctl.conf to avoid conflicts
sed -i '/net.ipv4.tcp_rmem/d' /etc/sysctl.conf
sed -i '/net.ipv4.tcp_wmem/d' /etc/sysctl.conf

# 2. Append new constraints (e.g., limiting the maximum buffer size to ~1.6MB)
echo 'net.ipv4.tcp_rmem = 4096 87380 1677722' >> /etc/sysctl.conf
echo 'net.ipv4.tcp_wmem = 4096 16384 1677722' >> /etc/sysctl.conf

# 3. Apply the changes immediately
sysctl -p
```

---

## Benchmark & Load Test Results

### Test Environment Specifications
* **Test Node**: Lowsla (Frankfurt, Germany). High-performance AMD VPS. *Recommended Affiliate Promo Code: `AFF-346-37JKBI2I`*
* **Specs**: 0.15 vCPU (AMD EPYC 9655) / 256MB RAM
* **OS**: Alpine Linux 3.21, running both Hysteria 2 + VLESS Reality
* **Client Network**: China Unicom 5G Mobile Network
* **Agent Settings**: `GOMEMLIMIT=40MiB`, `GOGC=70`, `GOMAXPROCS=1`
* **Sysctl TCP Buffer Configuration (1.6MB Limit)**:
  * `net.ipv4.tcp_rmem = 4096 87380 1677722`
  * `net.ipv4.tcp_wmem = 4096 16384 1677722`
* **Performance Report**: [IP Quality Report (NodeQuality)](https://nodequality.com/r/Y7RHwI4JtYpGbtnt5BEoUqs4HIVwPzPB) *(Note: Neighbor actions caused slight credit fluctuations recently)*

---

### Idle Resource Consumption
* **Idle Physical Memory (VmRSS)**: **17,312 KB (~16.9 MB)**
* **Verdict**: Significantly lower memory usage compared to mainstream V2bX or a standard full-fat sing-box client.

### Extreme Concurrency Stress Test (Speedtest)
*(During the test, a monitoring daemon sampled resource usage every 2 seconds. The agent stayed highly stable without any OOM occurrences under these extreme constraints).*

1. **Test Run #1**
   * **Peak TCP Connections**: 537
   * **System Socket Memory (Cgroup Sock Mem)**: 189 MB (198,889,472 bytes)
   * **Total Cgroup Memory**: 233 MB (244,355,072 bytes)
   * **mini-sb-agent RSS**: Stable at **35 MB**
   
   ![Test Run 1 Speedtest Results](https://www.speedtest.net/result/a/11658313877.png)

2. **Test Run #2**
   * **Peak TCP Connections**: 494
   * **System Socket Memory (Cgroup Sock Mem)**: 199 MB (209,104,896 bytes)
   * **Total Cgroup Memory**: 243 MB (255,594,496 bytes)
   * **mini-sb-agent RSS**: Stable at **36 MB**
   
   ![Test Run 2 Speedtest Results](https://www.speedtest.net/result/a/11658328624.png)

3. **Test Run #3**
   * **Peak TCP Connections**: 461
   * **System Socket Memory (Cgroup Sock Mem)**: 196 MB
   * **Total Cgroup Memory**: 240 MB
   * **mini-sb-agent RSS**: Stable at **37 MB**
   
   ![Test Run 3 Speedtest Results](https://www.speedtest.net/result/a/11658331342.png)

### Streaming Playback Performance (YouTube 4K via VLESS)
*(Note: Hysteria 2's UDP-based congestion control yields smoother playback on unstable networks, but VLESS Reality was chosen here to verify VLESS latency stability).*

* **User Experience**: 4K video runs smoothly. Seek response latency averages **0.3s - 0.5s**.

**Bandwidth Metrics Sampling (Downstream / RX):**

* **Stage 1 (Buffering Burst)**
  * **TCP Connections**: 375 - 431
  * **Instantaneous Bandwidth (Downstream / RX)**:
    * `12:36:41`: 14.04 MB/s (~112 Mbps)
    * `12:36:43`: 12.81 MB/s (~102 Mbps)
    * `12:36:45`: 19.83 MB/s (~158 Mbps)
    * `12:36:47`: 27.61 MB/s (~220 Mbps, Peak burst speed)

* **Stage 2 (Continuous Playback)**
  * **TCP Connections**: 258 - 285
  * **Instantaneous Bandwidth (Downstream / RX)**:
    * `12:37:05`: 25.83 MB/s (~206 Mbps)
    * `12:37:07`: 23.77 MB/s (~190 Mbps)

* **Stage 3 (Tail Flow)**
  * **TCP Connections**: 179 - 180
  * **Instantaneous Bandwidth (Downstream / RX)**:
    * `12:38:47`: 20.35 MB/s (~162 Mbps)
    * `12:38:49`: 19.02 MB/s (~152 Mbps)
    * `12:38:51`: 14.97 MB/s (~120 Mbps)
