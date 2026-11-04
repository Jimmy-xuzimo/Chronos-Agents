# 部署与维护

## Hub

推荐独立域名，例如 `https://hub.example.com`。根地址必须与浏览器访问地址完全一致，包括非默认端口；不支持子路径部署。没有默认管理员账号，首次设置一个管理密码，可选启用 TOTP。

原生部署示例：

```sh
chronos-hub --data /var/lib/chronos-hub --listen 127.0.0.1:17870 \
  --origin https://hub.example.com --trusted-proxy 127.0.0.1/32
```

Nginx / Caddy 在本机终止 TLS，保留正确的 `Host`，设置 `X-Forwarded-Proto: https`。代理必须限制请求体（例如 256 KiB）和连接数。Hub 只信任显式 CIDR 内的直接代理，不以任意转发头绕过 TLS 要求；限流使用直接连接身份，因此代理后的未认证配对／登录限流会共享代理 IP，个人部署可接受，不能当成大规模多租户方案。

也可以不使用代理，传入 `--tls-cert /path/fullchain.pem --tls-key /path/privkey.pem` 直接提供 TLS。私钥必须只对服务用户可读。证书更新后重启服务重新载入。

Docker 健康检查验证本地监听端口，不能代替认证、TLS 和上报链路检查。

Docker：

```sh
export HUB_ORIGIN=https://hub.example.com:17870
# 证书放在 deploy/docker/certs；其私钥不可提交 Git。
docker compose -f deploy/docker/compose.hub.yml up -d --build
```

默认端口仅发布到 `127.0.0.1`。明确需要外部直连 TLS 时才设置 `HUB_BIND_IP` 为指定接口地址，并配防火墙。服务以 UID/GID 10001 运行，证书需可被此身份读取。配置卷为私有数据，不得当公开附件上传。

取出首次凭据（只在自己的终端查看，不贴到工单或截图）：

```sh
docker compose -f deploy/docker/compose.hub.yml exec hub cat /data/bootstrap-secret
```

初始化后该文件删除。找不到文件可能说明已经初始化；不要删除数据库来重置密码。忘记密码时，先备份数据并通过受控维护流程恢复；本版不提供公开免认证密码重置接口。

## Windows

安装包安装到当前用户的 `LocalAppData/Programs/ChronosAgent`，不覆盖旧的 Chronos Monitoring Windows Agent。配置默认在 `%AppData%/ChronosAgents`，设备 Token 由当前用户 DPAPI 加密；不要跨 Windows 用户直接复制凭据目录。

开始菜单打开管理页；若采集器尚未运行，会先启动它。托盘提供打开管理页、暂停和退出。安装时可选择登录自动运行。关闭浏览器不会退出采集器；从托盘退出才会停止。修改自启动可重新运行安装器选择任务，或用 Windows 启动应用设置关闭。正式发布版无控制台窗口。

首次本地管理凭据在上述配置目录的 `bootstrap-secret`。管理页默认仅监听 `127.0.0.1:17860`，不接受公网 HTTP。升级使用 Restart Manager 关闭占用本安装目录程序文件的采集器进程，再替换文件并保留配置；内存补报队列随进程退出清空。不要跳过被占用文件。卸载保留配置以便回退；彻底删除前先在 Hub 撤销设备，再删除该目录。已加密凭据不会进入布局、日志或公开仓库。

## Linux 原生

将对应架构二进制安装到 `/usr/local/bin/chronos-agent`，创建无交互登录的 `chronos-agent` 服务用户，安装 `deploy/systemd/chronos-agent.service`，再 `systemctl enable --now chronos-agent`。服务文件需要系统管理员安装；应用本身不需要 root。

默认管理页为 `http://127.0.0.1:17860`。远程维护可使用自己的 SSH 隧道或受控 HTTPS 反向代理。桌面会话有 D-Bus 时可使用托盘，凭据优先存 Secret Service，需要 `secret-tool`；无桌面服务使用权限 0600 的文件。服务账号、目录权限和磁盘加密由部署者负责。

进程默认关闭。非 root 采集器看不到的进程不提升权限去强行读取。systemd 的 `ProtectHome` / `PrivateDevices` 会进一步限制可见性；如需特定数据卷，应只开放明确的目录，不去掉全部隔离。

## Linux / NAS Docker

```sh
export CHRONOS_VOLUME_PATH=/path/to/existing/data-volume
docker compose -f deploy/docker/compose.agent.yml up -d --build
```

基础模板面向真正的 Linux 宿主：只读挂载 `/proc`、`/sys` 与**明确选择的数据卷**。选定卷只用于容量统计；不要挂载含密钥的完整可写根目录。配置卷单独读写，不挂载 `docker.sock`，不启用 privileged 或 host PID。

模板采用 Linux host 网络，使 `127.0.0.1:17860` 真正只监听 NAS 本机，同时读取主机网卡计数。容器无法用此方法采集 Windows / macOS Docker Desktop 物理主机，后者应安装原生 Agent。

### 使用预构建镜像

Release 如提供对应架构的 `*-image.tar.gz`，可先校验 SHA256，再解压加载：

```sh
gzip -dc chronos-agent-0.1.0-linux-amd64-image.tar.gz | docker load
# 已加载镜像时使用 --no-build，避免在 NAS 重新编译。
docker compose -f deploy/docker/compose.agent.yml up -d --no-build
```

arm64 包的镜像标签为 `chronos-agent:0.1.0-arm64`，设置 `CHRONOS_AGENT_IMAGE` 指向它；Hub 同理使用 `CHRONOS_HUB_IMAGE`。不要将仅构建了 amd64 的包标为多架构镜像。

### 从局域网打开 NAS 管理页

默认 localhost **不能**从另一台电脑直接访问。可选择以下一种明确方式：

1. 使用 SSH 隧道转发 NAS 的 `127.0.0.1:17860`，浏览器仍访问本机转发地址，并让 Agent 的 origin 与该地址完全一致。
2. 使用 NAS 的 HTTPS 反向代理，Agent 参数改为 `--origin https://agent.example.com --trusted-proxy 127.0.0.1/32`，后端仍绑定 loopback；代理设置正确 Host / `X-Forwarded-Proto`，并限制来源网络。
3. 挂载自己的 TLS 证书，把 listen 改为 NAS 的指定 LAN IP 与端口，origin 为对应 HTTPS 名称，增加 `--tls-cert` / `--tls-key`。不要简单将 HTTP 改绑 `0.0.0.0`。

图形管理登录始终存在，反向代理不能代替应用登录。私有 CA 必须由客户端明确导入，不能关闭证书校验。

### 数据范围

`HOST_PROC` 和 `HOST_SYS` 同时配置才允许容器声明宿主基础指标；存储声明 `accessible_storage`，只反映指定卷。宿主地址和主机名在容器模式中可能不可用；进程能读取的范围仍受权限限制。磁盘块设备计数逐设备显示，不盲目汇总父设备与分区。

## macOS

运行完整 `.app` 包，包含同目录下的 Go 程序与 Swift Keychain helper。托盘需 macOS 本机 CGO 编译；交叉编译的 headless 二进制不能冒称完整桌面安装版。Intel 和 Apple Silicon 分别构建。

`deploy/macos/build.sh` 在 macOS 上生成应用；`install.sh` 安装到 `~/Applications` 并可选注册当前用户 LaunchAgent。卸载先停止 LaunchAgent，在 Hub 撤销设备，再移除对应 Keychain 条目和配置目录。

没有发行者证书时产物不含 Developer ID 签名或公证；Gatekeeper 提示属于已知发行限制，不应指导用户全局关闭安全策略。尚未经过 macOS 实机验证时应先测试，不直接替换正式采集器。

## Android

安装 APK 后，在 Hub 创建 Android 设备并手动输入配对码，或导入二维码图片。不会申请相机权限；读取所选图片使用系统文档选择器。

后台周期最短 15 分钟，系统可能延迟；前台实时任务仅在应用可见、已连接且未暂停时运行。计量网络和电池暂停设置也适用于前台上传。锁屏、Doze、厂商省电、强制停止不提供永久保活承诺；强制停止后需重新打开应用。后台同步延迟不等于手机关机。

Token 由 Android Keystore 保护，禁用系统备份。清除数据或密钥失效需重新配对。应用不读取整机其他应用进程、通讯录、定位或短信；缺失指标标为不支持。Debug APK 仅用于测试，正式升级要求相同应用 ID 和签名私钥。

## 备份、恢复与回退

- 停止 Hub 后备份整个私有数据目录，保留文件权限；或使用 SQLite 在线备份 API，不直接复制运行中的单个数据库文件遗漏 WAL。
- 在隔离目录恢复，使用相同 origin；对外开放前检查权限与设备／读者范围。恢复旧备份可能恢复已撤销凭据，必须重做之后的撤销操作并审查审计。
- Agent 只备份配置；设备凭据依赖平台用户或密钥环。跨设备恢复应重新配对，不导出可迁移明文 Token。
- 切换工作台数据源前保留旧历史、旧配置和旧程序。先停同设备旧采集器，再启新 Agent；回退时反向执行。新项目安装器不会自动替换旧程序。
- Hub 默认 SQLite 数据页预算 2048 MiB，剩余空间低于 128 MiB 暂停接收并返回 503 / Retry-After。可用 `--database-limit-mib`、`--min-free-mib` 调整；WAL 与备份需预留额外空间，不能将数据库预算当成整个目录的精确硬上限。
- Hub 默认每小时清理过期样本和审计。数据库页释放不等于立即收缩文件；维护窗口可在完整备份后进行 SQLite VACUUM。不要在小内存设备高峰时执行。

## 构建与校验

`scripts/build.ps1` 生成 Windows 与 Linux 多架构二进制、依赖清单和 SHA256；Windows 安装器另需 Inno Setup。Android 使用 Gradle 依赖校验。macOS 包在 macOS CI / 本机生成，不能由 Windows 编译 helper。

构建产物未做商业代码签名。用 Release 的 SHA256 验证下载完整性；哈希证明文件一致，不替代可信签名。未实测的硬件／系统限制见 [VALIDATION.md](VALIDATION.md)。

## Android 发行签名

本地发行可通过环境变量 `CHRONOS_KEYSTORE`、`CHRONOS_KEYSTORE_PASSWORD`、`CHRONOS_KEY_ALIAS`、`CHRONOS_KEY_PASSWORD` 传入自己的私有签名配置，再运行 `:app:assembleRelease`。未配置时不会把 debug 密钥冒充正式签名。不要把密钥、密码或签名配置文件提交 Git；同一应用后续升级必须使用同一签名私钥。构建完清理进程环境变量，独立备份私钥及密码。CI 默认仅构建开发 APK，不持有发行私钥。
