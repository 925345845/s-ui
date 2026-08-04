# 1S-UI IPv4/IPv6 配对中转

此补丁为 1S-UI 增加第三种中转模式：`paired`。

每一条入口代理对应：

- 一个上游 IPv4 SOCKS5 出口；
- 一个绑定到 VPS 网卡的 IPv6 出口；
- IPv4-only 目标走 SOCKS5；
- IPv6-only 或双栈目标优先走对应 VPS IPv6。

## 直接安装

先在 VPS 执行 `uname -m`。下载通用安装脚本，并按架构下载一个安装包，上传到 VPS 的同一目录：

- `install-s-ui-paired.sh`
- `x86_64`：`s-ui-linux-amd64-paired.tar.gz`
- `aarch64`：`s-ui-linux-arm64-paired.tar.gz`

然后执行：

```bash
chmod +x install-s-ui-paired.sh
sudo ./install-s-ui-paired.sh
```

安装脚本支持全新安装和覆盖现有 1S-UI。现有数据库会保留在 `/usr/local/s-ui/db`，原面板二进制会先备份；新版本启动失败时自动恢复。

本地构建包支持 `amd64/x86_64` 和 `arm64/aarch64` Linux VPS，并省略与配对功能无关的 Naive 出站。使用 Naive 节点或其他 CPU 架构时，请使用下方完整源码包通过仓库 GitHub Actions 构建。

## 应用补丁

在 1S-UI 源码仓库执行：

```bash
git apply --check 1s-ui-ipv4-ipv6-paired.patch
git apply 1s-ui-ipv4-ipv6-paired.patch
```

然后按仓库现有发布流程重新构建前端和 Linux 二进制。建议在自己的 fork 中提交改动并使用仓库 GitHub Actions 生成 Release。

## 面板使用

1. 打开“入站管理 -> 一键中转 -> IPv4/IPv6 配对”。
2. 选择 VPS 的公网 IPv6 网卡，必要时填写已路由 IPv6 前缀。
3. 将 IPWO 接口返回的文本粘贴到“上游列表”。支持：

```text
203.0.113.10:1080
203.0.113.11:1080:user:password
socks5://user:password@203.0.113.12:1080
```

4. 上游列表有多少行，就会按顺序生成多少个 IPv6 和入口端口。
5. 创建成功后复制面板导出的 `VPS地址:端口:账号:密码`。

## 验证

对同一个入口分别访问 IPv4-only 和 IPv6-only 查询地址：

```bash
curl --socks5-hostname VPS地址:端口 --proxy-user 账号:密码 https://api.ipify.org
curl --socks5-hostname VPS地址:端口 --proxy-user 账号:密码 https://api6.ipify.org
```

第一条应显示该入口对应的上游 SOCKS5 IPv4；第二条应显示该入口对应的 VPS IPv6。

## 限制

- VPS 必须拥有服务商实际路由或授权的 IPv6 前缀。
- 上游 SOCKS5 的 UDP 支持取决于代理供应商；浏览器 QUIC 可能回退到 TCP。
- 域名先由 VPS 本地 DNS 解析再按地址族分流，因此 CDN 节点选择可能反映 VPS DNS 所在地，而不是上游 IPv4 代理所在地。
