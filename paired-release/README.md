# IPv4/IPv6 配对中转交付文件

本补丁版本为 **v1.5.20-ipv6fix.3**，直接基于 **v1.5.20**（`f4a0d051473abe38404e51d14df41ac37dfed150`）构建，不包含 v1.5.21 及以后版本的功能、依赖升级或数据库变更。

本次优化：

- 按上游 SOCKS5 条数生成同样数量、前缀内不重复的 IPv6；自动生成时避开 VPS 已绑定地址。显式指定地址仍按原规则使用。
- 先在 VPS 上准备整个 IPv6 池（最多 4 个地址同时添加），统一等待 DAD 和 2 秒网络稳定，再验证 IPv6/IPv4，最后统一创建。失败只清理本次实际新增地址。
- 界面显示“上游数量 → IPv6 数量”的计划，修复创建阶段提示的语言资源位置。

- IPv6 两个 TCP 目标并行检测，任一成功即可通过。每轮最多 5 秒，失败后间隔 1 秒、2 秒重试，单个地址最多约 18 秒。
- 同时检测最多 16 个 IPv6（最多 32 个 TCP 连接），整批 IPv6 检测总时限 45 秒，超时后停止排队并回滚。地址添加与 DAD 准备阶段总时限 30 秒。
- 配对/双栈批次逐条验证 IPv4 SOCKS5 的认证及 IPv4 CONNECT，最多 8 条并发，整个 IPv4 检测阶段总时限 45 秒。失败显示上游行号，不返回账号密码。
- 本机创建显示实际阶段、完成数量与耗时；阶段变化也写入日志。重复创建立即提示操作正在进行，避免请求排队。
- 新建表单恢复默认开启“Apple ID 专用 IPv4，其余仅 IPv6”，保留 v1.5.20 的业务域名分流规则。
- 修复进度刷新导致的长时间加载显示，并在打开数据库写事务前取得旧配置快照。

网络检测有时限，但保存配置及代理服务启动耗时取决于服务器资源。检测成功代表创建时的 TCP 可达性，不能保证上游长期在线或所有网站可用。

请使用本页在线命令，或本补丁 Release 的附件安装；仓库内历史打包文件不作为本次补丁的安装包。

## 直接安装

以 root 身份执行一行在线安装命令：

```bash
curl -fsSL https://raw.githubusercontent.com/925345845/s-ui/v1.5.20-ipv6fix.3/paired-release/install-s-ui-paired-online.sh | bash
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
