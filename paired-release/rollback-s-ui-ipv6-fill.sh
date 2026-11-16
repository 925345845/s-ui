#!/usr/bin/env bash
set -euo pipefail

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "请使用 root 或 sudo 运行。" >&2
  exit 1
fi

backup_dir="/usr/local/s-ui/backups/ipv6-fill"
marker="$backup_dir/latest"
if [[ ! -f "$marker" ]]; then
  echo "未找到回退记录：$marker" >&2
  exit 1
fi

backup="$(head -n 1 "$marker")"
case "$backup" in
  "$backup_dir"/sui.before-ipv6-fill.*) ;;
  *)
    echo "回退记录路径无效。" >&2
    exit 1
    ;;
esac
if [[ ! -f "$backup" ]]; then
  echo "备份文件不存在：$backup" >&2
  exit 1
fi

systemctl stop s-ui 2>/dev/null || true
cp -p "$backup" /usr/local/s-ui/sui
if ! systemctl restart s-ui; then
  echo "已恢复二进制，但 s-ui 启动失败；请检查 journalctl -u s-ui。" >&2
  exit 1
fi
echo "已恢复安装前版本：$backup"
