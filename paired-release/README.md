# IPv4/IPv6 配对中转交付文件

这些文件是基于本仓库源码构建的 IPv4/IPv6 配对和双栈回退中转版本。

## 直接安装

以 root 身份执行一行在线安装命令：

```bash
curl -fsSL https://raw.githubusercontent.com/925345845/s-ui/main/paired-release/install-s-ui-paired-online.sh | S_UI_PAIRED_VERSION=v1.5.11 bash
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

安装后进入“入站管理 -> 一键中转 -> 双栈出口”。每行上游 SOCKS5 按顺序对应一条 VPS IPv6。双栈目标会同时尝试 VPS IPv6 和同一行的 IPv4 SOCKS5，哪个连接先成功就使用哪个；单栈目标仍只使用对应地址族。完整配置说明见 `1s-ui-paired-relay-guide.md`。

Apple ID 登录域名 `appleid.apple.com`、`idmsa.apple.com`、`gsa.apple.com` 会自动固定走同一行的 IPv4 SOCKS5，其他域名继续使用双栈竞速。

在“中转批次”中，每条 IPv6 都有独立的手动轮转链接。访问一条链接只更换对应 IPv6，不改变其他地址、现有端口、账号、密码或 IPv4 SOCKS5 配对。

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
