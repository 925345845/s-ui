# 1S-UI

[![Release](https://img.shields.io/github/v/release/Hhz0823/1s-ui?label=release)](https://github.com/Hhz0823/1s-ui/releases/latest)
[![License](https://img.shields.io/github/license/Hhz0823/1s-ui)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8)](go.mod)
[![Vue](https://img.shields.io/badge/Vue-3-42b883)](frontend/package.json)
[![sing-box](https://img.shields.io/badge/default%20core-sing--box-1677ff)](https://github.com/SagerNet/sing-box)
[![Xray-core](https://img.shields.io/badge/optional%20core-Xray--core-28a745)](https://github.com/XTLS/Xray-core)

**Current version: [v1.5.0](https://github.com/Hhz0823/1s-ui/releases/tag/v1.5.0)**

1S-UI is a modern S-UI based proxy management panel focused on Linux server deployments, especially Ubuntu and Debian. It provides a cleaner web interface, sing-box first runtime, optional Xray-core inbound support, TLS automation, quick node creation, v2rayN compatible links, and Nezha/Komari-style multi-server agents with remote control and interactive terminal.

1S-UI 是基于 S-UI 二次开发的现代代理管理面板，目前主要维护 Linux 服务器版本，优先支持 Ubuntu 和 Debian。项目主打更清爽的 Web 页面、sing-box 默认内核、可选 Xray-core 入站、TLS 自动化、快速添加节点、v2rayN 兼容分享链接，以及类似哪吒 / Komari 的多服务器 Agent 监控与远程控制（含交互终端）。

> For learning, research, and technical communication only. Please comply with local laws and regulations.
>
> 本项目仅用于学习、研究和技术交流。请遵守所在地法律法规。

**Languages:** [简体中文](#简体中文) | [English](#english) | [日本語](#日本語) | [한국어](#한국어)

---

## Screenshots / 页面截图

The screenshots use local demo data and do not contain real server secrets.

以下截图使用本地演示数据，不包含真实服务器密钥或真实节点信息。

| Dashboard / 首页 | Inbounds / 入站管理 |
| --- | --- |
| ![Dashboard](docs/screenshots/dashboard.png) | ![Inbounds](docs/screenshots/inbounds.png) |

| Login / 登录 |
| --- |
| ![Login](docs/screenshots/login.png) |

---

## 简体中文

### 项目定位

1S-UI fork 自 [alireza0/s-ui](https://github.com/alireza0/s-ui)，继续优化了页面布局、节点创建、TLS 配置、Linux 部署、v2rayN 链接兼容、多内核运行能力和多服务器 Agent 管控。

> 当前维护策略：主要维护 Linux（Ubuntu、Debian）服务器版本。Windows 暂停维护，只保留源码和手动兼容构建；OpenWrt Lite 保留现有实验版本，暂不作为主要开发目标。

默认运行核心是 [sing-box](https://github.com/SagerNet/sing-box)。当需要 Xray 独有能力时，例如 VLESS XHTTP、Reality、部分 Xray TLS/传输设置，可以在入站级别选择 [Xray-core](https://github.com/XTLS/Xray-core)。

### 核心特性

- 入站、出站、端点、服务、DNS、路由、用户和管理员管理
- 入站级核心选择：默认 sing-box，可选 Xray-core
- **Xray 入站协议**：VLESS、VMess、Trojan、Shadowsocks、SOCKS、HTTP、Mixed、Hysteria2、Dokodemo-door、**WireGuard**
- Xray 传输支持 XHTTP、RAW、mKCP、gRPC、WebSocket、HTTPUpgrade；HTTP/2 与旧 QUIC 已按上游状态标记移除，推荐 XHTTP stream-one / HTTP3
- Xray 自检：二进制版本、配置校验、运行状态、支持/不支持协议与传输清单
- 一键添加节点，自动生成端口、标签、用户、TLS 和协议默认参数
- TLS、ACME、ECH、Reality、Pinned Peer Certificate SHA256 集中管理
- Hysteria2 / TLS 分享链接兼容 v2rayN，`pinSHA256` 按 Xray 需要输出 hex 指纹
- Shadowsocks 默认使用 `2022-blake3-aes-256-gcm` 和 256 位密码强度
- 一键中转节点：导入上游 SOCKS5，或按本机公网 IPv6 前缀批量生成 SOCKS5、HTTP、Mixed、Shadowsocks、VLESS、VMess、Trojan、Hysteria2、TUIC、Naive、AnyTLS
- 内置安全集成 [help660vip/auto-add-ipv6](https://github.com/help660vip/auto-add-ipv6)：一键生成 IPv6 出口 SOCKS5；面板以 Go 实现地址生成流程，不下载或执行上游脚本
- 中转批次自动创建独立账号、入站、出站和路由规则；SOCKS5/Mixed 导出 `IPv6:端口:账号:密码`，其它协议导出标准分享链接
- 中转批次可直接导出 BitBrowser 官方批量导入 Excel；IPv6 代理使用无方括号的导入格式
- IPv6 地址由面板内置 `ip -6 addr add` 管理，等待 DAD 就绪并在失败时回滚；不修改系统默认路由，避免影响 SSH 和面板连通性
- 首页仪表卡、运行状态、日志、备份恢复、使用量统计
- **服务器 Agent（v1.5）**
  - Agent 主动出站连接面板（HTTP 心跳 + **WebSocket 长连接**）
  - 上报 CPU / 内存 / 磁盘 / 负载 / 进程数 / 实时带宽 / 地址 / sing-box·Xray 状态
  - **远程控制**（需 WS 在线）：立即刷新、Ping、设置间隔、重启 Xray / sing-box / Agent、执行 Shell
  - **交互式终端**（PTY，类似哪吒）：浏览器直连面板桥接到远端 Shell
  - **多节点批量指令**：勾选多台可控制节点，统一下发命令并汇总结果
  - 面板登录态鉴权；Agent Token 仅创建/轮换时显示一次
- 响应式 Vue 3 + Vuetify 页面，支持顶部菜单、侧边栏、暗色模式和背景设置
- 主要维护 Linux（Ubuntu、Debian）服务器版本和 Linux Docker 镜像；Windows 暂停维护，OpenWrt Lite 保留实验版本

### 快速安装

Linux 服务器推荐使用一键脚本：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh)
```

安装指定版本：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) v1.5.0
```

默认配置通常为：

| 配置项 | 默认值 |
| --- | --- |
| 面板端口 | `2095` |
| 面板路径 | `/app/` |
| 订阅端口 | `2096` |
| 订阅路径 | `/sub/` |
| 数据目录 | `/usr/local/s-ui/db` |

### 服务器 Agent

在面板 **服务器监控** 页面添加节点，复制一次性显示的安装命令。命令会：

1. 下载对应架构的轻量 `sui-agent`
2. 先验证与面板的连通（心跳）
3. 写入权限 `0600` 的 `/etc/default/1s-ui-agent`
4. 启用 systemd 服务

Agent **主动连接面板**，不在节点上开放控制端口。默认优先建立 **WebSocket 长连接**（失败回落 HTTP 心跳）。

- **仅 HTTP 在线**：可看监控，不可远程控制
- **WebSocket 在线**：可远程控制、交互终端、批量指令

常用面板命令：

```bash
s-ui
s-ui status
s-ui log
s-ui update
```

手动运行 Agent 示例：

```bash
sui agent --panel 'https://your-domain:2095/app/' --token '<enrollment-token>'
```

### Docker

```yaml
services:
  s-ui:
    image: ghcr.io/Hhz0823/1s-ui
    container_name: s-ui
    hostname: "s-ui"
    network_mode: host
    volumes:
      - "./db:/app/db"
      - "./cert:/app/cert"
    tty: true
    restart: unless-stopped
    entrypoint: "./entrypoint.sh"
```

```bash
docker compose up -d
```

或者：

```bash
docker run -itd \
  --network host \
  -v "$PWD/db:/app/db" \
  -v "$PWD/cert:/app/cert" \
  --name s-ui \
  --restart unless-stopped \
  ghcr.io/Hhz0823/1s-ui
```

### OpenWrt Lite

OpenWrt Lite 面向路由器和低内存设备，只保留 sing-box 核心，不包含 Xray-core 运行时，以减少安装包体积和运行占用。

从 [Releases](https://github.com/Hhz0823/1s-ui/releases/latest) 下载对应架构的 `s-ui-lite_*.ipk` 后安装：

```bash
opkg install ./s-ui-lite_1.5.0-1_x86_64.ipk
/etc/init.d/s-ui-lite enable
/etc/init.d/s-ui-lite start
```

更多说明见 [docs/openwrt-lite.md](docs/openwrt-lite.md)。

### 页面功能

| 页面 | 说明 |
| --- | --- |
| 首页 | 系统仪表、运行状态、备份恢复、日志、使用量统计 |
| 入站管理 | 创建、编辑、克隆、删除入站，快速添加节点，Xray 自检 |
| 一键中转 | IPv6 地址池或上游 SOCKS5 批量创建多协议节点 |
| 用户管理 | 用户、流量、到期时间、分组、在线状态、二维码 |
| 出站管理 | 出站协议、拨号参数、TLS、传输层配置 |
| 节点管理 | WireGuard、Tailscale、Warp 等端点 |
| 服务管理 | CCM、OCM、DERP、SSMAPI |
| TLS 设置 | TLS、ACME、ECH、Reality、Pinned SHA256 |
| 基础信息 | 日志、实验项、全局 sing-box 配置 |
| 路由列表 | 路由规则、规则集、导入和规则动作 |
| DNS | DNS 服务器、DNS 规则、Fake-IP |
| **服务器监控** | Agent 列表、指标详情、远程控制、交互终端、批量指令 |
| 管理员 | 管理员账号、API Token、变更记录 |
| 设置 | 面板、订阅、网络、BBR/FQ/CAKE 等配置 |

### v1.5.0 更新摘要

- 扩展 Xray 入站协议与自检展示（含 WireGuard）
- Agent 升级为 WebSocket 控制面（类哪吒 / Komari）
- 交互式 PTY 终端与多节点批量命令
- 更丰富的节点指标（带宽、负载、进程数等）

---

## English

### Overview

1S-UI is a proxy management panel based on [S-UI](https://github.com/alireza0/s-ui). It keeps sing-box as the default runtime and adds a refined UI, quick node creation, TLS automation, v2rayN-compatible sharing links, optional Xray-core inbound support, and multi-server agents with remote control and an interactive terminal (Nezha/Komari style).

> Maintenance policy: Linux servers, especially Ubuntu and Debian, are the primary supported targets. Windows maintenance is paused; its source and manual compatibility workflow remain available. OpenWrt Lite remains experimental and is not a primary development target.

### Features

- Manage inbounds, outbounds, endpoints, services, DNS, routes, users, and administrators
- Per-inbound core selection: sing-box by default, Xray-core when needed
- **Xray inbounds**: VLESS, VMess, Trojan, Shadowsocks, SOCKS, HTTP, Mixed, Hysteria2, Dokodemo-door, **WireGuard**
- Xray transports: XHTTP, RAW, mKCP, gRPC, WebSocket, HTTPUpgrade (HTTP/2 and legacy QUIC removed upstream; prefer XHTTP)
- Xray self-check for binary, config validation, runtime status, and capability matrix
- Quick node creation with generated port, tag, user, TLS, and protocol defaults
- Centralized TLS, ACME, ECH, Reality, and pinned SHA256 management
- v2rayN-compatible Hysteria2 links, including Xray-ready hex `pinSHA256`
- One-click multi-protocol relay pools from local public IPv6 addresses or upstream SOCKS5 entries
- Safe built-in integration of [help660vip/auto-add-ipv6](https://github.com/help660vip/auto-add-ipv6) for one-click IPv6 egress SOCKS5 pools; 1S-UI implements the workflow in Go and never downloads or executes the upstream script
- Each relay item gets its own inbound, credentials, outbound, route, and protocol-specific export
- IPv6 address management waits for duplicate-address detection, rolls back failed batches, removes only panel-created addresses, and never changes the host default route
- Shadowsocks defaults to `2022-blake3-aes-256-gcm` with 256-bit password strength
- Dashboard cards, runtime status, logs, backup and restore, usage statistics
- **Server agents (v1.5)**
  - Outbound-only agent connection (HTTP heartbeat + **WebSocket stream**)
  - Metrics: CPU, memory, disk, load, processes, live bandwidth, addresses, core status
  - **Remote control** (WS required): refresh, ping, set interval, restart Xray/sing-box/agent, shell exec
  - **Interactive PTY terminal** bridged through the panel
  - **Batch commands** across selected controllable nodes
- Responsive Vue 3 + Vuetify frontend with top menu, sidebar, dark mode, and background settings
- Primary support for Linux servers (Ubuntu and Debian) and Linux Docker images; Windows maintenance is paused and OpenWrt Lite remains experimental

### Install

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh)
```

Install a specific version:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) v1.5.0
```

Common commands:

```bash
s-ui
s-ui status
s-ui log
s-ui update
```

### Server agents

Open **Server Agents** in the panel, enroll a node, and copy the one-time install command. The agent connects **outbound** to the panel (no inbound control port on the node). Prefer WebSocket for control and terminal; HTTP-only agents remain monitoring-only.

```bash
sui agent --panel 'https://your-domain:2095/app/' --token '<enrollment-token>'
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

### OpenWrt Lite

OpenWrt Lite targets routers and low-memory devices. It only ships the sing-box runtime and leaves Xray-core out to reduce package size and memory usage.

```bash
opkg install ./s-ui-lite_1.5.0-1_x86_64.ipk
/etc/init.d/s-ui-lite enable
/etc/init.d/s-ui-lite start
```

See [docs/openwrt-lite.md](docs/openwrt-lite.md) for architecture and packaging details.

### v1.5.0 highlights

- Broader Xray inbound protocol coverage and self-check output
- Agent control plane over WebSocket (Nezha/Komari style)
- Interactive PTY terminal and multi-node batch commands
- Richer node metrics (bandwidth, load, process count, and more)

---

## 日本語

### 概要

1S-UI は [S-UI](https://github.com/alireza0/s-ui) をベースにしたプロキシ管理パネルです。標準ランタイムは sing-box で、必要に応じて Xray-core 入站も利用できます。v1.5 では WebSocket 経由のサーバー Agent 監視・遠隔操作・対話型ターミナルにも対応しています。現在は Ubuntu と Debian を中心とする Linux サーバー版を主に保守しています。

> 保守方針：Linux（Ubuntu、Debian）を主要サポート対象とします。Windows の保守は一時停止し、ソースと手動互換ビルドのみを残します。OpenWrt Lite は実験版として維持します。

### 主な機能

- 入站、出站、エンドポイント、サービス、DNS、ルーティング、ユーザー、管理者を管理
- 入站ごとのコア選択：標準は sing-box、必要時に Xray-core
- Xray 入站：VLESS / VMess / Trojan / Shadowsocks / SOCKS / HTTP / Mixed / Hysteria2 / Dokodemo-door / WireGuard
- クイックノード作成：ポート、タグ、ユーザー、TLS、プロトコル既定値を自動生成
- TLS、ACME、ECH、Reality、Pinned SHA256 を集中管理
- v2rayN 互換の Hysteria2 共有リンクに対応し、Xray 用 `pinSHA256` は hex 形式で出力
- ダッシュボード、実行状態、ログ、バックアップ、使用量統計
- サーバー Agent：監視、遠隔操作、対話型端末、複数ノード一括コマンド
- Linux（Ubuntu、Debian）と Linux Docker を主にサポート。Windows は保守一時停止、OpenWrt Lite は実験版

### インストール

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh)
```

バージョン指定：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) v1.5.0
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

### OpenWrt Lite

OpenWrt Lite はルーターや低メモリ環境向けの軽量版です。sing-box のみを含み、Xray-core ランタイムは含まれません。

```bash
opkg install ./s-ui-lite_1.5.0-1_x86_64.ipk
/etc/init.d/s-ui-lite enable
/etc/init.d/s-ui-lite start
```

詳細は [docs/openwrt-lite.md](docs/openwrt-lite.md) を参照してください。

---

## 한국어

### 개요

1S-UI는 [S-UI](https://github.com/alireza0/s-ui)를 기반으로 한 프록시 관리 패널입니다. 기본 런타임은 sing-box이며, 필요한 경우 인바운드 단위로 Xray-core를 선택할 수 있습니다. v1.5부터 WebSocket 기반 서버 Agent 모니터링·원격 제어·대화형 터미널을 지원합니다. 현재 Ubuntu와 Debian 중심의 Linux 서버 버전을 주로 유지보수합니다.

> 유지보수 정책: Linux(Ubuntu, Debian)가 주요 지원 대상입니다. Windows 유지보수는 일시 중단하며 소스와 수동 호환 빌드만 유지합니다. OpenWrt Lite는 실험 버전으로 유지합니다.

### 주요 기능

- 인바운드, 아웃바운드, 엔드포인트, 서비스, DNS, 라우팅, 사용자, 관리자 관리
- 인바운드별 코어 선택: 기본 sing-box, 선택 Xray-core
- Xray 인바운드: VLESS, VMess, Trojan, Shadowsocks, SOCKS, HTTP, Mixed, Hysteria2, Dokodemo-door, WireGuard
- 빠른 노드 생성: 포트, 태그, 사용자, TLS, 프로토콜 기본값 자동 생성
- TLS, ACME, ECH, Reality, Pinned SHA256 통합 관리
- v2rayN 호환 Hysteria2 공유 링크와 Xray용 hex `pinSHA256` 출력
- 대시보드 카드, 런타임 상태, 로그, 백업/복원, 사용량 통계
- 서버 Agent: 모니터링, 원격 제어, 대화형 터미널, 다중 노드 일괄 명령
- Linux(Ubuntu, Debian)와 Linux Docker를 주로 지원하며 Windows 유지보수는 중단, OpenWrt Lite는 실험 버전으로 유지

### 설치

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh)
```

특정 버전 설치:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Hhz0823/1s-ui/main/install.sh) v1.5.0
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

### OpenWrt Lite

OpenWrt Lite는 라우터와 저메모리 장치를 위한 경량 버전입니다. 패키지 크기와 메모리 사용량을 줄이기 위해 sing-box만 포함하며 Xray-core 런타임은 포함하지 않습니다.

```bash
opkg install ./s-ui-lite_1.5.0-1_x86_64.ipk
/etc/init.d/s-ui-lite enable
/etc/init.d/s-ui-lite start
```

자세한 내용은 [docs/openwrt-lite.md](docs/openwrt-lite.md)를 참고하세요.

---

## Build From Source / 源码构建

Frontend:

```bash
cd frontend
npm install
npm run build
```

Backend:

```bash
rm -rf web/html/*
cp -R frontend/dist/* web/html/
go build -o sui main.go
```

Validation:

```bash
cd frontend && npm run build
go test ./...
```

## Runtime Paths / 运行路径

| Variable | Default | Description |
| --- | --- | --- |
| `SUI_LOG_LEVEL` | `info` | Log level |
| `SUI_DEBUG` | `false` | Debug mode |
| `SUI_DB_FOLDER` | Program `db` folder | Database directory |
| `SUI_BIN_FOLDER` | Program `bin` folder | Runtime binary directory |
| `SUI_XRAY_PATH` | `SUI_BIN_FOLDER/xray` | Xray-core binary path |
| `SUI_XRAY_CONFIG` | `SUI_BIN_FOLDER/xray.json` | Generated Xray config path |

## Directory Structure / 项目结构

```text
.
├── api/          # HTTP API, agent control bridge, terminal WS
├── agent/        # Lightweight server agent (metrics, control, PTY)
├── app/          # Application bootstrap
├── cmd/          # CLI commands, agent runner, migrations
├── config/       # Version, name, and environment config
├── core/         # sing-box / Xray-core runtime
├── database/     # Database and models
├── docs/         # Documentation and screenshots
├── frontend/     # Vue 3 + Vuetify frontend
├── service/      # Business services and agent hub
├── sub/          # Subscription generation
├── util/         # Link, subscription, and config utilities
├── web/          # Web server
└── windows/      # Windows installation scripts
```

## Security Notes / 安全建议

- Change the administrator username and password after installation
- Use non-default panel paths and ports
- Keep databases, private keys, certificate files, and API tokens private
- Agent tokens are shown only during enrollment or rotation; store the copied command securely
- Put public panels behind HTTPS reverse proxies
- **Remote control and interactive terminal require panel login and an online WebSocket agent session**
- Shell/terminal access has the same privilege level as the agent process user (often root) — limit who can log in to the panel
- Review subscription and sharing links before sending them to others

## Credits / 鸣谢

- [SagerNet/sing-box](https://github.com/SagerNet/sing-box)
- [XTLS/Xray-core](https://github.com/XTLS/Xray-core)
- [alireza0/s-ui](https://github.com/alireza0/s-ui)
- Everyone who tests, reports issues, and contributes feedback

## License / 许可证

This project is released under the [GPL-3.0](LICENSE) license.
