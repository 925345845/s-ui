# IPv4/IPv6 配对中转交付文件

这些文件是基于本仓库源码构建的 IPv4/IPv6 配对和双栈回退中转版本。

## 直接安装

以 root 身份执行一行在线安装命令：

```bash
curl -fsSL https://raw.githubusercontent.com/925345845/s-ui/main/paired-release/install-s-ui-paired-online.sh | S_UI_PAIRED_VERSION=v1.5.22 bash
```

脚本会自动识别 amd64/arm64、下载对应安装包并调用本地安装脚本。

也可以手动下载。先在 VPS 上下载 `install-s-ui-paired.sh`，并根据 `uname -m` 再下载对应的压缩包到同一目录：

- `x86_64` / `amd64`: `s-ui-linux-amd64-paired.tar.gz`
- `aarch64` / `arm64`: `s-ui-linux-arm64-paired.tar.gz`

执行：

```bash
chmod +x install-s-ui-paired.sh
sudo ./install-s-ui-paired.sh
```

安装后进入“入站管理 -> 一键中转 -> 双栈出口”。上游列表支持常见 SOCKS5 文本格式（`host:port`、`host:port:user:pass`、`user:pass@host:port`、`host:port@user:pass`、`socks5://...`/`socks5h://...`、逗号/竖线/空格分隔的四字段）以及常见 JSON 数组或对象格式。每条上游按顺序对应一条 VPS IPv6。双栈目标会同时尝试 VPS IPv6 和同一行的 IPv4 SOCKS5，哪个连接先成功就使用哪个；单栈目标仍只使用对应地址族。HTTP/HTTPS 上游不会被当作 SOCKS5 接受。完整配置说明见 `1s-ui-paired-relay-guide.md`。

创建配对或双栈批次时开启“Apple ID 专用 IPv4，其余仅 IPv6”，即可让 `appleid.apple.com`、`idmsa.apple.com`、`gsa.apple.com` 固定走同一行 IPv4 SOCKS5，其他域名默认仅使用 VPS IPv6。

在“中转批次”中，每条 IPv6 都有独立的手动轮转链接。访问一条链接只更换对应 IPv6，不改变其他地址、现有端口、账号、密码或 IPv4 SOCKS5 配对。

项目创建的 IPv6 会随中转批次保存在数据库中。VPS 重启或供应商网络重置导致地址从网卡消失时，面板会在启动时恢复，并每分钟自动校验补回；短暂的网络或 DAD 未就绪不会再删除已保存地址。升级前已有的中转批次无需逐条重新录入。

配对/双栈中转最多支持 500 条上游代理；每条上游会生成一个独立入口和对应出口。500 条批次会生成较大的 sing-box 配置，建议在创建前确认 VPS 内存至少 2 GiB，并确保起始端口加 499 不超过 65535。

导出 BitBrowser 批量导入文件时，每条代理自己的完整轮转链接会写入同一行的“窗口备注”（J 列），并可直接点击。继续使用 BitBrowser 原有标准列，不增加可能影响批量导入的自定义字段。

## 文件说明

- `install-s-ui-paired.sh`: 自动识别 VPS 架构并安装。
- `install-s-ui-paired-online.sh`: 自动下载对应架构安装包并安装。
- `s-ui-linux-amd64-paired.tar.gz`: Linux amd64 预编译包。
- `s-ui-linux-arm64-paired.tar.gz`: Linux arm64 预编译包。
- `1s-ui-ipv4-ipv6-paired.patch`: 应用到原始源码的 Git 补丁。
- `1s-ui-paired-complete-source.zip`: 已集成配对功能的完整源码。
- `1s-ui-paired-replacement-files.zip`: 仅包含修改过的源码文件。
- `SHA256SUMS.txt`: SHA256 校验值。
