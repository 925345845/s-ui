# IPv4/IPv6 配对中转交付文件

这些文件是基于本仓库源码构建的 IPv4/IPv6 配对和双栈回退中转版本。

## 直接安装

以 root 身份执行一行在线安装命令：

```bash
curl -fsSL https://raw.githubusercontent.com/925345845/s-ui/main/paired-release/install-s-ui-paired-online.sh | bash
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

安装后进入“入站管理 -> 一键中转 -> 双栈出口”。每行上游 SOCKS5 按顺序对应一条 VPS IPv6。所有目标先尝试 VPS IPv6，连接失败后回退到同一行的 IPv4 SOCKS5。完整配置说明见 `1s-ui-paired-relay-guide.md`。

在“中转批次”中可点击同步图标立即轮转 IPv6，或开启定时轮转并设置 5–10080 分钟间隔。轮转不会改变现有端口、账号、密码或 IPv4 SOCKS5 配对。

## 文件说明

- `install-s-ui-paired.sh`: 自动识别 VPS 架构并安装。
- `install-s-ui-paired-online.sh`: 自动下载对应架构安装包并安装。
- `s-ui-linux-amd64-paired.tar.gz`: Linux amd64 预编译包。
- `s-ui-linux-arm64-paired.tar.gz`: Linux arm64 预编译包。
- `1s-ui-ipv4-ipv6-paired.patch`: 应用到原始源码的 Git 补丁。
- `1s-ui-paired-complete-source.zip`: 已集成配对功能的完整源码。
- `1s-ui-paired-replacement-files.zip`: 仅包含修改过的源码文件。
- `SHA256SUMS.txt`: SHA256 校验值。
