#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
blue='\033[0;34m'
plain='\033[0m'

cur_dir=$(pwd)

# Decision flags (set by analyze_vps)
INSTALL_PANEL=1
INSTALL_XRAY=0               # never auto-install during panel setup (OOM risk)
INSTALL_PROXY=0               # reverse proxy in front of panel (default off)
PROXY_ENGINE=""               # caddy | nginx | ""
PROXY_DOMAIN=""
PROXY_EMAIL=""
INSTALL_MODE="fresh"          # fresh | upgrade
PROFILE="standard"            # tiny | low | standard | high
MEM_TOTAL_MB=0
MEM_AVAIL_MB=0
SWAP_MB=0
CPU_CORES=1
DISK_FREE_MB=0
FORCE_INSTALL=0
AUTO_YES=0
FORCE_XRAY=""                 # "" | 1 | 0
FORCE_PROXY=""                # "" | 1 | 0
SKIP_CORE=1                   # default: panel only, cores on-demand (safer)
START_SERVICE=1               # start systemd unit after install
REQUESTED_VERSION=""
PORT80_FREE=1
PORT443_FREE=1
# Cluster control-plane (multi-server Agent hub) recommendation only.
# Normal proxy panel can install on any size VPS.
CLUSTER_CPU_CORES=2
CLUSTER_MEM_MB=2048

# 检查 root 权限
[[ $EUID -ne 0 ]] && echo -e "${red}致命错误：${plain}请使用 root 权限运行此脚本 \n " && exit 1

usage() {
    cat <<EOF
用法: install.sh [版本号] [选项]

选项:
  -y, --yes             非交互：接受推荐方案并安装
  --with-xray           强制安装 Xray-core
  --no-xray             跳过 Xray-core（仅 sing-box）
  --with-proxy          安装反代（Caddy/Nginx，默认不装）
  --no-proxy            不安装反代
  --domain DOMAIN       反代域名（启用 HTTPS）
  --email EMAIL         ACME 邮箱（Caddy 可选）
  --start-core          安装后自动启动 sing-box 内核（默认不启动，更安全）
  --no-start            只安装文件，不 systemctl start
  --force               兼容旧参数（已无硬性拦截，可忽略）
  -h, --help            显示帮助

配置说明:
  - 普通面板：任意配置均可安装（低配会默认不启内核、不装反代等更稳妥策略）
  - 集群服务端（多服务器 Agent 控制面）：建议 ≥${CLUSTER_CPU_CORES:-2} 核 CPU + ≥${CLUSTER_MEM_MB:-2048}MB 内存

示例:
  bash install.sh
  bash install.sh v1.5.4 -y --no-xray --no-proxy
  bash install.sh -y --with-proxy --domain panel.example.com --email a@b.com
EOF
}

parse_args() {
    local args=()
    while [[ $# -gt 0 ]]; do
        case "$1" in
        -y | --yes)
            AUTO_YES=1
            shift
            ;;
        --with-xray)
            FORCE_XRAY=1
            shift
            ;;
        --no-xray)
            FORCE_XRAY=0
            shift
            ;;
        --with-proxy)
            FORCE_PROXY=1
            shift
            ;;
        --no-proxy)
            FORCE_PROXY=0
            shift
            ;;
        --domain)
            PROXY_DOMAIN="${2:-}"
            shift 2
            ;;
        --email)
            PROXY_EMAIL="${2:-}"
            shift 2
            ;;
        --start-core)
            SKIP_CORE=0
            shift
            ;;
        --no-start)
            START_SERVICE=0
            shift
            ;;
        --force)
            FORCE_INSTALL=1
            shift
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        -*)
            echo -e "${red}未知参数: $1${plain}"
            usage
            exit 1
            ;;
        *)
            args+=("$1")
            shift
            ;;
        esac
    done
    if [[ ${#args[@]} -gt 0 ]]; then
        REQUESTED_VERSION="${args[0]}"
    fi
}

# 检查系统并设置 release 变量
if [[ -f /etc/os-release ]]; then
    source /etc/os-release
    release=$ID
elif [[ -f /usr/lib/os-release ]]; then
    source /usr/lib/os-release
    release=$ID
else
    echo "检测系统失败，请联系作者！" >&2
    exit 1
fi

arch() {
    case "$(uname -m)" in
    x86_64 | x64 | amd64) echo 'amd64' ;;
    i*86 | x86) echo '386' ;;
    armv8* | armv8 | arm64 | aarch64) echo 'arm64' ;;
    armv7* | armv7 | arm) echo 'armv7' ;;
    armv6* | armv6) echo 'armv6' ;;
    armv5* | armv5) echo 'armv5' ;;
    s390x) echo 's390x' ;;
    *) echo -e "${red}不支持的 CPU 架构！${plain}" && exit 1 ;;
    esac
}

port_in_use() {
    local port="$1"
    if command -v ss >/dev/null 2>&1; then
        ss -lntu 2>/dev/null | awk '{print $5}' | grep -Eq "[:.]${port}$" && return 0
    elif command -v netstat >/dev/null 2>&1; then
        netstat -lntu 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]${port}$" && return 0
    fi
    return 1
}

analyze_vps() {
    echo -e "${blue}========== VPS 预检分析 ==========${plain}"
    echo -e "系统：${green}${PRETTY_NAME:-$release}${plain}"
    echo -e "内核：$(uname -r)"
    echo -e "架构：$(arch) ($(uname -m))"

    local mem_kb avail_kb swap_kb
    mem_kb=$(awk '/MemTotal/ {print $2}' /proc/meminfo 2>/dev/null || echo 0)
    avail_kb=$(awk '/MemAvailable/ {print $2}' /proc/meminfo 2>/dev/null || echo 0)
    swap_kb=$(awk '/SwapTotal/ {print $2}' /proc/meminfo 2>/dev/null || echo 0)
    MEM_TOTAL_MB=$((mem_kb / 1024))
    MEM_AVAIL_MB=$((avail_kb / 1024))
    SWAP_MB=$((swap_kb / 1024))
    CPU_CORES=$(nproc 2>/dev/null || grep -c ^processor /proc/cpuinfo 2>/dev/null || echo 1)
    DISK_FREE_MB=$(df -Pm /usr/local 2>/dev/null | awk 'NR==2{print $4}')
    [[ -z "$DISK_FREE_MB" ]] && DISK_FREE_MB=$(df -Pm / 2>/dev/null | awk 'NR==2{print $4}')
    [[ -z "$DISK_FREE_MB" ]] && DISK_FREE_MB=0

    echo -e "CPU：${CPU_CORES} 核"
    echo -e "内存：总计 ${MEM_TOTAL_MB}MB / 可用 ${MEM_AVAIL_MB}MB / Swap ${SWAP_MB}MB"
    echo -e "磁盘：/usr/local 可用约 ${DISK_FREE_MB}MB"

    # Profile for install strategy (not a hard install gate)
    if [[ "$MEM_TOTAL_MB" -lt 900 || ( "$MEM_TOTAL_MB" -lt 1200 && "$SWAP_MB" -eq 0 ) || "$CPU_CORES" -lt 2 ]]; then
        PROFILE="low"
    elif [[ "$MEM_TOTAL_MB" -lt 2800 || "$CPU_CORES" -lt 4 ]]; then
        PROFILE="standard"
    else
        PROFILE="high"
    fi
    echo -e "资源档位：${green}${PROFILE}${plain}"

    # Existing installation
    if [[ -x /usr/local/s-ui/sui ]] || systemctl list-unit-files 2>/dev/null | grep -q '^s-ui\.service'; then
        INSTALL_MODE="upgrade"
        echo -e "面板：${yellow}检测到已安装 s-ui，将执行升级${plain}"
    else
        INSTALL_MODE="fresh"
        echo -e "面板：${green}未检测到面板，将全新安装普通面板${plain}"
    fi

    # Port check
    for p in 2095 2096 80 443; do
        if port_in_use "$p"; then
            echo -e "端口 ${p}：${yellow}已被占用${plain}"
            [[ "$p" == "80" ]] && PORT80_FREE=0
            [[ "$p" == "443" ]] && PORT443_FREE=0
        else
            echo -e "端口 ${p}：空闲"
            [[ "$p" == "80" ]] && PORT80_FREE=1
            [[ "$p" == "443" ]] && PORT443_FREE=1
        fi
    done

    # Conflicting proxies (informational)
    for svc in x-ui 3x-ui v2ray xray sing-box nginx caddy; do
        if systemctl is-active --quiet "$svc" 2>/dev/null; then
            echo -e "运行中服务：${yellow}${svc}${plain}"
        fi
    done

    # ---- Normal panel: always installable. Cluster hub: recommend 2c/2G only. ----
    INSTALL_PANEL=1
    if [[ "$DISK_FREE_MB" -gt 0 && "$DISK_FREE_MB" -lt 300 ]]; then
        echo -e "${yellow}警告：磁盘可用约 ${DISK_FREE_MB}MB，空间偏紧，建议至少 500MB${plain}"
    fi

    if [[ "$CPU_CORES" -lt "$CLUSTER_CPU_CORES" || "$MEM_TOTAL_MB" -lt "$CLUSTER_MEM_MB" ]]; then
        echo -e "普通面板：${green}可安装（无硬性最低配置）${plain}"
        echo -e "集群服务端（多服务器 Agent 控制面）：${yellow}建议 ≥${CLUSTER_CPU_CORES} 核 / ≥${CLUSTER_MEM_MB}MB${plain}"
        echo -e "  当前 ${CPU_CORES} 核 / ${MEM_TOTAL_MB}MB — 作普通代理面板即可；若要当集群控制端，请升级 VPS"
    else
        echo -e "配置检查：${green}满足普通面板，也满足集群服务端建议（≥${CLUSTER_CPU_CORES} 核 / ≥${CLUSTER_MEM_MB}MB）${plain}"
    fi

    if [[ "$PROFILE" == "low" ]]; then
        echo -e "${yellow}低配策略：默认不启代理内核、不装反代/Xray，优先保证面板 Web 能装能跑${plain}"
    fi

    # Xray: NEVER auto-install during panel install (zip + geo files spike RAM).
    # Install later with --with-xray when the panel is already stable.
    INSTALL_XRAY=0
    local xray_reason="默认不装 Xray（安装期下载/解压易 OOM；稳定后可用 --with-xray）"
    if [[ -z "$(xray_asset)" ]]; then
        xray_reason="当前架构无自动 Xray 包"
    fi

    # Explicit overrides
    if [[ "$FORCE_XRAY" == "1" ]]; then
        if [[ "$MEM_TOTAL_MB" -lt 1500 && "$FORCE_INSTALL" -ne 1 ]]; then
            INSTALL_XRAY=0
            xray_reason="内存 <1.5G，拒绝安装期装 Xray（加 --force 可强行）"
        else
            INSTALL_XRAY=1
            xray_reason="用户指定 --with-xray"
        fi
    elif [[ "$FORCE_XRAY" == "0" ]]; then
        INSTALL_XRAY=0
        xray_reason="用户指定 --no-xray"
    fi

    # Reverse proxy: OFF by default (installing Caddy/Nginx has caused instability on some VPS).
    INSTALL_PROXY=0
    PROXY_ENGINE=""
    local proxy_reason="默认不装反代（避免安装期额外服务拖垮机器）"
    if [[ "$FORCE_PROXY" == "1" || -n "$PROXY_DOMAIN" ]]; then
        local has_nginx=0 has_caddy=0
        systemctl is-active --quiet nginx 2>/dev/null && has_nginx=1
        systemctl is-active --quiet caddy 2>/dev/null && has_caddy=1
        if [[ "$PORT80_FREE" -ne 1 && "$has_nginx" -ne 1 && "$has_caddy" -ne 1 ]]; then
            INSTALL_PROXY=0
            proxy_reason="80 端口占用且无现成反代，跳过"
        else
            INSTALL_PROXY=1
            PROXY_ENGINE="caddy"
            [[ "$PROFILE" == "standard" && "$MEM_TOTAL_MB" -lt 3000 ]] && PROXY_ENGINE="nginx"
            [[ "$has_nginx" -eq 1 ]] && PROXY_ENGINE="nginx"
            [[ "$has_caddy" -eq 1 ]] && PROXY_ENGINE="caddy"
            proxy_reason="用户要求启用反代（${PROXY_ENGINE}）"
        fi
    fi
    if [[ "$FORCE_PROXY" == "0" ]]; then
        INSTALL_PROXY=0
        PROXY_ENGINE=""
        proxy_reason="用户指定 --no-proxy"
    fi

    # Core auto-start: default SKIP to prevent OOM power-offs on small VPS.
    # High-memory hosts may enable cores with --start-core.
    if [[ "$SKIP_CORE" -eq 0 ]]; then
        :
    elif [[ "$PROFILE" == "high" && "$MEM_AVAIL_MB" -ge 1500 ]]; then
        # Still default skip unless --start-core; keep safe.
        SKIP_CORE=1
    else
        SKIP_CORE=1
    fi

    echo -e "推荐：普通面板=${green}${INSTALL_PANEL}${plain}  Xray=${green}${INSTALL_XRAY}${plain}  反代=${green}${INSTALL_PROXY}${plain}$([ "$INSTALL_PROXY" -eq 1 ] && echo "(${PROXY_ENGINE})" || true)  自动启内核=${green}$([ "$SKIP_CORE" -eq 1 ] && echo 否 || echo 是)${plain}"
    echo -e "Xray 原因：${xray_reason}"
    echo -e "反代 原因：${proxy_reason}"
    echo -e "内核策略：${yellow}默认不自动启动 sing-box/Xray，仅启动面板 Web，避免 OOM 关机${plain}"
    echo -e "${blue}=================================${plain}"

    if [[ "$AUTO_YES" -ne 1 ]]; then
        echo -e "将按以上方案安装（模式: ${INSTALL_MODE}, 档位: ${PROFILE}）。"
        read -r -p "确认继续？[Y/n]: " confirm
        if [[ "${confirm}" == "n" || "${confirm}" == "N" ]]; then
            echo "已取消。"
            exit 0
        fi
        if [[ "$INSTALL_XRAY" -eq 0 && "$FORCE_XRAY" == "" ]]; then
            read -r -p "是否安装 Xray-core 二进制？[y/N]: " xans
            if [[ "${xans}" == "y" || "${xans}" == "Y" ]]; then
                INSTALL_XRAY=1
            fi
        fi
        if [[ "$INSTALL_PROXY" -eq 0 && "$FORCE_PROXY" == "" && "$PORT80_FREE" -eq 1 ]]; then
            read -r -p "是否安装反向代理（Caddy/Nginx）？[y/N]: " pans
            if [[ "${pans}" == "y" || "${pans}" == "Y" ]]; then
                INSTALL_PROXY=1
                PROXY_ENGINE="caddy"
                [[ "$PROFILE" == "standard" && "$MEM_TOTAL_MB" -lt 3000 ]] && PROXY_ENGINE="nginx"
                read -r -p "反代域名（可留空）：" PROXY_DOMAIN
                if [[ -n "$PROXY_DOMAIN" ]]; then
                    read -r -p "ACME 邮箱（可留空）：" PROXY_EMAIL
                fi
            fi
        fi
        if [[ "$SKIP_CORE" -eq 1 ]]; then
            read -r -p "是否在安装后自动启动代理内核？[y/N]: " cans
            if [[ "${cans}" == "y" || "${cans}" == "Y" ]]; then
                SKIP_CORE=0
            fi
        fi
    fi
}

apply_systemd_optimize() {
    local unit="/etc/systemd/system/s-ui.service"
    [[ -f "$unit" ]] || return 0

    # Strip dangerous hard memory caps if present (from older installs).
    # Hard MemoryMax can thrash small VPS into a reboot-like freeze.
    sed -i '/^MemoryMax=/d;/^MemoryHigh=/d' "$unit" 2>/dev/null || true

    mkdir -p /etc/systemd/system/s-ui.service.d
    local skip_line=""
    local go_mem_lines=""
    if [[ "$SKIP_CORE" -eq 1 ]]; then
        skip_line="Environment=SUI_SKIP_CORE=true"
        mkdir -p /usr/local/s-ui/db
        touch /usr/local/s-ui/db/.skip_core
        chmod 644 /usr/local/s-ui/db/.skip_core
    else
        rm -f /usr/local/s-ui/db/.skip_core
    fi

    # Cap Go heap so panel web UI does not balloon toward total RAM.
    # Cores are separate processes (Xray) or started later (sing-box).
    if [[ "$MEM_TOTAL_MB" -lt 1200 ]]; then
        go_mem_lines=$'Environment=GOMEMLIMIT=180MiB\nEnvironment=GOGC=40'
    elif [[ "$MEM_TOTAL_MB" -lt 2048 ]]; then
        go_mem_lines=$'Environment=GOMEMLIMIT=280MiB\nEnvironment=GOGC=50'
    elif [[ "$MEM_TOTAL_MB" -lt 4096 ]]; then
        go_mem_lines=$'Environment=GOMEMLIMIT=512MiB\nEnvironment=GOGC=75'
    fi

    cat >/etc/systemd/system/s-ui.service.d/optimize.conf <<EOF
[Service]
# Prefer killing the panel, not the whole VPS, under memory pressure.
OOMScoreAdjust=800
Nice=10
${skip_line}
${go_mem_lines}
EOF

    if [[ "$SKIP_CORE" -eq 1 ]]; then
        echo -e "${green}已启用安全模式：面板启动时不自动加载 sing-box/Xray（防 OOM 关机）${plain}"
        echo -e "${yellow}需要代理时在面板配置入站后点「重启内核」${plain}"
    else
        echo -e "${yellow}已配置为自动启动代理内核（--start-core）${plain}"
    fi
    [[ -n "$go_mem_lines" ]] && echo -e "${green}已限制面板 Go 内存（GOMEMLIMIT），降低 OOM 风险${plain}"
}

suggest_swap() {
    if [[ "$SWAP_MB" -lt 512 ]]; then
        echo -e "${yellow}建议配置 ≥1G Swap 提升稳定性，例如：${plain}"
        echo -e "  fallocate -l 1G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile"
        echo -e "  echo '/swapfile none swap sw 0 0' >> /etc/fstab"
    fi
}

install_base() {
    # Only install missing tools. Never full upgrade. Skip apt update when possible
    # (apt update alone can OOM tiny VPS during install).
    local need=0
    for bin in curl tar; do
        command -v "$bin" >/dev/null 2>&1 || need=1
    done
    command -v wget >/dev/null 2>&1 || command -v curl >/dev/null 2>&1 || need=1
    if [[ "$need" -eq 0 ]]; then
        echo -e "${green}基础工具已就绪，跳过包管理器安装（省内存）${plain}"
        return 0
    fi
    case "${release}" in
    centos | almalinux | rocky | oracle)
        yum install -y -q wget curl tar ca-certificates
        ;;
    fedora)
        dnf install -y -q wget curl tar ca-certificates
        ;;
    arch | manjaro | parch)
        pacman -Sy --noconfirm wget curl tar ca-certificates
        ;;
    opensuse-tumbleweed)
        zypper -q install -y wget curl tar ca-certificates
        ;;
    *)
        # Avoid apt-get update on low RAM unless packages missing
        if [[ "$MEM_TOTAL_MB" -ge 1500 ]]; then
            apt-get update -qq 2>/dev/null || true
        fi
        DEBIAN_FRONTEND=noninteractive apt-get install -y -q --no-install-recommends curl tar ca-certificates \
            || DEBIAN_FRONTEND=noninteractive apt-get install -y -q curl tar ca-certificates
        ;;
    esac
}

xray_asset() {
    case "$(arch)" in
    amd64) echo 'Xray-linux-64.zip' ;;
    386) echo 'Xray-linux-32.zip' ;;
    arm64) echo 'Xray-linux-arm64-v8a.zip' ;;
    armv7) echo 'Xray-linux-arm32-v7a.zip' ;;
    armv6) echo 'Xray-linux-arm32-v6.zip' ;;
    armv5) echo 'Xray-linux-arm32-v5.zip' ;;
    s390x) echo 'Xray-linux-s390x.zip' ;;
    *) echo '' ;;
    esac
}

install_package() {
    local pkg="$1"
    case "${release}" in
    centos | almalinux | rocky | oracle)
        yum install -y -q "$pkg"
        ;;
    fedora)
        dnf install -y -q "$pkg"
        ;;
    arch | manjaro | parch)
        pacman -Sy --noconfirm "$pkg"
        ;;
    opensuse-tumbleweed)
        zypper -q install -y "$pkg"
        ;;
    *)
        DEBIAN_FRONTEND=noninteractive apt-get install -y -q "$pkg"
        ;;
    esac
}

install_caddy_pkg() {
    if command -v caddy >/dev/null 2>&1; then
        return 0
    fi
    case "${release}" in
    ubuntu | debian | armbian)
        apt-get update -qq
        # Official Caddy repo when available; fall back to distro package.
        if ! DEBIAN_FRONTEND=noninteractive apt-get install -y -q caddy 2>/dev/null; then
            apt-get install -y -q debian-keyring debian-archive-keyring apt-transport-https 2>/dev/null || true
            curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' 2>/dev/null | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg 2>/dev/null || true
            curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' 2>/dev/null | tee /etc/apt/sources.list.d/caddy-stable.list >/dev/null 2>&1 || true
            apt-get update -qq 2>/dev/null || true
            DEBIAN_FRONTEND=noninteractive apt-get install -y -q caddy || return 1
        fi
        ;;
    *)
        install_package caddy || return 1
        ;;
    esac
    command -v caddy >/dev/null 2>&1
}

write_caddy_config() {
    local domain="$1"
    local email="$2"
    local panel_port="${3:-2095}"
    mkdir -p /etc/caddy
    {
        if [[ -n "$domain" && -n "$email" ]]; then
            echo "{"
            echo "	email ${email}"
            echo "}"
        fi
        if [[ -n "$domain" ]]; then
            echo "${domain} {"
        else
            echo ":80 {"
        fi
        cat <<EOF
	encode gzip
	reverse_proxy 127.0.0.1:${panel_port} {
		header_up X-Real-IP {remote_host}
		header_up X-Forwarded-For {remote_host}
		header_up X-Forwarded-Proto {scheme}
	}
}
EOF
    } >/etc/caddy/Caddyfile
}

write_nginx_config() {
    local domain="$1"
    local panel_port="${2:-2095}"
    local conf_dir="/etc/nginx/conf.d"
    mkdir -p "$conf_dir" /etc/nginx/sites-available /etc/nginx/sites-enabled 2>/dev/null || true
    local conf_file="${conf_dir}/s-ui.conf"
    # Prefer sites-available when present (Debian style)
    if [[ -d /etc/nginx/sites-available ]]; then
        conf_file="/etc/nginx/sites-available/s-ui.conf"
    fi
    local server_name="_"
    [[ -n "$domain" ]] && server_name="$domain"

    cat >"$conf_file" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${server_name};

    client_max_body_size 32m;

    location / {
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_pass http://127.0.0.1:${panel_port};
    }
}
EOF
    if [[ -d /etc/nginx/sites-enabled ]]; then
        ln -sfn "$conf_file" /etc/nginx/sites-enabled/s-ui.conf
        # Disable default site if it would catch all traffic
        rm -f /etc/nginx/sites-enabled/default 2>/dev/null || true
    fi
}

bind_panel_localhost() {
    local domain="$1"
    local uri=""
    if [[ -n "$domain" ]]; then
        uri="https://${domain}/app/"
        /usr/local/s-ui/sui setting -listen 127.0.0.1 -domain "$domain" -uri "$uri" >/dev/null 2>&1 || true
    else
        /usr/local/s-ui/sui setting -listen 127.0.0.1 >/dev/null 2>&1 || true
    fi
    # Avoid restart storm; caller restarts once if needed.
}

install_reverse_proxy() {
    if [[ "$INSTALL_PROXY" -ne 1 ]]; then
        echo -e "${yellow}按预检结果跳过反代安装。${plain}"
        return 0
    fi

    local panel_port=2095
    if [[ -x /usr/local/s-ui/sui ]]; then
        local p
        p=$(/usr/local/s-ui/sui setting -show 2>/dev/null | awk -F'\t' '/Panel port/ {print $NF}' | tr -d ' ')
        [[ "$p" =~ ^[0-9]+$ ]] && panel_port="$p"
    fi

    echo -e "${yellow}正在配置反向代理（引擎: ${PROXY_ENGINE}）...${plain}"
    bind_panel_localhost "$PROXY_DOMAIN"
    systemctl try-restart s-ui 2>/dev/null || true
    sleep 1

    if [[ "$PROXY_ENGINE" == "caddy" ]]; then
        if ! install_caddy_pkg; then
            echo -e "${yellow}Caddy 安装失败，回退 Nginx${plain}"
            PROXY_ENGINE="nginx"
        else
            write_caddy_config "$PROXY_DOMAIN" "$PROXY_EMAIL" "$panel_port"
            systemctl enable caddy >/dev/null 2>&1 || true
            if systemctl restart caddy && systemctl is-active --quiet caddy; then
                echo -e "${green}Caddy 反代已启动${plain}"
                if [[ -n "$PROXY_DOMAIN" ]]; then
                    echo -e "面板地址：${green}https://${PROXY_DOMAIN}/app/${plain}"
                else
                    echo -e "面板地址：${green}http://服务器IP/app/${plain}（80 端口）"
                fi
                return 0
            fi
            echo -e "${yellow}Caddy 启动失败，回退 Nginx${plain}"
            PROXY_ENGINE="nginx"
        fi
    fi

    if [[ "$PROXY_ENGINE" == "nginx" ]]; then
        if ! install_package nginx; then
            echo -e "${red}Nginx 安装失败，跳过反代${plain}"
            /usr/local/s-ui/sui setting -listen - >/dev/null 2>&1 || true
            systemctl try-restart s-ui 2>/dev/null || true
            return 1
        fi
        write_nginx_config "$PROXY_DOMAIN" "$panel_port"
        nginx -t 2>/dev/null || true
        systemctl enable nginx >/dev/null 2>&1 || true
        if systemctl restart nginx && systemctl is-active --quiet nginx; then
            echo -e "${green}Nginx 反代已启动${plain}"
            if [[ -n "$PROXY_DOMAIN" ]]; then
                echo -e "HTTP：${green}http://${PROXY_DOMAIN}/app/${plain}"
                echo -e "${yellow}提示：可用 certbot --nginx -d ${PROXY_DOMAIN} 配置 HTTPS${plain}"
            else
                echo -e "面板地址：${green}http://服务器IP/app/${plain}（80 端口）"
            fi
            return 0
        fi
        echo -e "${red}Nginx 启动失败，已恢复面板监听全部网卡${plain}"
        /usr/local/s-ui/sui setting -listen - >/dev/null 2>&1 || true
        systemctl try-restart s-ui 2>/dev/null || true
        return 1
    fi
}

install_xray() {
    if [[ "$INSTALL_XRAY" -ne 1 ]]; then
        echo -e "${yellow}按预检结果跳过 Xray-core 安装（面板默认使用 sing-box）。${plain}"
        echo -e "${yellow}之后可在资源充足时重新执行安装脚本并加 --with-xray。${plain}"
        return 0
    fi

    local asset
    asset="$(xray_asset)"
    if [[ -z "$asset" ]]; then
        echo -e "${yellow}当前架构暂未配置 Xray-core 自动下载，跳过${plain}"
        return 0
    fi

    local xray_version
    xray_version=$(curl -Ls "https://api.github.com/repos/XTLS/Xray-core/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [[ ! -n "$xray_version" ]]; then
        echo -e "${yellow}获取 Xray-core 最新版本失败，跳过 Xray 准备${plain}"
        return 1
    fi

    echo -e "${yellow}正在安装 Xray-core ${xray_version}...${plain}"
    local tmp_dir="/tmp/s-ui-xray"
    rm -rf "$tmp_dir"
    mkdir -p "$tmp_dir" /usr/local/s-ui/bin

    local zip_path="/tmp/${asset}"
    local url="https://github.com/XTLS/Xray-core/releases/download/${xray_version}/${asset}"
    wget -N --no-check-certificate -O "$zip_path" "$url"
    if [[ $? -ne 0 ]]; then
        echo -e "${yellow}下载 Xray-core 失败，可稍后手动放置到 /usr/local/s-ui/bin/xray${plain}"
        rm -rf "$tmp_dir" "$zip_path"
        return 1
    fi

    unzip -qo "$zip_path" -d "$tmp_dir"
    if [[ ! -f "$tmp_dir/xray" ]]; then
        echo -e "${yellow}Xray-core 压缩包中未找到 xray 二进制，跳过${plain}"
        rm -rf "$tmp_dir" "$zip_path"
        return 1
    fi

    install -m 755 "$tmp_dir/xray" /usr/local/s-ui/bin/xray
    [[ -f "$tmp_dir/geoip.dat" ]] && install -m 644 "$tmp_dir/geoip.dat" /usr/local/s-ui/bin/geoip.dat
    [[ -f "$tmp_dir/geosite.dat" ]] && install -m 644 "$tmp_dir/geosite.dat" /usr/local/s-ui/bin/geosite.dat
    rm -rf "$tmp_dir" "$zip_path"
    echo -e "${green}Xray-core 已安装到 /usr/local/s-ui/bin/xray${plain}"
}

config_after_install() {
    # migrate is done once by install_s-ui before this; avoid double-loading 90MB binary.
    echo -e "${yellow}安装/更新完成！出于安全考虑，建议修改面板设置 ${plain}"
    read -r -p "是否继续修改设置 [y/n]？: " config_confirm
    if [[ "${config_confirm}" == "y" || "${config_confirm}" == "Y" ]]; then
        echo -e "请输入${yellow}面板端口${plain}（留空则使用现有/默认值）："
        read -r config_port
        echo -e "请输入${yellow}面板路径${plain}（留空则使用现有/默认值）："
        read -r config_path
        echo -e "请输入${yellow}订阅端口${plain}（留空则使用现有/默认值）："
        read -r config_subPort
        echo -e "请输入${yellow}订阅路径${plain}（留空则使用现有/默认值）："
        read -r config_subPath

        echo -e "${yellow}正在初始化，请稍候...${plain}"
        params=""
        [ -z "$config_port" ] || params="$params -port $config_port"
        [ -z "$config_path" ] || params="$params -path $config_path"
        [ -z "$config_subPort" ] || params="$params -subPort $config_subPort"
        [ -z "$config_subPath" ] || params="$params -subPath $config_subPath"
        # shellcheck disable=SC2086
        GOMEMLIMIT=200MiB GOGC=40 /usr/local/s-ui/sui setting ${params}

        read -r -p "是否修改管理员账号密码 [y/n]？: " admin_confirm
        if [[ "${admin_confirm}" == "y" || "${admin_confirm}" == "Y" ]]; then
            read -r -p "请设置用户名：" config_account
            read -r -p "请设置密码：" config_password
            echo -e "${yellow}正在初始化，请稍候...${plain}"
            GOMEMLIMIT=200MiB GOGC=40 /usr/local/s-ui/sui admin -username "${config_account}" -password "${config_password}"
        else
            echo -e "${yellow}当前管理员账号密码：${plain}"
            GOMEMLIMIT=200MiB GOGC=40 /usr/local/s-ui/sui admin -show
        fi
    else
        echo -e "${yellow}跳过自定义。默认管理员：admin / admin（请尽快修改）${plain}"
        echo -e "${yellow}忘记密码可执行： s-ui → 重置管理员${plain}"
    fi
}

prepare_services() {
    if [[ -f "/etc/systemd/system/sing-box.service" ]]; then
        echo -e "${yellow}正在停止 sing-box 服务... ${plain}"
        systemctl stop sing-box 2>/dev/null || true
        rm -f /usr/local/s-ui/bin/sing-box /usr/local/s-ui/bin/runSingbox.sh /usr/local/s-ui/bin/signal
    fi
    if [[ -e "/usr/local/s-ui/bin" ]]; then
        echo -e "###############################################################"
        echo -e "${green}/usr/local/s-ui/bin${yellow} 目录已存在，将保留其中自定义二进制${plain}"
        echo -e "###############################################################"
    fi
    systemctl daemon-reload
}

free_page_cache() {
    sync 2>/dev/null || true
    # drop_caches is optional; never fail install if blocked
    echo 1 >/proc/sys/vm/drop_caches 2>/dev/null || true
}

# Reclaimable + swap free, in MB (best-effort).
mem_budget_mb() {
    local avail_kb swap_free_kb
    avail_kb=$(awk '/MemAvailable/ {print $2}' /proc/meminfo 2>/dev/null || echo 0)
    swap_free_kb=$(awk '/SwapFree/ {print $2}' /proc/meminfo 2>/dev/null || echo 0)
    echo $(((avail_kb + swap_free_kb) / 1024))
}

# Refuse to launch the ~90MB binary if remaining budget is too low.
require_mem_budget() {
    local need="${1:-280}"
    local have
    have=$(mem_budget_mb)
    echo -e "可用内存预算（MemAvailable+SwapFree）：${have}MB（启动面板建议 ≥${need}MB）"
    if [[ "$have" -lt "$need" && "$FORCE_INSTALL" -ne 1 ]]; then
        echo -e "${red}内存预算不足，强行启动 90MB 面板进程极易 OOM 整机重启。${plain}"
        echo -e "${yellow}请：1) 确认 Swap 已启用  2) 关闭其它占内存进程  3) 或换 ≥2G 机器${plain}"
        echo -e "${yellow}排查： free -h; swapon --show; dmesg | grep -i oom | tail${plain}"
        return 1
    fi
    if [[ "$have" -lt "$need" ]]; then
        echo -e "${yellow}--force：内存预算不足仍继续（高风险）${plain}"
    fi
    return 0
}

ensure_swap_if_needed() {
    # Critical: create swap BEFORE download/extract. 90MB binary + Go RSS
    # easily OOMs 1GB machines with 0 swap (looks like a hard reboot).
    local need_mb=0
    if [[ "$MEM_TOTAL_MB" -lt 1500 ]]; then
        need_mb=2048
    elif [[ "$MEM_TOTAL_MB" -lt 2500 && "$SWAP_MB" -lt 512 ]]; then
        need_mb=1024
    else
        return 0
    fi
    if [[ "$SWAP_MB" -ge "$need_mb" ]]; then
        return 0
    fi
    if [[ -f /swapfile ]] && swapon --show 2>/dev/null | grep -q '/swapfile'; then
        local cur
        cur=$(swapon --show --bytes 2>/dev/null | awk '/swapfile/ {print int($3/1024/1024); exit}')
        [[ -n "$cur" && "$cur" -ge 512 ]] && return 0
    fi

    # Always create on low-RAM (no interactive cancel) — reboot risk is worse.
    local size_mb="$need_mb"
    echo -e "${yellow}低内存机器：自动创建 ${size_mb}MB Swap（防止安装时 OOM 关机）${plain}"

    # Prefer fallocate (cheap). dd as fallback but with low-memory-friendly block size.
    swapoff /swapfile 2>/dev/null || true
    rm -f /swapfile 2>/dev/null || true
    free_page_cache

    local ok=0
    if fallocate -l "${size_mb}M" /swapfile 2>/dev/null; then
        ok=1
    elif dd if=/dev/zero of=/swapfile bs=64M count=$((size_mb / 64)) status=none conv=fsync 2>/dev/null; then
        ok=1
    elif dd if=/dev/zero of=/swapfile bs=1M count="$size_mb" status=none conv=fsync 2>/dev/null; then
        ok=1
    fi
    if [[ "$ok" -ne 1 ]]; then
        echo -e "${red}创建 Swap 失败：请先手动加 Swap 再安装，否则 1G 机器极易重启${plain}"
        echo -e "  fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile"
        return 1
    fi
    chmod 600 /swapfile
    if ! mkswap /swapfile >/dev/null 2>&1; then
        echo -e "${red}mkswap 失败${plain}"
        rm -f /swapfile
        return 1
    fi
    if ! swapon /swapfile; then
        echo -e "${red}swapon 失败（部分虚拟机禁用 Swap）${plain}"
        return 1
    fi
    grep -q '/swapfile' /etc/fstab 2>/dev/null || echo '/swapfile none swap sw 0 0' >>/etc/fstab
    SWAP_MB=$size_mb
    echo -e "${green}Swap 已启用：${size_mb}MB${plain}"
    free -h 2>/dev/null || true
}

# Sets DOWNLOAD_TARBALL path on success.
download_release() {
    local version="$1"
    local arch_name
    arch_name="$(arch)"
    local url="https://github.com/Hhz0823/1s-ui/releases/download/${version}/s-ui-linux-${arch_name}.tar.gz"
    local out="/tmp/s-ui-linux-${arch_name}.tar.gz"
    DOWNLOAD_TARBALL=""
    rm -f "$out"
    echo -e "下载：${url}"
    if command -v curl >/dev/null 2>&1; then
        curl -fL --retry 3 --retry-delay 2 --connect-timeout 20 -o "$out" "$url" || return 1
    else
        wget -q --no-check-certificate -O "$out" "$url" || return 1
    fi
    local sz
    sz=$(stat -c%s "$out" 2>/dev/null || stat -f%z "$out" 2>/dev/null || echo 0)
    if [[ "${sz:-0}" -lt 5000000 ]]; then
        echo -e "${red}下载文件过小（${sz} bytes），可能 404 或截断${plain}"
        head -c 200 "$out" 2>/dev/null || true
        rm -f "$out"
        return 1
    fi
    DOWNLOAD_TARBALL="$out"
}

install_s-ui() {
    cd /tmp/ || exit 1

    # ------------------------------------------------------------------
    # OOM root-cause controls (see docs/oom-reboot-analysis.md):
    #  - Panel binary ~90MB; each `sui` CLI invocation is a full process.
    #  - Fresh install: migrate is a no-op (no DB) but still costs full RSS.
    #  - Critical path must launch the 90MB binary AT MOST ONCE (systemd start).
    # ------------------------------------------------------------------

    echo -e "${yellow}安装前停止旧服务并准备 Swap（防 OOM 重启）...${plain}"
    systemctl stop s-ui 2>/dev/null || true
    systemctl stop s-ui-agent 2>/dev/null || true
    free_page_cache
    if ! ensure_swap_if_needed; then
        if [[ "$MEM_TOTAL_MB" -lt 1500 && "$FORCE_INSTALL" -ne 1 ]]; then
            echo -e "${red}内存 ${MEM_TOTAL_MB}MB 且无法创建 Swap：1G 级机器上 90MB 面板极易 OOM 整机重启。${plain}"
            echo -e "${yellow}请先手动创建 Swap 后重试，或加 --force（不推荐）。${plain}"
            exit 1
        fi
        echo -e "${yellow}Swap 未就绪，继续安装（风险高）${plain}"
    fi
    free -h 2>/dev/null || true
    swapon --show 2>/dev/null || true

    local last_version
    if [[ -z "$REQUESTED_VERSION" ]]; then
        last_version=$(curl -Ls "https://api.github.com/repos/Hhz0823/1s-ui/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
        if [[ ! -n "$last_version" ]]; then
            last_version=$(curl -Ls "https://api.github.com/repos/Hhz0823/1s-ui/releases?per_page=5" | grep '"tag_name":' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')
        fi
        if [[ ! -n "$last_version" ]]; then
            echo -e "${red}获取 s-ui 版本失败，可能是 Github API 限制导致，请稍后重试${plain}"
            exit 1
        fi
        echo -e "已获取 s-ui 版本：${last_version}，开始安装..."
    else
        last_version="$REQUESTED_VERSION"
        [[ "${last_version}" != v* ]] && last_version="v${last_version}"
        echo -e "开始安装 s-ui ${last_version}"
    fi

    if ! download_release "$last_version"; then
        echo -e "${red}下载 s-ui ${last_version} 失败，请确认可访问 Github${plain}"
        exit 1
    fi
    local tarball="$DOWNLOAD_TARBALL"

    systemctl stop s-ui 2>/dev/null || true
    rm -rf /tmp/s-ui-extract
    mkdir -p /tmp/s-ui-extract

    # Selective extract: only panel binary + unit + helper. Skip sui-agent (~6.6MB)
    # and anything else so peak disk/page-cache is one 90MB file, not two.
    echo -e "${yellow}选择性解压（仅 sui + service，跳过 agent）...${plain}"
    local extract_ok=0
    if tar xzf "$tarball" -C /tmp/s-ui-extract s-ui/sui s-ui/s-ui.service s-ui/s-ui.sh 2>/dev/null; then
        extract_ok=1
    elif tar xzf "$tarball" -C /tmp/s-ui-extract sui s-ui.service s-ui.sh 2>/dev/null; then
        extract_ok=1
    else
        # Fallback: full extract (older layouts)
        echo -e "${yellow}选择性解压失败，回退完整解压${plain}"
        tar xzf "$tarball" -C /tmp/s-ui-extract || extract_ok=0
        [[ -f /tmp/s-ui-extract/s-ui/sui || -f /tmp/s-ui-extract/sui ]] && extract_ok=1
    fi
    rm -f "$tarball"
    free_page_cache
    if [[ "$extract_ok" -ne 1 ]]; then
        echo -e "${red}解压失败${plain}"
        rm -rf /tmp/s-ui-extract
        exit 1
    fi

    local src_dir="/tmp/s-ui-extract/s-ui"
    [[ -d "$src_dir" ]] || src_dir="/tmp/s-ui-extract"
    if [[ ! -f "$src_dir/sui" ]]; then
        echo -e "${red}包内未找到 sui 二进制${plain}"
        ls -la "$src_dir" /tmp/s-ui-extract 2>/dev/null || true
        rm -rf /tmp/s-ui-extract
        exit 1
    fi

    mkdir -p /usr/local/s-ui /usr/local/s-ui/db /usr/local/s-ui/bin
    # Single copy of the heavy binary into place, then delete extract tree immediately.
    cp -f "$src_dir/sui" /usr/local/s-ui/sui
    chmod +x /usr/local/s-ui/sui
    if [[ -f "$src_dir/s-ui.sh" ]]; then
        cp -f "$src_dir/s-ui.sh" /usr/local/s-ui/s-ui.sh
        cp -f "$src_dir/s-ui.sh" /usr/bin/s-ui
        chmod +x /usr/bin/s-ui /usr/local/s-ui/s-ui.sh
    fi
    if [[ -f "$src_dir/s-ui.service" ]]; then
        cp -f "$src_dir/s-ui.service" /etc/systemd/system/s-ui.service
    elif [[ -f /tmp/s-ui-extract/s-ui.service ]]; then
        cp -f /tmp/s-ui-extract/s-ui.service /etc/systemd/system/s-ui.service
    fi
    # Never install agent binary during low-memory panel install (optional later).
    if [[ "$MEM_TOTAL_MB" -ge 2048 && -f "$src_dir/sui-agent" ]]; then
        cp -f "$src_dir/sui-agent" /usr/local/s-ui/sui-agent
        chmod +x /usr/local/s-ui/sui-agent
        [[ -f "$src_dir/s-ui-agent.service" ]] && cp -f "$src_dir/s-ui-agent.service" /etc/systemd/system/
    fi
    rm -rf /tmp/s-ui-extract
    free_page_cache

    # systemd: SUI_SKIP_CORE + GOMEMLIMIT before first start
    apply_systemd_optimize
    prepare_services

    local has_db=0
    [[ -f /usr/local/s-ui/db/s-ui.db ]] && has_db=1

    # ---- CLI policy (critical) ----
    # Fresh install: DO NOT run `sui migrate` or `sui admin`.
    #   migrate() exits immediately when DB missing, but still loads ~90MB RSS.
    #   InitDB on first service start creates admin/admin.
    # Upgrade: run migrate ONCE only if DB already exists.
    if [[ "$has_db" -eq 1 ]]; then
        echo -e "${yellow}检测到已有数据库：升级路径，执行一次 migrate...${plain}"
        if require_mem_budget 300; then
            GOMEMLIMIT=200MiB GOGC=40 /usr/local/s-ui/sui migrate || \
                echo -e "${yellow}migrate 非零退出，继续启动服务${plain}"
            free_page_cache
        else
            echo -e "${red}内存不足，跳过 migrate；请空闲时手动： /usr/local/s-ui/sui migrate${plain}"
        fi
    else
        echo -e "${green}全新安装：跳过 migrate/admin CLI（避免无意义的 90MB 进程峰值）${plain}"
        echo -e "${yellow}默认登录：admin / admin（登录后请立即修改）${plain}"
    fi

    # Interactive settings only when user is present AND memory allows an extra CLI.
    # Skip on -y / low RAM to keep critical path = single service start.
    if [[ "$AUTO_YES" -ne 1 && "$MEM_TOTAL_MB" -ge 1500 ]]; then
        config_after_install
    elif [[ "$AUTO_YES" -ne 1 ]]; then
        echo -e "${yellow}低内存：跳过交互式 setting CLI；可用面板 Web 或稍后 s-ui 菜单修改${plain}"
    fi

    systemctl daemon-reload
    systemctl enable s-ui >/dev/null 2>&1 || true

    if [[ "$START_SERVICE" -eq 1 ]]; then
        free_page_cache
        # This should be the ONLY full binary launch on a fresh install.
        require_mem_budget 280 || exit 1
        echo -e "${yellow}启动面板（唯一一次加载 90MB 进程；安全模式不启内核）...${plain}"
        if ! systemctl start s-ui; then
            echo -e "${red}s-ui 服务启动失败，最近日志：${plain}"
            journalctl -u s-ui -n 80 --no-pager || true
            echo -e "${yellow}请查 OOM: dmesg | grep -iE 'oom|killed' | tail${plain}"
            exit 1
        fi
        sleep 3
        if ! systemctl is-active --quiet s-ui; then
            echo -e "${red}s-ui 未能保持运行，最近日志：${plain}"
            journalctl -u s-ui -n 80 --no-pager || true
            dmesg 2>/dev/null | grep -iE 'oom|kill' | tail -20 || true
            exit 1
        fi
        echo -e "${green}面板已运行（未自动加载代理内核）${plain}"

        # Never install Xray/proxy in the same critical window on tiny hosts.
        if [[ "$INSTALL_XRAY" -eq 1 ]]; then
            if [[ "$MEM_TOTAL_MB" -lt 1500 ]]; then
                echo -e "${yellow}内存 <1.5G：拒绝安装期装 Xray（面板稳定后用 --with-xray）${plain}"
            else
                install_xray || echo -e "${yellow}Xray-core 未安装${plain}"
            fi
        fi
        if [[ "$INSTALL_PROXY" -eq 1 ]]; then
            if [[ "$MEM_TOTAL_MB" -lt 1500 ]]; then
                echo -e "${yellow}内存 <1.5G：跳过安装期反代${plain}"
            else
                install_reverse_proxy || true
            fi
        fi
    else
        echo -e "${yellow}已按 --no-start 跳过启动。稍后：systemctl start s-ui${plain}"
    fi

    echo -e "${green}s-ui ${last_version}${plain} 安装完成"
    echo -e "安装摘要：模式=${INSTALL_MODE} 档位=${PROFILE} Xray=$([ "$INSTALL_XRAY" -eq 1 ] && echo 是 || echo 否) 反代=$([ "$INSTALL_PROXY" -eq 1 ] && echo "是(${PROXY_ENGINE})" || echo 否) 自动启内核=$([ "$SKIP_CORE" -eq 1 ] && echo 否 || echo 是) Swap=${SWAP_MB}MB"
    echo -e "访问：浏览器打开 http://服务器IP:2095/app/  （默认 admin/admin）"
    if [[ "$SKIP_CORE" -eq 1 ]]; then
        echo -e "${yellow}安全模式：配置入站后在面板内重启内核再启用代理。${plain}"
    fi
    echo -e ""
    echo -e "${yellow}若仍关机： free -h; swapon --show; dmesg | grep -iE 'oom|kill' | tail -30; journalctl -u s-ui -n 80 --no-pager${plain}"
    echo -e "${blue}分析文档：https://github.com/Hhz0823/1s-ui/blob/main/docs/oom-reboot-analysis.md${plain}"
}

# ---- main ----
parse_args "$@"
echo -e "${green}1S-UI 安装程序${plain}"
echo -e "当前系统发行版为：${release}"
echo -e "架构：$(arch)"
analyze_vps
# Swap as early as possible (before apt / downloads)
ensure_swap_if_needed || true
install_base
install_s-ui
