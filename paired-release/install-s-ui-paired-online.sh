#!/usr/bin/env bash
set -euo pipefail

case "$(uname -m)" in
  x86_64|amd64) package_arch="amd64" ;;
  aarch64|arm64) package_arch="arm64" ;;
  *)
    echo "不支持当前架构：$(uname -m)" >&2
    exit 1
    ;;
esac

base_url="${S_UI_PAIRED_BASE_URL:-https://raw.githubusercontent.com/925345845/s-ui/main/paired-release}"
tmp_dir="$(mktemp -d /tmp/1s-ui-paired-online.XXXXXX)"
cleanup() {
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT

installer="$tmp_dir/install-s-ui-paired.sh"
archive="$tmp_dir/s-ui-linux-${package_arch}-paired.tar.gz"

echo "正在下载 ${package_arch} 安装包..."
curl -fL --retry 3 --connect-timeout 15 \
  "$base_url/install-s-ui-paired.sh" \
  -o "$installer"
curl -fL --retry 3 --connect-timeout 15 \
  "$base_url/s-ui-linux-${package_arch}-paired.tar.gz" \
  -o "$archive"

chmod +x "$installer"
"$installer" "$archive"
