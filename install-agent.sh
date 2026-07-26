#!/usr/bin/env bash

set -euo pipefail

repo="Hhz0823/1s-ui"
panel_url=""
token=""
version=""
insecure="false"

usage() {
    echo "Usage: install-agent.sh --panel URL --token TOKEN [--version VERSION] [--insecure]"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --panel)
            panel_url="${2:-}"
            shift 2
            ;;
        --token)
            token="${2:-}"
            shift 2
            ;;
        --version)
            version="${2:-}"
            shift 2
            ;;
        --insecure)
            insecure="true"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

if [[ ${EUID} -ne 0 ]]; then
    echo "Run this installer as root." >&2
    exit 1
fi
if [[ "$(uname -s)" != "Linux" ]] || ! command -v systemctl >/dev/null 2>&1; then
    echo "The 1S-UI Agent installer requires Linux with systemd." >&2
    exit 1
fi
if [[ ! "$panel_url" =~ ^https?://[^[:space:]\'\"\\]+$ ]]; then
    echo "Invalid panel URL." >&2
    exit 1
fi
if [[ ! "$token" =~ ^[A-Za-z0-9_-]{32,128}$ ]]; then
    echo "Invalid enrollment token." >&2
    exit 1
fi

case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    i386|i486|i586|i686) arch="386" ;;
    aarch64|arm64) arch="arm64" ;;
    armv7*|armhf) arch="armv7" ;;
    armv6*) arch="armv6" ;;
    armv5*) arch="armv5" ;;
    s390x) arch="s390x" ;;
    *)
        echo "Unsupported architecture: $(uname -m)" >&2
        exit 1
        ;;
esac

if [[ -z "$version" ]]; then
    version=$(curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" | sed -nE 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/p' | head -n1)
fi
[[ "$version" == v* ]] || version="v${version}"
if [[ ! "$version" =~ ^v[0-9A-Za-z._-]+$ ]]; then
    echo "Invalid release version." >&2
    exit 1
fi

tmp_dir=$(mktemp -d /tmp/1s-ui-agent.XXXXXX)
trap 'rm -rf "$tmp_dir"' EXIT
archive="$tmp_dir/s-ui.tar.gz"
url="https://github.com/${repo}/releases/download/${version}/s-ui-linux-${arch}.tar.gz"

echo "Downloading 1S-UI Agent ${version} for ${arch}..."
curl --fail --location --retry 3 --output "$archive" "$url"
tar -xzf "$archive" -C "$tmp_dir" s-ui/sui-agent s-ui/s-ui-agent.service

install -d -m 0755 /usr/local/s-ui /etc/default /etc/systemd/system
install -m 0755 "$tmp_dir/s-ui/sui-agent" /usr/local/s-ui/sui-agent
install -m 0644 "$tmp_dir/s-ui/s-ui-agent.service" /etc/systemd/system/s-ui-agent.service
umask 077
{
    printf 'SUI_AGENT_PANEL=%s\n' "$panel_url"
    printf 'SUI_AGENT_TOKEN=%s\n' "$token"
    printf 'SUI_AGENT_INTERVAL=15s\n'
    printf 'SUI_AGENT_INSECURE=%s\n' "$insecure"
} > /etc/default/1s-ui-agent

echo "Validating the panel connection..."
/usr/local/s-ui/sui-agent --once
systemctl daemon-reload
systemctl enable --now s-ui-agent
systemctl is-active --quiet s-ui-agent
echo "1S-UI Agent is connected and running."
