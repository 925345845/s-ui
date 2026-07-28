#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
blue='\033[0;34m'
plain='\033[0m'

cur_dir=$(pwd)

# Decision flags (set by analyze_vps)
INSTALL_PANEL=1
INSTALL_XRAY=1
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
# Hard minimum for panel server install
MIN_CPU_CORES=2
MIN_MEM_MB=2048

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
  --force               强制安装（跳过 2核2G 最低配置检查，不推荐）
  -h, --help            显示帮助

服务端最低配置: ${MIN_CPU_CORES:-2} 核 CPU + ${MIN_MEM_MB:-2048}MB 内存

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

    echo -e "CPU：${CPU_CORES} 核（最低要求 ${MIN_CPU_CORES} 核）"
    echo -e "内存：总计 ${MEM_TOTAL_MB}MB / 可用 ${MEM_AVAIL_MB}MB / Swap ${SWAP_MB}MB（最低要求 ${MIN_MEM_MB}MB）"
    echo -e "磁盘：/usr/local 可用约 ${DISK_FREE_MB}MB"

    # Profile (only used after hard minimum is met)
    if [[ "$MEM_TOTAL_MB" -lt 2800 || "$CPU_CORES" -lt 4 ]]; then
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
        echo -e "面板：${green}未检测到面板，将全新安装服务端${plain}"
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

    # ---- Hard minimum: 2 cores + 2GB RAM for panel server ----
    INSTALL_PANEL=1
    local fail_reasons=()
    if [[ "$CPU_CORES" -lt "$MIN_CPU_CORES" ]]; then
        fail_reasons+=("CPU 仅 ${CPU_CORES} 核，需要至少 ${MIN_CPU_CORES} 核")
    fi
    if [[ "$MEM_TOTAL_MB" -lt "$MIN_MEM_MB" ]]; then
        fail_reasons+=("内存仅 ${MEM_TOTAL_MB}MB，需要至少 ${MIN_MEM_MB}MB（2G）")
    fi
    if [[ "$DISK_FREE_MB" -gt 0 && "$DISK_FREE_MB" -lt 500 ]]; then
        fail_reasons+=("磁盘可用约 ${DISK_FREE_MB}MB，建议至少 500MB")
    fi

    if [[ ${#fail_reasons[@]} -gt 0 ]]; then
        echo -e "${red}不满足服务端最低配置（${MIN_CPU_CORES} 核 / ${MIN_MEM_MB}MB 内存）：${plain}"
        local r
        for r in "${fail_reasons[@]}"; do
            echo -e "  - ${red}${r}${plain}"
        done
        if [[ "$FORCE_INSTALL" -eq 1 ]]; then
            echo -e "${yellow}已使用 --force，强制继续安装（极易 OOM，不推荐）${plain}"
            INSTALL_PANEL=1
        else
            INSTALL_PANEL=0
            echo -e "${yellow}请升级到至少 2 核 2G 的 VPS 后再安装服务端。${plain}"
            echo -e "${yellow}若坚持安装可加：--force（不保证稳定）${plain}"
        fi
    else
        echo -e "最低配置检查：${green}通过（≥${MIN_CPU_CORES} 核 / ≥${MIN_MEM_MB}MB）${plain}"
    fi

    # Xray: only when minimum met; auto on high, optional on standard
    INSTALL_XRAY=0
    local xray_reason="默认不装 Xray（可用 --with-xray 安装）"
    if [[ "$INSTALL_PANEL" -ne 1 ]]; then
        xray_reason="服务端不安装，跳过 Xray"
    elif [[ -z "$(xray_asset)" ]]; then
        INSTALL_XRAY=0
        xray_reason="当前架构无自动 Xray 包"
    elif [[ "$PROFILE" == "high" ]]; then
        INSTALL_XRAY=1
        xray_reason="≥4 核且内存较充足，建议安装 Xray 双内核"
    elif [[ "$PROFILE" == "standard" && "$MEM_TOTAL_MB" -ge 2048 && "$SWAP_MB" -ge 512 ]]; then
        INSTALL_XRAY=1
        xray_reason="达到 2 核 2G 且有 Swap，可安装 Xray"
    fi

    # Explicit overrides
    if [[ "$FORCE_XRAY" == "1" ]]; then
        INSTALL_XRAY=1
        xray_reason="用户指定 --with-xray"
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

    echo -e "推荐：服务端=${green}${INSTALL_PANEL}${plain}  Xray=${green}${INSTALL_XRAY}${plain}  反代=${green}${INSTALL_PROXY}${plain}$([ "$INSTALL_PROXY" -eq 1 ] && echo "(${PROXY_ENGINE})" || true)  自动启内核=${green}$([ "$SKIP_CORE" -eq 1 ] && echo 否 || echo 是)${plain}"
    echo -e "Xray 原因：${xray_reason}"
    echo -e "反代 原因：${proxy_reason}"
    echo -e "内核策略：${yellow}默认不自动启动 sing-box/Xray，仅启动面板 Web，避免 OOM 关机${plain}"
    echo -e "${blue}=================================${plain}"

    if [[ "$INSTALL_PANEL" -ne 1 ]]; then
        echo -e "${red}根据预检结果不安装服务端。最低配置：${MIN_CPU_CORES} 核 CPU + ${MIN_MEM_MB}MB 内存。${plain}"
        exit 1
    fi

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
    sed -i '/^MemoryMax=/d;/^MemoryHigh=/d' "$unit" 2>/dev/null || true

    mkdir -p /etc/systemd/system/s-ui.service.d
    local skip_line=""
    if [[ "$SKIP_CORE" -eq 1 ]]; then
        skip_line="Environment=SUI_SKIP_CORE=true"
        # Marker file as backup if env is lost
        mkdir -p /usr/local/s-ui/db
        touch /usr/local/s-ui/db/.skip_core
        chmod 644 /usr/local/s-ui/db/.skip_core
    else
        rm -f /usr/local/s-ui/db/.skip_core
    fi

    cat >/etc/systemd/system/s-ui.service.d/optimize.conf <<EOF
[Service]
# Prefer killing the panel, not the whole VPS, under memory pressure.
OOMScoreAdjust=500
Nice=5
${skip_line}
EOF

    if [[ "$SKIP_CORE" -eq 1 ]]; then
        echo -e "${green}已启用安全模式：面板启动时不自动加载 sing-box/Xray（防 OOM 关机）${plain}"
        echo -e "${yellow}需要代理时在面板保存入站/点重启内核，或：rm -f /usr/local/s-ui/db/.skip_core && systemctl edit 去掉 SUI_SKIP_CORE 后重启 s-ui${plain}"
    else
        echo -e "${yellow}已配置为自动启动代理内核（--start-core）${plain}"
    fi
}

suggest_swap() {
    if [[ "$SWAP_MB" -lt 512 ]]; then
        echo -e "${yellow}建议配置 ≥1G Swap 提升稳定性，例如：${plain}"
        echo -e "  fallocate -l 1G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile"
        echo -e "  echo '/swapfile none swap sw 0 0' >> /etc/fstab"
    fi
}

install_base() {
    # Only install missing tools. Never run a full system upgrade here.
    case "${release}" in
    centos | almalinux | rocky | oracle)
        yum install -y -q wget curl tar unzip tzdata ca-certificates
        ;;
    fedora)
        dnf install -y -q wget curl tar unzip tzdata ca-certificates
        ;;
    arch | manjaro | parch)
        pacman -Sy --noconfirm wget curl tar unzip tzdata ca-certificates
        ;;
    opensuse-tumbleweed)
        zypper -q install -y wget curl tar unzip timezone ca-certificates
        ;;
    *)
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -q wget curl tar unzip tzdata ca-certificates
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
    echo -e "${yellow}正在迁移... ${plain}"
    /usr/local/s-ui/sui migrate

    # Non-interactive: keep defaults / random admin on fresh install
    if [[ "$AUTO_YES" -eq 1 ]]; then
        if [[ ! -f "/usr/local/s-ui/db/s-ui.db" ]]; then
            local usernameTemp passwordTemp
            usernameTemp=$(head -c 6 /dev/urandom | base64 | tr -d '/+=' | head -c 8)
            passwordTemp=$(head -c 9 /dev/urandom | base64 | tr -d '/+=' | head -c 12)
            echo -e "自动安装：生成随机管理员账号"
            echo -e "###############################################"
            echo -e "${green}用户名：${usernameTemp}${plain}"
            echo -e "${green}密码：${passwordTemp}${plain}"
            echo -e "###############################################"
            /usr/local/s-ui/sui admin -username "${usernameTemp}" -password "${passwordTemp}"
        else
            echo -e "${yellow}自动安装：升级模式，保留原管理员与设置${plain}"
        fi
        return 0
    fi

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
        /usr/local/s-ui/sui setting ${params}

        read -r -p "是否修改管理员账号密码 [y/n]？: " admin_confirm
        if [[ "${admin_confirm}" == "y" || "${admin_confirm}" == "Y" ]]; then
            read -r -p "请设置用户名：" config_account
            read -r -p "请设置密码：" config_password
            echo -e "${yellow}正在初始化，请稍候...${plain}"
            /usr/local/s-ui/sui admin -username "${config_account}" -password "${config_password}"
        else
            echo -e "${yellow}当前管理员账号密码：${plain}"
            /usr/local/s-ui/sui admin -show
        fi
    else
        echo -e "${red}已取消自定义设置...${plain}"
        if [[ ! -f "/usr/local/s-ui/db/s-ui.db" ]]; then
            local usernameTemp passwordTemp
            usernameTemp=$(head -c 6 /dev/urandom | base64 | tr -d '/+=' | head -c 8)
            passwordTemp=$(head -c 9 /dev/urandom | base64 | tr -d '/+=' | head -c 12)
            echo -e "这是全新安装，出于安全考虑将生成随机登录信息："
            echo -e "###############################################"
            echo -e "${green}用户名：${usernameTemp}${plain}"
            echo -e "${green}密码：${passwordTemp}${plain}"
            echo -e "###############################################"
            echo -e "${red}如果忘记登录信息，可以输入 ${green}s-ui${red} 打开配置菜单${plain}"
            /usr/local/s-ui/sui admin -username "${usernameTemp}" -password "${passwordTemp}"
        else
            echo -e "${red}这是升级安装，将保留旧设置；如果忘记登录信息，可以输入 ${green}s-ui${red} 打开配置菜单${plain}"
        fi
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

ensure_swap_if_needed() {
    # Auto-create 1G swap on small VPS with no swap — major OOM mitigator.
    if [[ "$SWAP_MB" -ge 256 ]]; then
        return 0
    fi
    if [[ "$MEM_TOTAL_MB" -ge 2048 ]]; then
        return 0
    fi
    if [[ -f /swapfile ]] && swapon --show 2>/dev/null | grep -q swapfile; then
        return 0
    fi
    local do_swap=0
    if [[ "$AUTO_YES" -eq 1 ]]; then
        do_swap=1
    else
        read -r -p "检测到无 Swap，是否自动创建 1G Swap 防止关机？[Y/n]: " sans
        if [[ "${sans}" != "n" && "${sans}" != "N" ]]; then
            do_swap=1
        fi
    fi
    if [[ "$do_swap" -ne 1 ]]; then
        return 0
    fi
    echo -e "${yellow}正在创建 /swapfile (1G)...${plain}"
    if fallocate -l 1G /swapfile 2>/dev/null || dd if=/dev/zero of=/swapfile bs=1M count=1024 status=none; then
        chmod 600 /swapfile
        mkswap /swapfile >/dev/null
        swapon /swapfile
        grep -q '/swapfile' /etc/fstab 2>/dev/null || echo '/swapfile none swap sw 0 0' >>/etc/fstab
        SWAP_MB=1024
        echo -e "${green}Swap 已启用${plain}"
    else
        echo -e "${yellow}创建 Swap 失败，继续安装但请留意 OOM${plain}"
    fi
}

install_s-ui() {
    cd /tmp/ || exit 1

    # Free RAM before large download / extract (critical on ~1GB VPS)
    echo -e "${yellow}安装前停止旧服务以释放内存...${plain}"
    systemctl stop s-ui 2>/dev/null || true
    systemctl stop s-ui-agent 2>/dev/null || true
    # Best-effort drop caches (safe, needs root)
    sync 2>/dev/null || true
    echo 3 >/proc/sys/vm/drop_caches 2>/dev/null || true
    ensure_swap_if_needed

    local last_version
    if [[ -z "$REQUESTED_VERSION" ]]; then
        last_version=$(curl -Ls "https://api.github.com/repos/Hhz0823/1s-ui/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
        if [[ ! -n "$last_version" ]]; then
            echo -e "${red}获取 s-ui 版本失败，可能是 Github API 限制导致，请稍后重试${plain}"
            exit 1
        fi
        echo -e "已获取 s-ui 最新版本：${last_version}，开始安装..."
        wget --no-check-certificate -O /tmp/s-ui-linux-$(arch).tar.gz "https://github.com/Hhz0823/1s-ui/releases/download/${last_version}/s-ui-linux-$(arch).tar.gz"
        if [[ $? -ne 0 ]]; then
            echo -e "${red}下载 s-ui 失败，请确认服务器可以访问 Github ${plain}"
            exit 1
        fi
    else
        last_version="$REQUESTED_VERSION"
        [[ "${last_version}" != v* ]] && last_version="v${last_version}"
        local url="https://github.com/Hhz0823/1s-ui/releases/download/${last_version}/s-ui-linux-$(arch).tar.gz"
        echo -e "开始安装 s-ui ${last_version}"
        wget --no-check-certificate -O /tmp/s-ui-linux-$(arch).tar.gz "${url}"
        if [[ $? -ne 0 ]]; then
            echo -e "${red}下载 s-ui ${last_version} 失败，请检查该版本是否存在${plain}"
            exit 1
        fi
    fi

    # Quiet extract to reduce IO; stop again in case something restarted
    systemctl stop s-ui 2>/dev/null || true
    rm -rf /tmp/s-ui-extract
    mkdir -p /tmp/s-ui-extract
    tar xzf "s-ui-linux-$(arch).tar.gz" -C /tmp/s-ui-extract
    rm -f "s-ui-linux-$(arch).tar.gz"

    # Support both s-ui/ layout and flat extract
    local src_dir="/tmp/s-ui-extract/s-ui"
    [[ -d "$src_dir" ]] || src_dir="/tmp/s-ui-extract"

    chmod +x "$src_dir/sui" "$src_dir/s-ui.sh" 2>/dev/null || true
    cp "$src_dir/s-ui.sh" /usr/bin/s-ui
    mkdir -p /usr/local/s-ui
    # Preserve existing db/cert while replacing binaries
    cp -f "$src_dir/sui" /usr/local/s-ui/sui
    cp -f "$src_dir/s-ui.sh" /usr/local/s-ui/s-ui.sh 2>/dev/null || true
    cp -f "$src_dir"/*.service /etc/systemd/system/ 2>/dev/null || true
    # Copy remaining assets if present (html etc may be embedded)
    if [[ -d "$src_dir/bin" ]]; then
        mkdir -p /usr/local/s-ui/bin
        cp -rf "$src_dir/bin/." /usr/local/s-ui/bin/ 2>/dev/null || true
    fi
    rm -rf /tmp/s-ui-extract
    chmod +x /usr/local/s-ui/sui /usr/bin/s-ui

    # ALWAYS enable safe core skip unless user passed --start-core
    apply_systemd_optimize
    config_after_install
    prepare_services

    # Defer Xray binary install until after panel is up (reduces peak RAM during install)
    systemctl daemon-reload
    systemctl enable s-ui

    if [[ "$START_SERVICE" -eq 1 ]]; then
        sync 2>/dev/null || true
        echo 3 >/proc/sys/vm/drop_caches 2>/dev/null || true
        if ! systemctl start s-ui; then
            echo -e "${red}s-ui 服务启动失败，最近日志：${plain}"
            journalctl -u s-ui -n 80 --no-pager || true
            echo -e "${yellow}请查 OOM: dmesg | grep -iE 'oom|killed' | tail${plain}"
            exit 1
        fi
        sleep 2
        if ! systemctl is-active --quiet s-ui; then
            echo -e "${red}s-ui 未能保持运行状态，最近日志：${plain}"
            journalctl -u s-ui -n 80 --no-pager || true
            exit 1
        fi
        echo -e "${green}面板进程已稳定运行（安全模式：未自动加载代理内核）${plain}"

        # Install Xray binary only after panel is healthy (optional)
        install_xray || echo -e "${yellow}Xray-core 未安装；Sing-Box 功能不受影响${plain}"
        # Reverse proxy only if explicitly requested
        install_reverse_proxy || true
    else
        echo -e "${yellow}已按 --no-start 跳过启动。稍后执行: systemctl start s-ui${plain}"
        install_xray || true
    fi

    echo -e "${green}s-ui ${last_version}${plain} 安装完成"
    echo -e "安装摘要：模式=${INSTALL_MODE} 档位=${PROFILE} 面板=是 Xray=$([ "$INSTALL_XRAY" -eq 1 ] && echo 是 || echo 否) 反代=$([ "$INSTALL_PROXY" -eq 1 ] && echo "是(${PROXY_ENGINE})" || echo 否) 自动启内核=$([ "$SKIP_CORE" -eq 1 ] && echo 否 || echo 是)"
    if systemctl is-active --quiet s-ui 2>/dev/null; then
        echo -e "面板访问地址：${green}"
        /usr/local/s-ui/sui uri 2>/dev/null || true
        echo -e "${plain}"
    fi
    if [[ "$SKIP_CORE" -eq 1 ]]; then
        echo -e "${yellow}安全模式：代理内核未自动启动。打开面板配置入站后，在面板中重启内核，或：${plain}"
        echo -e "  rm -f /usr/local/s-ui/db/.skip_core"
        echo -e "  rm -f /etc/systemd/system/s-ui.service.d/optimize.conf"
        echo -e "  systemctl daemon-reload && systemctl restart s-ui"
    fi
    suggest_swap
    echo -e ""
    echo -e "${yellow}若仍出现关机/断连，请提供： free -h; dmesg | grep -iE 'oom|kill' | tail; journalctl -u s-ui -n 50${plain}"
    s-ui help 2>/dev/null || true
}

# ---- main ----
parse_args "$@"
echo -e "${green}1S-UI 安装程序${plain}"
echo -e "当前系统发行版为：${release}"
echo -e "架构：$(arch)"
analyze_vps
install_base
install_s-ui
