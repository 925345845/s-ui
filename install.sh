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
REQUESTED_VERSION=""

# 检查 root 权限
[[ $EUID -ne 0 ]] && echo -e "${red}致命错误：${plain}请使用 root 权限运行此脚本 \n " && exit 1

usage() {
    cat <<EOF
用法: install.sh [版本号] [选项]

选项:
  -y, --yes           非交互：接受推荐方案并安装
  --with-xray         强制安装 Xray-core
  --no-xray           跳过 Xray-core（仅 sing-box）
  --force             内存不足时仍允许安装面板
  -h, --help          显示帮助

示例:
  bash install.sh
  bash install.sh v1.5.1
  bash install.sh v1.5.1 -y --no-xray
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

    # Profile by total RAM
    if [[ "$MEM_TOTAL_MB" -lt 450 ]]; then
        PROFILE="tiny"
    elif [[ "$MEM_TOTAL_MB" -lt 900 ]]; then
        PROFILE="low"
    elif [[ "$MEM_TOTAL_MB" -lt 2800 ]]; then
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
    local p_busy=0
    for p in 2095 2096; do
        if port_in_use "$p"; then
            echo -e "端口 ${p}：${yellow}已被占用${plain}（安装后可在面板修改）"
            p_busy=1
        else
            echo -e "端口 ${p}：空闲"
        fi
    done

    # Conflicting proxies (informational)
    for svc in x-ui 3x-ui v2ray xray sing-box nginx caddy; do
        if systemctl is-active --quiet "$svc" 2>/dev/null; then
            echo -e "运行中服务：${yellow}${svc}${plain}"
        fi
    done

    # Disk guard
    if [[ "$DISK_FREE_MB" -gt 0 && "$DISK_FREE_MB" -lt 250 ]]; then
        echo -e "${red}磁盘空间过小（<250MB），不建议安装。${plain}"
        if [[ "$FORCE_INSTALL" -ne 1 ]]; then
            INSTALL_PANEL=0
        fi
    fi

    # Panel decision
    INSTALL_PANEL=1
    if [[ "$PROFILE" == "tiny" && "$FORCE_INSTALL" -ne 1 ]]; then
        echo -e "${yellow}内存 <450MB：安装服务端风险较高（可能 OOM）。${plain}"
        if [[ "$AUTO_YES" -eq 1 ]]; then
            if [[ "$SWAP_MB" -ge 512 || "$MEM_AVAIL_MB" -ge 280 ]]; then
                echo -e "${yellow}自动模式：检测到可用内存/Swap 尚可，继续安装面板（不装 Xray）。${plain}"
            else
                echo -e "${red}自动模式：资源过紧，中止安装。请加 Swap 或使用 --force。${plain}"
                INSTALL_PANEL=0
            fi
        else
            read -r -p "仍要安装服务端？[y/N]: " ans
            if [[ "${ans}" != "y" && "${ans}" != "Y" ]]; then
                INSTALL_PANEL=0
            fi
        fi
    fi

    # Xray decision
    INSTALL_XRAY=1
    local xray_reason=""
    if [[ -z "$(xray_asset)" ]]; then
        INSTALL_XRAY=0
        xray_reason="当前架构无自动 Xray 包"
    elif [[ "$PROFILE" == "tiny" ]]; then
        INSTALL_XRAY=0
        xray_reason="内存过小，优先保证 sing-box 面板稳定"
    elif [[ "$PROFILE" == "low" ]]; then
        INSTALL_XRAY=0
        xray_reason="内存 <1GB，默认跳过 Xray，可稍后手动装"
        if [[ "$MEM_AVAIL_MB" -ge 450 && "$SWAP_MB" -ge 256 ]]; then
            INSTALL_XRAY=1
            xray_reason="可用内存与 Swap 尚可，建议安装 Xray"
        fi
    elif [[ "$PROFILE" == "standard" || "$PROFILE" == "high" ]]; then
        INSTALL_XRAY=1
        xray_reason="资源充足，建议安装 Xray 以支持双内核"
    fi

    if [[ -x /usr/local/s-ui/bin/xray ]] || command -v xray >/dev/null 2>&1; then
        echo -e "Xray：${green}系统中已存在 xray 二进制${plain}"
        if [[ "$FORCE_XRAY" != "0" ]]; then
            INSTALL_XRAY=1
            xray_reason="已有 Xray，将刷新/保留"
        fi
    fi

    # Explicit overrides
    if [[ "$FORCE_XRAY" == "1" ]]; then
        INSTALL_XRAY=1
        xray_reason="用户指定 --with-xray"
    elif [[ "$FORCE_XRAY" == "0" ]]; then
        INSTALL_XRAY=0
        xray_reason="用户指定 --no-xray"
    fi

    echo -e "推荐：安装服务端=${green}${INSTALL_PANEL}${plain}  安装 Xray=${green}${INSTALL_XRAY}${plain}"
    echo -e "原因：${xray_reason}"
    echo -e "${blue}=================================${plain}"

    if [[ "$INSTALL_PANEL" -ne 1 ]]; then
        echo -e "${red}根据预检结果不安装服务端。可加 Swap 后重试，或使用 --force。${plain}"
        exit 1
    fi

    if [[ "$AUTO_YES" -ne 1 ]]; then
        echo -e "将按以上方案安装（模式: ${INSTALL_MODE}, 档位: ${PROFILE}）。"
        read -r -p "确认继续？[Y/n]: " confirm
        if [[ "${confirm}" == "n" || "${confirm}" == "N" ]]; then
            echo "已取消。"
            exit 0
        fi
        if [[ "$INSTALL_XRAY" -eq 0 && "$FORCE_XRAY" == "" && "$PROFILE" != "tiny" ]]; then
            read -r -p "是否仍安装 Xray-core？[y/N]: " xans
            if [[ "${xans}" == "y" || "${xans}" == "Y" ]]; then
                INSTALL_XRAY=1
            fi
        fi
    fi
}

apply_systemd_optimize() {
    local unit="/etc/systemd/system/s-ui.service"
    [[ -f "$unit" ]] || return 0

    local mem_high mem_max oom_score
    case "$PROFILE" in
    tiny)
        mem_high=256M
        mem_max=450M
        oom_score=600
        ;;
    low)
        mem_high=384M
        mem_max=700M
        oom_score=450
        ;;
    high)
        mem_high=1G
        mem_max=3G
        oom_score=200
        ;;
    *)
        mem_high=768M
        mem_max=1536M
        oom_score=300
        ;;
    esac

    # Rewrite memory guidance in place if keys exist; otherwise append under [Service].
    if grep -q '^MemoryHigh=' "$unit"; then
        sed -i "s/^MemoryHigh=.*/MemoryHigh=${mem_high}/" "$unit"
    else
        sed -i "/^\[Service\]/a MemoryHigh=${mem_high}" "$unit"
    fi
    if grep -q '^MemoryMax=' "$unit"; then
        sed -i "s/^MemoryMax=.*/MemoryMax=${mem_max}/" "$unit"
    else
        sed -i "/^\[Service\]/a MemoryMax=${mem_max}" "$unit"
    fi
    if grep -q '^OOMScoreAdjust=' "$unit"; then
        sed -i "s/^OOMScoreAdjust=.*/OOMScoreAdjust=${oom_score}/" "$unit"
    else
        sed -i "/^\[Service\]/a OOMScoreAdjust=${oom_score}" "$unit"
    fi

    # Low-memory environments: prefer fewer FDs only if tiny
    if [[ "$PROFILE" == "tiny" ]]; then
        if grep -q '^LimitNOFILE=' "$unit"; then
            sed -i "s/^LimitNOFILE=.*/LimitNOFILE=65535/" "$unit"
        fi
    fi

    # Drop-in env for GOGC on tiny/low hosts to reduce heap pressure a bit.
    mkdir -p /etc/systemd/system/s-ui.service.d
    case "$PROFILE" in
    tiny)
        cat >/etc/systemd/system/s-ui.service.d/optimize.conf <<EOF
[Service]
Environment=GOGC=40
Environment=GOMEMLIMIT=350MiB
EOF
        ;;
    low)
        cat >/etc/systemd/system/s-ui.service.d/optimize.conf <<EOF
[Service]
Environment=GOGC=50
Environment=GOMEMLIMIT=550MiB
EOF
        ;;
    high)
        cat >/etc/systemd/system/s-ui.service.d/optimize.conf <<EOF
[Service]
Environment=GOGC=100
EOF
        ;;
    *)
        cat >/etc/systemd/system/s-ui.service.d/optimize.conf <<EOF
[Service]
Environment=GOGC=75
EOF
        ;;
    esac

    echo -e "${green}已按档位 ${PROFILE} 优化 systemd：MemoryHigh=${mem_high} MemoryMax=${mem_max}${plain}"
}

suggest_swap() {
    if [[ "$SWAP_MB" -lt 256 && ( "$PROFILE" == "tiny" || "$PROFILE" == "low" ) ]]; then
        echo -e "${yellow}建议添加 1G Swap 以降低 OOM 风险，例如：${plain}"
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

install_s-ui() {
    cd /tmp/ || exit 1

    local last_version
    if [[ -z "$REQUESTED_VERSION" ]]; then
        last_version=$(curl -Ls "https://api.github.com/repos/Hhz0823/1s-ui/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
        if [[ ! -n "$last_version" ]]; then
            echo -e "${red}获取 s-ui 版本失败，可能是 Github API 限制导致，请稍后重试${plain}"
            exit 1
        fi
        echo -e "已获取 s-ui 最新版本：${last_version}，开始安装..."
        wget -N --no-check-certificate -O /tmp/s-ui-linux-$(arch).tar.gz "https://github.com/Hhz0823/1s-ui/releases/download/${last_version}/s-ui-linux-$(arch).tar.gz"
        if [[ $? -ne 0 ]]; then
            echo -e "${red}下载 s-ui 失败，请确认服务器可以访问 Github ${plain}"
            exit 1
        fi
    else
        last_version="$REQUESTED_VERSION"
        [[ "${last_version}" != v* ]] && last_version="v${last_version}"
        local url="https://github.com/Hhz0823/1s-ui/releases/download/${last_version}/s-ui-linux-$(arch).tar.gz"
        echo -e "开始安装 s-ui ${last_version}"
        wget -N --no-check-certificate -O /tmp/s-ui-linux-$(arch).tar.gz "${url}"
        if [[ $? -ne 0 ]]; then
            echo -e "${red}下载 s-ui ${last_version} 失败，请检查该版本是否存在${plain}"
            exit 1
        fi
    fi

    if [[ -e /usr/local/s-ui/ ]]; then
        systemctl stop s-ui 2>/dev/null || true
    fi

    tar zxvf "s-ui-linux-$(arch).tar.gz"
    rm -f "s-ui-linux-$(arch).tar.gz"

    chmod +x s-ui/sui s-ui/s-ui.sh
    cp s-ui/s-ui.sh /usr/bin/s-ui
    cp -rf s-ui /usr/local/
    cp -f s-ui/*.service /etc/systemd/system/
    rm -rf s-ui

    apply_systemd_optimize
    config_after_install
    prepare_services
    install_xray || echo -e "${yellow}Xray-core 未安装；Sing-Box 功能不受影响${plain}"

    systemctl daemon-reload
    systemctl enable s-ui
    if ! systemctl start s-ui; then
        echo -e "${red}s-ui 服务启动失败，最近日志：${plain}"
        journalctl -u s-ui -n 50 --no-pager || true
        echo -e "${yellow}可执行: systemctl status s-ui -l  与  journalctl -u s-ui -xe${plain}"
        exit 1
    fi
    sleep 1
    if ! systemctl is-active --quiet s-ui; then
        echo -e "${red}s-ui 未能保持运行状态，最近日志：${plain}"
        journalctl -u s-ui -n 50 --no-pager || true
        exit 1
    fi

    echo -e "${green}s-ui ${last_version}${plain} 安装完成，现已启动并运行..."
    echo -e "安装摘要：模式=${INSTALL_MODE} 档位=${PROFILE} 面板=是 Xray=$([ "$INSTALL_XRAY" -eq 1 ] && echo 是 || echo 否)"
    echo -e "你可以通过以下 URL 访问面板：${green}"
    /usr/local/s-ui/sui uri 2>/dev/null || true
    echo -e "${plain}"
    suggest_swap
    echo -e ""
    echo -e "${yellow}若 SSH 断开或机器异常，请查 OOM： dmesg | grep -i oom | tail${plain}"
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
