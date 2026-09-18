# IPv4/IPv6 配对中转交付文件

本补丁版本为 **v1.5.20-ipv6fix.5**，直接基于 **v1.5.20**（`f4a0d051473abe38404e51d14df41ac37dfed150`）构建，不包含 v1.5.21 及以后版本的功能、依赖升级或数据库变更。

本次优化：

- IPv6 添加、地址就绪、IPv6 出口或配对 IPv4 SOCKS5 检测失败时，只跳过原行，继续检查其他条目。成功条目保留原来的 IPv4/IPv6 配对。
- 只为通过检测的条目创建入站、出站、用户和路由，批次数量及导出数量按实际成功数计算。全部失败时不创建空批次。
- 创建结果显示请求、成功及跳过数量，并列出失败条目的原行号、IPv6、阶段与原因，可复制报告。失败报告仅保留在本次创建结果中，请在刷新页面前复制。
- 清理失败条目时只删除本次新增 IPv6，保留已存在的系统地址。请求取消、数据库或代理配置错误仍回滚未提交的整个批次。
- “Apple ID 专用 IPv4，其余仅 IPv6”仍默认开启，保留 v1.5.20 的业务域名分流规则。

保留此前独立补丁的地址生成、/128 绑定和网络检测优化：地址池仍按供应商的 /64 分配，额外地址按 /128 绑定；最多 4 个地址并行添加（阶段时限 30 秒），批量等待 DAD（10 秒）后等待 2 秒网络稳定。IPv6 检测最多 16 条并发，IPv4 上游检测最多 8 条并发，各阶段时限 45 秒；超时未通过的行会跳过。配置保存及内核启动时间另计。

检测成功仅代表检测时连接可用，不能保证地址长期在线或所有网站可用。供应商已确认整个 /64 路由到 VPS 时，无需逐个登记地址或在 VPS 配置 NDP 代理。

可在“指定 IPv6”中每行粘贴一个现成地址（不带 /128），与同数量 IPv4 SOCKS5 按原行配对；填写地址池前缀 64。列表不足时仍会随机补齐。外部脚本添加的地址不会自动转为面板所有，重启后的恢复需由原网络配置负责。

请使用本页安装命令，或本补丁 Release 的附件安装。仓库内的历史交付文件不作为本次补丁的安装包。

## 直接安装

以 root 身份执行一行在线安装命令：

```bash
curl -fsSL https://raw.githubusercontent.com/925345845/s-ui/v1.5.20-ipv6fix.5/paired-release/install-s-ui-paired-online.sh | bash
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

安装后进入“入站管理 -> 一键中转 -> 双栈出口”。每行上游 SOCKS5 按顺序对应一条 VPS IPv6。未开启专用 IPv4 分流时，双栈目标优先使用 VPS IPv6，IPv6 连接失败才回退同一行 IPv4 SOCKS5。完整配置说明见 `1s-ui-paired-relay-guide.md`。

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
