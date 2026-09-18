# IPv4/IPv6 配对中转交付文件

本补丁版本为 **v1.5.20-ipv6fix.7**，直接基于 **v1.5.20**（`f4a0d051473abe38404e51d14df41ac37dfed150`）构建，不包含 v1.5.21 及以后版本的功能、依赖升级或数据库变更。

本次优化：

- “创建前检测公网连通性”默认开启，API 未指定该选项时也默认检测。失败 IPv6 自动排除，不创建为节点。
- IPv6 候选与 IPv4 SOCKS5 上游独立检测，通过后分别按输入顺序匹配。例如仅第 5 个 IPv6 可用，它会匹配第一条检测通过的 IPv4，不会丢弃前四条 IPv4。
- 可用数量不足时仅创建实际可配对的条目，不重复复用同一个 IPv6，也不无限随机重试。结果显示未创建的 IPv4 原行号和各阶段失败记录，支持复制；已创建条目保存双方原行号。
- 本次新加的失败 IPv6 以及没有匹配到上游的新 IPv6 会清理。外部脚本添加的既有地址不删除；取消、数据库或代理配置错误仍回滚本次未提交资源。
- 保留 v1.5.20 的“Apple ID 专用 IPv4，其余仅 IPv6”，默认开启。手动关闭公网检测仍可直接创建供业务网站测试，批次标注未检测。

检测时间：IPv6 两个 TCP 目标并行，每轮最多 8 秒，三轮间隔 3 秒、5 秒，每个地址完整检测最多约 32 秒；最多 16 个地址并发。IPv6 阶段总预算为 `ceil(数量/16) × 36 秒 + 30 秒`，不再使用整批 45 秒截断。500 个候选预算约 19 分 42 秒，100 个约 4 分 42 秒，成功会提前结束。

IPv4 最多 8 条并发，阶段预算为 `ceil(数量/8) × 12 秒 + 30 秒`；500 条约 13 分 6 秒。远程创建及内部控制连接等待时间增加至 45 分钟，远程使用需同步升级 Agent。本地地址添加阶段 30 秒、DAD 阶段 10 秒，保存及内核启动耗时另计。外部反向代理的超时设置仍需允许长请求。

指定 IPv6 时每行填一个纯地址，不带 /128，地址池前缀填 64；留空或数量不足时生成候选补齐。地址实际按 /128 绑定。检测目标可达不保证所有业务网站可用；现有批次不会自动删除或重新配对。

请使用本页安装命令，或本补丁 Release 的附件安装。仓库内的历史交付文件不作为本次补丁的安装包。

## 直接安装

以 root 身份执行一行在线安装命令：

```bash
curl -fsSL https://raw.githubusercontent.com/925345845/s-ui/v1.5.20-ipv6fix.7/paired-release/install-s-ui-paired-online.sh | bash
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
