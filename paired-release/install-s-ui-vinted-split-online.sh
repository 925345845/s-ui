#!/usr/bin/env bash
set -euo pipefail

case "$(uname -m)" in
  x86_64|amd64)
    package_arch="amd64"
    archive_sha256="69d5f50f22dfea516e19c1b98ca039ff46343f151210d090109f50575224d328"
    ;;
  aarch64|arm64)
    package_arch="arm64"
    archive_sha256="289f6fbf5d5522e1cefd1f546f179fcb18966ebec36b7e164855a1f4753276a2"
    ;;
  *)
    echo "不支持当前架构：$(uname -m)" >&2
    exit 1
    ;;
esac

release_branch="${S_UI_VINTED_SPLIT_BRANCH:-vinted-domain-split-experimental}"
base_url="${S_UI_VINTED_SPLIT_BASE_URL:-https://raw.githubusercontent.com/925345845/s-ui/${release_branch}/paired-release}"
release_version="${S_UI_VINTED_SPLIT_VERSION:-v1.5.28-socks5-500-vinted-split-test-1}"
archive_base_url="${S_UI_VINTED_SPLIT_ARCHIVE_BASE_URL:-https://github.com/925345845/s-ui/releases/download/${release_version}}"
tmp_dir="$(mktemp -d /tmp/s-ui-vinted-split-online.XXXXXX)"
cleanup() {
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT

installer="$tmp_dir/install-s-ui-vinted-split.sh"
rollback="$tmp_dir/rollback-s-ui-vinted-split.sh"
archive="$tmp_dir/s-ui-linux-${package_arch}-vinted-split-test.tar.gz"

echo "正在下载 ${package_arch} Vinted 分流测试版..."
curl -fL --retry 3 --connect-timeout 15 \
  "$base_url/install-s-ui-vinted-split.sh" \
  -o "$installer"
curl -fL --retry 3 --connect-timeout 15 \
  "$base_url/rollback-s-ui-vinted-split.sh" \
  -o "$rollback"
curl -fL --retry 3 --connect-timeout 15 \
  "$archive_base_url/s-ui-linux-${package_arch}-vinted-split-test.tar.gz" \
  -o "$archive"

printf '%s  %s\n' "$archive_sha256" "$archive" | sha256sum -c -
chmod +x "$installer"
"$installer" "$archive"
install -d -m 755 /usr/local/s-ui/bin
install -m 755 "$rollback" /usr/local/s-ui/bin/rollback-s-ui-vinted-split
echo "恢复安装前版本：sudo /usr/local/s-ui/bin/rollback-s-ui-vinted-split"
