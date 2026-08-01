# 1S-UI

[![Release](https://img.shields.io/github/v/release/Hhz0823/1s-ui?label=release)](https://github.com/Hhz0823/1s-ui/releases/latest)
[![Dependency Security](https://github.com/Hhz0823/1s-ui/actions/workflows/security.yml/badge.svg)](https://github.com/Hhz0823/1s-ui/actions/workflows/security.yml)
[![License](https://img.shields.io/github/license/Hhz0823/1s-ui)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8)](go.mod)
[![Vue](https://img.shields.io/badge/Vue-3-42b883)](frontend/package.json)
[![sing-box](https://img.shields.io/badge/default%20core-sing--box-1677ff)](https://github.com/SagerNet/sing-box)
[![Xray-core](https://img.shields.io/badge/optional%20core-Xray--core-28a745)](https://github.com/XTLS/Xray-core)

**最新 Linux 版本 Latest Linux release:** [v1.5.8](https://github.com/Hhz0823/1s-ui/releases/tag/v1.5.8)

**OpenWrt Lite:** [v1.5.7](https://github.com/Hhz0823/1s-ui/releases/tag/v1.5.7)（暂缓更新，仅 sing-box）

基于 [S-UI](https://github.com/alireza0/s-ui) 二次开发的现代代理管理面板。默认内核 **sing-box**，入站可切换 **Xray-core**；面向 Linux 服务器（Ubuntu / Debian），提供 Web 管理、订阅、TLS 自动化、一键中转，以及类似哪吒 / Komari 的多服务器 Agent 监控与远程控制。

A modern proxy panel forked from S-UI: sing-box first, optional Xray-core per inbound, Linux-focused, with multi-server agents (monitor + remote control + terminal).

> 仅用于学习、研究与技术交流。请遵守当地法律法规。  
> For learning and research only. Comply with local laws.

**语言 Languages:** [简体中文](#简体中文) · [English](#english) · [日本語](#日本語) · [한국어](#한국어)

**快速导航:** [安装](#快速安装) · [功能](#功能特性) · [一键中转](#一键中转) · [页面一览](#页面一览) · [Docker](#docker) · [安全建议](#安全建议-security)

---

## 安装选择

| 使用场景 | 建议 | 要求 |
| --- | --- | --- |
| 单机代理、小内存 VPS | **轻量 Web 面板** `--minimal` | 完整可视化 Web UI；最低目标 1 核 CPU + 512MB 内存；低于 1.5GB 默认安全模式只启动面板 |
| Xray、反向代理、多服务器 Agent | **全面服务端** `--full` | 最低 2 核 CPU + 2GB 内存 |
| OpenWrt | **Lite v1.5.7** | 仅 sing-box；本轮 Linux 更新不包含新的 IPK |

```bash
# 推荐：交互选择轻量 Web 面板或全面服务端
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh)

# 轻量 Web 面板自动安装（包含完整 Web UI）
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) v1.5.8 -y --minimal
```

> 全面服务端启用反向代理后，公网入口是 `http://服务器IP/app/` 或 `https://域名/app/`，不是公网 `IP:2095`。

## Screenshots

截图来自 v1.5.8 默认实色主题，不含账号密码或节点密钥。 Screenshots use the v1.5.8 default solid theme and contain no credentials.

| 实色首页 Dashboard | 入站与批量节点 Inbounds | 登录 Login |
| --- | --- | --- |
| ![Dashboard](docs/screenshots/dashboard.png) | ![Inbounds](docs/screenshots/inbounds.png) | ![Login](docs/screenshots/login.png) |

---

## 简体中文

### 项目说明

| 项 | 内容 |
| --- | --- |
| 上游 | [alireza0/s-ui](https://github.com/alireza0/s-ui) |
| 默认内核 | [sing-box](https://github.com/SagerNet/sing-box) |
| 可选内核 | [Xray-core](https://github.com/XTLS/Xray-core)（按入站选择） |
| 普通面板 | **最低性能目标 1 核 CPU + 512MB 内存**；低配档位仅安装/允许 sing-box，不下载或启动 Xray-core；内存 ≥1500MB 时自动启动 sing-box，低于该值默认只启 Web 面板 |
| **集群服务端**（多服务器 Agent 控制面） | **最低 2 核 CPU + 2GB 内存**（安装与 Agent 创建均硬性校验） |
| 推荐配置 | 2 核 4GB + 1GB Swap |
| 主维护平台 | Linux：Ubuntu、Debian、Docker |
| 暂停 / 实验 | Windows 暂停维护；OpenWrt Lite 实验版 |

低配基线以 Web 管理流畅和安全启动为准：入站列表按 20 条分页，后台标签页暂停轮询，首页状态每 5 秒刷新且不并发请求。只要检测为低配档位（包括 1 核 VPS），安装器就会强制使用 sing-box，并通过运行时保护阻止旧版本残留的 Xray 自动启动；512MB 机器还会检查 cgroup、可用内存、Swap 和磁盘，并默认使用安全模式避免 OOM。代理内核吞吐量仍取决于实际协议、连接数和线路。

### 功能特性

#### 面板与代理

- 入站 / 出站 / 端点 / 服务 / DNS / 路由 / 用户 / 管理员
- **双内核**：入站可选 `sing-box` 或 `xray`
- 一键添加节点（1–100 条、连续端口、标签、用户、TLS、协议默认值）
- TLS / ACME / ECH / Reality / Pinned SHA256 集中管理
- 订阅服务与分享链接（Clash / JSON / 标准 URI；v2rayN 7.23.4 实测导入）
- Shadowsocks 默认 `2022-blake3-aes-256-gcm`
- 首页仪表盘、日志、备份恢复、流量统计
- 默认使用实色界面；可选玻璃 / 清透按钮、自定义背景和模糊强度
- 服务端面板内管理 Caddy / Nginx 反向代理状态、域名与配置应用

#### Xray-core（v1.5）

| 类型 | 支持 |
| --- | --- |
| 入站协议 | VLESS、VMess、Trojan、Shadowsocks、SOCKS、HTTP、Mixed、Hysteria2、Dokodemo-door、**WireGuard** |
| 传输 | XHTTP、RAW、mKCP、gRPC、WebSocket、HTTPUpgrade |
| 已移除（上游） | HTTP/2、旧 QUIC → 请用 XHTTP stream-one / H3 |
| 自检 | 二进制版本、配置校验、运行状态、协议/传输能力列表 |

> Xray Hysteria2 请使用 Xray-core `26.7.11` 或更新版本。全面安装和 `--with-xray` 会下载当前官方版本，并在更新后重启 1S-UI 以加载新内核。

#### 一键中转

- 模式：本机公网 IPv6 池 / 上游 SOCKS5
- 协议：SOCKS、HTTP、Mixed、SS、VLESS、VMess、Trojan、Hysteria2、TUIC、Naive、AnyTLS
- 单批可创建 1–100 条；已用端口自动跳过并继续分配
- 自动创建入站、用户、出站、路由；支持 BitBrowser Excel 与纯文本导出
- IPv6 池模式由客户端连接原 VPS IPv4/域名，每条生成 IPv6 仅绑定对应出口
- IPv6 用 `ip -6 addr add` 添加，并逐个绑定源地址验证公网出口；DAD 或公网验证失败会回滚，**不改默认路由**
- VPS 必须提供已路由或已授权的 IPv6 前缀；仅在系统里添加随机 `/64` 地址无法绕过服务商的源地址过滤
- 参考 [auto-add-ipv6](https://github.com/help660vip/auto-add-ipv6) 流程，Go 内置实现，不执行远程脚本

#### 服务器 Agent（v1.5，类哪吒 / Komari）

Agent **主动出站**连面板，节点上不开放控制端口。

| 能力 | 说明 |
| --- | --- |
| 连接 | HTTP 心跳 + **WebSocket 长连接**（优先 WS） |
| 监控 | CPU、内存、磁盘、负载、进程数、实时带宽、地址、sing-box/Xray 状态 |
| 远程控制 | 刷新指标、Ping、改上报间隔、重启 Xray / sing-box / Agent、执行 Shell（需 WS） |
| 实时延迟 | 中心到 Agent 的 WebSocket RTT、平均值、P95 与丢包率，不使用本机假 `pong` 耗时 |
| 受管入站 | 远程查看、创建、编辑和删除客户端入站，并可一键创建 1–100 条节点 |
| 受管中转 | 从服务端打开客户端入站后，可远程创建 IPv6 出口或上游 SOCKS5 中转 |
| 交互终端 | 浏览器 PTY 终端，经面板桥接到远端 Shell |
| 批量控制 | 勾选多节点，统一下发指令并汇总结果 |
| 鉴权 | 面板登录 Session；Agent Token 仅创建/轮换时显示一次 |

一个 Agent 代表一台 VPS/服务器，不代表单条代理节点；一台受管客户端可以包含多条入站。仅 HTTP 在线时可监控；WebSocket 在线且检测到本地 1S-UI 控制通道时，可管理入站。远端入站通过权限为 `0600` 的 Unix Socket 交给客户端本地面板处理，不开放额外控制端口，也不直接写 SQLite。

客户端不需要单独获取 WebSocket 地址。安装受管客户端或 Agent 时，安装器会把面板 URL 和一次性 Token 写入权限为 `0600` 的 `/etc/default/1s-ui-agent`；Agent 自动将 `http(s)://面板地址/面板路径/` 转换为 `ws(s)://面板地址/面板路径/agent/v1/ws`，并使用 `Authorization: Bearer <Token>` 主动出站连接。WS 暂时断开时会回退到 `/agent/v1/heartbeat` 上报，并自动重连。

### 快速安装

安装脚本提供三种方案（也可交互选择）：

| 模式 | 参数 | 包含 | 适用 |
| --- | --- | --- | --- |
| **轻量 Web 面板** | `--minimal` / `-m` | 完整 Web UI + sing-box | 日常 / 小机器 / 只做代理面板 |
| **受管客户端** | `--managed-client` | 完整 Web UI + sing-box + Agent | 由中心管理的低配 VPS，目标 1核512MB |
| **全面服务端** | `--full` | 面板 + Xray + 反代 + Agent + 自动启内核 | 生产 / 多节点控制面（要求 ≥2核2G） |

```bash
# 交互选择：1 轻量 Web 面板 / 2 全面服务端 / 3 受管客户端
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh)

# 轻量 Web 面板自动安装（推荐；-y 未指定模式时也默认轻量模式）
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) -y --minimal

# 指定版本安装轻量 Web 面板
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) v1.5.8 -y -m

# 全面服务端 + 域名 HTTPS
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) -y --full --domain panel.example.com --email a@b.com

# 全新受管客户端；实际 URL 和 Token 可直接从“服务器监控”页面复制
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) -y --managed-client --controller https://panel.example.com/app/ --agent-token TOKEN
```

| 组件 | 轻量 Web 面板 | 受管客户端 | 全面服务端 |
| --- | --- | --- | --- |
| 面板 Web / sing-box | 是 | 是 | 是 |
| Xray-core | 仅非低配档位可选 | 仅非低配档位可选 | 是 |
| Caddy/Nginx 反代 | 可选 | 可选 | 是 |
| sui-agent 二进制 | 否 | 是并自动绑定 | 是 |
| 自动启动代理内核 | 低内存默认安全模式 | 低内存默认安全模式 | 是 |
| 最低配置 | 1 核 512MB | 1 核 512MB | ≥2 核 2G |

其它开关：`--no-xray` / `--no-proxy` / `--skip-core` / `--start-core` / `--domain`。旧命令中的 `--force` 仍兼容，但不会绕过 2核2G、OOM 或磁盘保护。

反代启用后，面板默认仅监听 `127.0.0.1:2095`，公网必须通过 80/443 访问 `/app/`：

| 安装结果 | 正确访问地址 |
| --- | --- |
| 轻量 Web 面板 / 未启用反代 | `http://服务器IP:2095/app/` |
| 全面服务端、未填写域名 | `http://服务器IP/app/` |
| 全面服务端、Caddy + 域名 | `https://你的域名/app/` |

全面服务端的公网 `IP:2095` 默认不可访问，这是避免面板绕过反代直接暴露的安全设计。请以安装结束时输出的“访问”地址为准。

安装后可在 **面板设置 → 服务端面板 → 反向代理** 查看 Caddy/Nginx 状态、修改域名并应用配置。面板只会接管由 1S-UI 生成或识别为旧版安装器生成的配置；检测到自定义站点配置时会拒绝覆盖。

交互式全面安装不再要求输入域名：首次先提供 `http://服务器IP/app/`，登录后在上述页面配置域名。自动化部署仍可使用 `--domain panel.example.com --email a@b.com` 直接启用 Caddy HTTPS。

| 配置 | 默认 |
| --- | --- |
| 面板端口 | `2095` |
| 面板路径 | `/app/` |
| 订阅端口 | `2096` |
| 订阅路径 | `/sub/` |
| 数据目录 | `/usr/local/s-ui/db` |
| 默认账号 | `admin` / `admin`（安装后请立刻修改） |

```bash
s-ui            # 管理菜单
s-ui status
s-ui log
s-ui update
```

### 安装 Agent

1. 打开面板 **服务器监控**
2. 添加节点，复制一次性安装命令
3. 在目标机器执行（会写入 `/etc/default/1s-ui-agent` 并启用 systemd）

手动示例：

```bash
sui agent --panel 'https://你的域名/app/' --token '<一次性Token>'
```

### Docker

```bash
docker run -itd \
  --network host \
  -v "$PWD/db:/app/db" \
  -v "$PWD/cert:/app/cert" \
  --name s-ui \
  --restart unless-stopped \
  ghcr.io/Hhz0823/1s-ui
```

`docker-compose` 示例：

```yaml
services:
  s-ui:
    image: ghcr.io/Hhz0823/1s-ui
    container_name: s-ui
    network_mode: host
    volumes:
      - "./db:/app/db"
      - "./cert:/app/cert"
    restart: unless-stopped
    entrypoint: "./entrypoint.sh"
```

### OpenWrt Lite（实验）

仅含 sing-box，无 Xray。OpenWrt Lite 暂停在 `v1.5.7`，从 [v1.5.7 Release](https://github.com/Hhz0823/1s-ui/releases/tag/v1.5.7) 下载 `s-ui-lite_*.ipk`：

```bash
opkg install ./s-ui-lite_1.5.7-1_x86_64.ipk
/etc/init.d/s-ui-lite enable
/etc/init.d/s-ui-lite start
```

详见 [docs/openwrt-lite.md](docs/openwrt-lite.md)。

### 页面一览

| 页面 | 说明 |
| --- | --- |
| 首页 | 仪表盘、状态、备份、日志、用量 |
| 入站管理 | 入站 CRUD、快速添加、Xray 自检 |
| 一键中转 | IPv6 / 上游 SOCKS5 批量节点 |
| 用户管理 | 流量、到期、分组、在线、二维码 |
| 出站 / 端点 / 服务 | 协议与拨号、WG/Tailscale/Warp、CCM 等 |
| TLS / DNS / 路由 / 基础 | 证书与 Reality、DNS、规则、全局配置 |
| **服务器监控** | Agent 列表、详情、控制、终端、批量 |
| 管理员 / 设置 | 账号 Token、面板与系统网络参数 |

### v1.5.8 变更

- 修复 HY2、TUIC、AnyTLS、VLESS、Trojan、VMess、Naive 分享链接在 v2rayN 的转义、TLS 钉扎和传输兼容
- Xray Hysteria2 改用官方 `users` 配置，并要求 Xray-core 26.7.11 或更新版本
- 一键创建的 sing-box VLESS / Trojan 默认使用稳定的 RAW 传输，TUIC 自动补齐 H3 ALPN
- 服务端面板增加反向代理状态与配置管理；安装器更新 Xray 后会安全重启服务加载新内核
- 继续保留 1–100 批量节点、IPv6 出口验证和低内存安装保护
- GitHub Release 提供 `amd64`、`arm64`、`armv5`、`armv6`、`armv7`、`386`、`s390x` 七种 Linux 架构包

---

## English

### Overview

| | |
| --- | --- |
| Upstream | [alireza0/s-ui](https://github.com/alireza0/s-ui) |
| Default core | [sing-box](https://github.com/SagerNet/sing-box) |
| Optional core | [Xray-core](https://github.com/XTLS/Xray-core) (per inbound) |
| Normal panel | **Performance floor: 1 vCPU + 512MB RAM**; low-resource profiles install and permit sing-box only, without downloading or starting Xray-core; minimal mode auto-starts sing-box with at least 1500MB RAM and otherwise starts panel-only safe mode |
| **Cluster control plane** (Agent hub) | **Minimum 2 vCPU + 2GB RAM** (hard-gated by installer and Agent API) |
| Recommended | 2 vCPU + 4GB RAM + 1GB Swap |
| Primary OS | Linux (Ubuntu, Debian), Docker |
| Other | Windows maintenance paused; OpenWrt Lite experimental |

### Features

**Panel:** inbounds/outbounds/endpoints/services/DNS/routes/users/admins; per-inbound core selection; 1–100 node quick add; TLS/ACME/ECH/Reality; subscriptions and v2rayN-tested links; reverse-proxy management; dashboard, logs, backup, traffic stats. The solid UI is the default, with optional custom backgrounds and glass styles.

**Xray (v1.5):** VLESS, VMess, Trojan, Shadowsocks, SOCKS, HTTP, Mixed, Hysteria2, Dokodemo-door, WireGuard; transports XHTTP/RAW/mKCP/gRPC/WS/HTTPUpgrade; self-check UI.

**Relay:** IPv6 pool or upstream SOCKS5; 1–100 nodes per batch with automatic used-port skipping; clients connect to the original VPS IPv4/hostname while each generated IPv6 is bound only to its matching egress; multi-protocol batch; BitBrowser Excel/plain-text export; DAD plus per-address public IPv6 egress validation and rollback (no default-route changes). The provider must route or authorize the IPv6 prefix; adding random local `/64` addresses cannot bypass upstream source filtering.

**Agents (v1.5):** outbound HTTP + WebSocket; metrics; real control-plane RTT, averages, P95 and loss; editable server names/public hosts; remote control and PTY terminal; managed-client inbound CRUD, 1–100 node quick-add, and IPv6/upstream relay creation with optimistic revision checks. The installer stores the panel URL and token in `/etc/default/1s-ui-agent`; the Agent derives `/agent/v1/ws` automatically and falls back to `/agent/v1/heartbeat` during reconnects. One Agent represents one server, and each managed server can contain many inbounds. Remote configuration is applied by the client's own panel through a root-only Unix socket, never by direct SQLite writes.

**Low-resource baseline:** inbound cards render 20 per page, background tabs stop polling, dashboard metrics refresh every 5 seconds without overlapping requests, and the installer retains cgroup/Swap/OOM guards. The 1 vCPU / 512MB target covers a responsive minimal Web panel; proxy throughput still depends on protocol, concurrent traffic, and network quality.

### Install

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh)
# or
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) v1.5.8
```

Defaults: panel upstream `2095` `/app/`, sub `2096` `/sub/`, DB `/usr/local/s-ui/db`, login `admin`/`admin` (change immediately). With the full reverse-proxy install, public access uses `http://server-ip/app/` or `https://your-domain/app/`; port `2095` remains localhost-only. Reverse proxy status and domain changes are available under **Panel Settings → Server Panel**.

```bash
s-ui / s-ui status / s-ui log / s-ui update
```

**Agent:** enroll under **Server Agents**, run the one-time install command on each host, or:

```bash
sui agent --panel 'https://host/app/' --token '<token>'
```

For a fresh managed server that must retain its own full Web UI, use the generated `--managed-client` command instead of installing an Agent alone.

### Docker

```bash
docker run -itd --network host \
  -v "$PWD/db:/app/db" -v "$PWD/cert:/app/cert" \
  --name s-ui --restart unless-stopped \
  ghcr.io/Hhz0823/1s-ui
```

### OpenWrt Lite

OpenWrt Lite remains at `v1.5.7` while Linux development is prioritized.

```bash
opkg install ./s-ui-lite_1.5.7-1_x86_64.ipk
/etc/init.d/s-ui-lite enable && /etc/init.d/s-ui-lite start
```

See [docs/openwrt-lite.md](docs/openwrt-lite.md).

### v1.5.8 highlights

- Fix v2rayN link escaping, TLS pinning, and transport compatibility for HY2, TUIC, AnyTLS, VLESS, Trojan, and VMess
- Generate Xray Hysteria2 with the official `users` shape and require Xray-core 26.7.11 or newer
- Use stable RAW defaults for one-click sing-box VLESS/Trojan and add the required H3 ALPN for TUIC
- Add reverse-proxy management under Server Panel and restart 1S-UI after an Xray binary update
- Preserve 1–100 batch creation, IPv6 egress validation, and low-memory install guards
- Keep the minimal panel responsive on the 1 vCPU / 512MB baseline with paginated inbounds and reduced polling

---

## 日本語

[S-UI](https://github.com/alireza0/s-ui) ベースのプロキシ管理パネル。標準は sing-box、入站ごとに Xray-core を選択可能。主に Ubuntu / Debian 向け。v1.5 では Agent による監視・遠隔操作・対話型ターミナルに対応。

Linux の最新バージョンは `v1.5.8` です。1–100 件の一括ノード作成、IPv6 出口中継、Xray 自己診断、Caddy / Nginx リバースプロキシ管理に対応します。OpenWrt Lite は sing-box のみの `v1.5.7` を継続します。

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) v1.5.8
```

Docker: `ghcr.io/Hhz0823/1s-ui`（`network_mode: host` 推奨）。詳細は上の English / 中文 を参照。

---

## 한국어

[S-UI](https://github.com/alireza0/s-ui) 기반 프록시 관리 패널. 기본 코어 sing-box, 인바운드별 Xray-core 선택. 주로 Ubuntu/Debian 지원. v1.5: Agent 모니터링·원격 제어·대화형 터미널·다중 노드 일괄 명령.

최신 Linux 버전은 `v1.5.8`입니다. 1–100개 노드 일괄 생성, IPv6 출구 릴레이, Xray 자체 검사, Caddy / Nginx 리버스 프록시 관리를 지원합니다. OpenWrt Lite는 sing-box 전용 `v1.5.7`을 유지합니다.

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) v1.5.8
```

Docker: `ghcr.io/Hhz0823/1s-ui`. 자세한 내용은 위 중문/영문 섹션 참고.

---

## 源码构建 Build

```bash
cd frontend && npm install && npm run build && cd ..
rm -rf web/html/* && cp -R frontend/dist/* web/html/
go build -o sui main.go
```

```bash
go test . ./agent ./api/... ./app/... ./cmd/... ./config/... ./core/... \
  ./cronjob/... ./database/... ./logger/... ./middleware/... ./network/... \
  ./service/... ./sub/... ./util/... ./web/...
cd frontend && npm run build
```

这里显式列出 Go 包，是为了避免前端 `node_modules` 中第三方示例 Go 文件被 `go test ./...` 误识别为项目包。

## 环境变量 Environment

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `SUI_LOG_LEVEL` | `info` | 日志级别 |
| `SUI_DEBUG` | `false` | 调试模式 |
| `SUI_DB_FOLDER` | 程序目录下 `db` | 数据库目录 |
| `SUI_BIN_FOLDER` | 程序目录下 `bin` | 运行时二进制 |
| `SUI_XRAY_PATH` | `$SUI_BIN_FOLDER/xray` | Xray 路径 |
| `SUI_XRAY_CONFIG` | `$SUI_BIN_FOLDER/xray.json` | Xray 配置路径 |
| `SUI_DISABLE_XRAY` | `false` | 设为 `true` 时禁止 Xray 启动和创建 Xray 入站；低配安装器自动设置 |

## 目录结构 Structure

```text
.
├── api/          # HTTP API、Agent 控制桥接、终端 WebSocket
├── agent/        # 服务器 Agent（指标、指令、PTY）
├── app/          # 应用启动
├── cmd/          # CLI、agent runner、迁移
├── config/       # 版本与环境配置
├── core/         # sing-box / Xray 运行时
├── database/     # SQLite 与模型
├── docs/         # 文档与截图
├── frontend/     # Vue 3 + Vuetify
├── service/      # 业务与 agent hub
├── sub/          # 订阅
├── util/         # 链接与配置工具
├── web/          # Web 服务
└── windows/      # Windows 脚本（暂停维护）
```

## 安全建议 Security

1. 安装后立即修改默认管理员密码  
2. 使用非默认面板路径与端口  
3. 保护数据库、证书、私钥、API Token  
4. Agent Token 仅创建/轮换时显示，请安全保存安装命令  
5. 公网面板务必 HTTPS  
6. **远程 Shell / 终端权限 = Agent 进程用户（常为 root）**，严格限制面板登录权限  
7. 控制与终端需要：面板已登录 + Agent WebSocket 在线  

## 鸣谢 Credits

- [SagerNet/sing-box](https://github.com/SagerNet/sing-box)
- [XTLS/Xray-core](https://github.com/XTLS/Xray-core)
- [alireza0/s-ui](https://github.com/alireza0/s-ui)
- 测试与反馈的用户们

## 许可证 License

[GPL-3.0](LICENSE)
