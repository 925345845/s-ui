#!/usr/bin/env bash
set -euo pipefail

case "$(uname -m)" in
  x86_64|amd64)
    package_arch="amd64"
    archive_sha256="0d59498d272ec7dd3309fd5ec471472753354fd0138febbd5b365f4a09a0732e"
    ;;
  aarch64|arm64)
    package_arch="arm64"
    archive_sha256="2ba4fcac40c026b28de95956583799630b00ae5d84159cb7103275dd64cc8f10"
    ;;
  *)
    echo "不支持当前架构：$(uname -m)" >&2
    exit 1
    ;;
esac

release_branch="${S_UI_IPV6_FILL_BRANCH:-socks5-upstream-flexible-500}"
base_url="${S_UI_IPV6_FILL_BASE_URL:-https://raw.githubusercontent.com/925345845/s-ui/${release_branch}/paired-release}"
release_version="${S_UI_IPV6_FILL_VERSION:-v1.5.28-socks5-500-ipv6-fill-1}"
archive_base_url="${S_UI_IPV6_FILL_ARCHIVE_BASE_URL:-https://github.com/925345845/s-ui/releases/download/${release_version}}"
tmp_dir="$(mktemp -d /tmp/s-ui-ipv6-fill-online.XXXXXX)"
cleanup() {
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT

installer="$tmp_dir/install-s-ui-ipv6-fill.sh"
archive="$tmp_dir/s-ui-linux-${package_arch}-ipv6-fill.tar.gz"

echo "正在下载 ${package_arch} 成功保留/自动补齐版..."
curl -fL --retry 3 --connect-timeout 15 \
  "$base_url/install-s-ui-ipv6-fill.sh" \
  -o "$installer"
curl -fL --retry 3 --connect-timeout 15 \
  "$archive_base_url/s-ui-linux-${package_arch}-ipv6-fill.tar.gz" \
  -o "$archive"

printf '%s  %s\n' "$archive_sha256" "$archive" | sha256sum -c -
chmod +x "$installer"
"$installer" "$archive"
