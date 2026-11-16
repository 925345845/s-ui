#!/usr/bin/env bash
set -euo pipefail

case "$(uname -m)" in
  x86_64|amd64)
    package_arch="amd64"
    archive_sha256="f28014f7322f04d8a908afff6a2844e761cf23bbea95dff0d8a0d02d1248857a"
    ;;
  aarch64|arm64)
    package_arch="arm64"
    archive_sha256="9dcc355e2058755f412c5b53a908e72b9eb134f7011a94bbaa2e2b330244c7bb"
    ;;
  *)
    echo "不支持当前架构：$(uname -m)" >&2
    exit 1
    ;;
esac

release_branch="${S_UI_IPV6_FILL_BRANCH:-socks5-upstream-flexible-500}"
base_url="${S_UI_IPV6_FILL_BASE_URL:-https://raw.githubusercontent.com/925345845/s-ui/${release_branch}/paired-release}"
release_version="${S_UI_IPV6_FILL_VERSION:-v1.5.28-socks5-500-ipv6-fill-2}"
archive_base_url="${S_UI_IPV6_FILL_ARCHIVE_BASE_URL:-https://github.com/925345845/s-ui/releases/download/${release_version}}"
tmp_dir="$(mktemp -d /tmp/s-ui-ipv6-fill-online.XXXXXX)"
cleanup() {
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT

installer="$tmp_dir/install-s-ui-ipv6-fill.sh"
rollback="$tmp_dir/rollback-s-ui-ipv6-fill.sh"
archive="$tmp_dir/s-ui-linux-${package_arch}-ipv6-fill-v2.tar.gz"

echo "正在下载 ${package_arch} 成功保留/自动补齐版..."
curl -fL --retry 3 --connect-timeout 15 \
  "$base_url/install-s-ui-ipv6-fill.sh" \
  -o "$installer"
curl -fL --retry 3 --connect-timeout 15 \
  "$base_url/rollback-s-ui-ipv6-fill.sh" \
  -o "$rollback"
curl -fL --retry 3 --connect-timeout 15 \
  "$archive_base_url/s-ui-linux-${package_arch}-ipv6-fill-v2.tar.gz" \
  -o "$archive"

printf '%s  %s\n' "$archive_sha256" "$archive" | sha256sum -c -
chmod +x "$installer"
"$installer" "$archive"
install -d -m 755 /usr/local/s-ui/bin
install -m 755 "$rollback" /usr/local/s-ui/bin/rollback-s-ui-ipv6-fill
echo "回退命令：sudo /usr/local/s-ui/bin/rollback-s-ui-ipv6-fill"
