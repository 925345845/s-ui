#!/usr/bin/env bash
set -euo pipefail

case "$(uname -m)" in
  x86_64|amd64)
    package_arch="amd64"
    archive_sha256="952f192593285a1944d79ebb4bb4d56eea44f3a306a5bceaef1106d8695db7ad"
    ;;
  aarch64|arm64)
    package_arch="arm64"
    archive_sha256="0ea8ea69ebea3d232516ee5a4afbcd7593dcc215851065f7292b538b06a02489"
    ;;
  *)
    echo "不支持当前架构：$(uname -m)" >&2
    exit 1
    ;;
esac

release_ref="${S_UI_VINTED_STRICT_REF:-installer-vinted-strict-test-6}"
base_url="${S_UI_VINTED_STRICT_BASE_URL:-https://raw.githubusercontent.com/925345845/s-ui/${release_ref}/paired-release}"
release_version="${S_UI_VINTED_STRICT_VERSION:-v1.5.28-socks5-500-vinted-strict-test-6}"
archive_base_url="${S_UI_VINTED_STRICT_ARCHIVE_BASE_URL:-https://github.com/925345845/s-ui/releases/download/${release_version}}"
tmp_dir="$(mktemp -d /tmp/s-ui-vinted-strict-online.XXXXXX)"
cleanup() {
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT

installer="$tmp_dir/install-s-ui-vinted-strict.sh"
rollback="$tmp_dir/rollback-s-ui-vinted-strict.sh"
archive="$tmp_dir/s-ui-linux-${package_arch}.tar.gz"

echo "正在下载 ${package_arch} Vinted 严格会话测试版..."
curl -fL --retry 3 --connect-timeout 15 \
  "$base_url/install-s-ui-vinted-strict.sh" \
  -o "$installer"
curl -fL --retry 3 --connect-timeout 15 \
  "$base_url/rollback-s-ui-vinted-strict.sh" \
  -o "$rollback"
curl -fL --retry 3 --connect-timeout 15 \
  "$archive_base_url/s-ui-linux-${package_arch}.tar.gz" \
  -o "$archive"

printf '%s  %s\n' "$archive_sha256" "$archive" | sha256sum -c -
chmod +x "$installer"
"$installer" "$archive"
install -d -m 755 /usr/local/s-ui/bin
install -m 755 "$rollback" /usr/local/s-ui/bin/rollback-s-ui-vinted-strict
echo "恢复安装前版本：sudo /usr/local/s-ui/bin/rollback-s-ui-vinted-strict"
