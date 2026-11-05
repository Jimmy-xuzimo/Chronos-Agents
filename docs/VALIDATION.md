# 0.1.0 验证记录

本机初验：2026-11-04；发行验证持续至 2026-11-05。以下区分本机执行、交叉构建和未实测项目。预览版不是全部平台验收完成的稳定版。

## 本机已执行

| 范围 | 结果 |
|---|---|
| Windows Go 核心 | `go test ./...`、`go vet ./...` 通过 |
| 身份和认证 | 单次配对、跨设备冒充拒绝、设备／读者权限隔离、撤销、新旧凭据轮换宽限、Host / CSRF、TOTP 标准向量通过 |
| 协议和采样 | 真实 Windows 基础采样、CPU 预热、默认隐私关闭、缺失值、过大包和无效状态测试通过 |
| 历史 | 重试去重、乱序不覆盖最新、进程不写历史、缺失值不参与分钟均值、重开数据库读取通过 |
| 调度 | 本地暂停不请求、手动唤醒不绕过 Retry-After、RAM 队列条数上限测试通过 |
| 规模功能 | 100 个虚构节点独立配对、写入和读取通过；不等于百台设备长期压力测试 |
| TLS | 不信任的证书拒绝；重定向不跟随、不向第二地址泄露凭据 |
| 浏览器真实联调 | 独立本机 Hub + Windows Agent 初始化、配对、DPAPI 保存、持续上报成功 |
| 管理页布局 | 320 / 390 / 768 / 1366px 无整页横向溢出，无 JS pageerror；不是 iOS Safari 实机结论 |
| Android 构建 | Debug 与独立私钥签名 Release APK 构建、Lint 通过；警告不冒称为零 |
| Android 设备测试 | 独立 Android 16 模拟器 4 项通过：Keystore 加解密／非明文文件、能力 null、暂停不请求、HTTPS 根地址限制 |
| 工作台适配 | Python 4 项映射测试通过：缺失值、HTTPS／稳定绑定、绑定丢失占位和缓存复用 |
| Windows 安装器 | 独立目录安装、运行、关闭占用进程升级、配置哈希保持、卸载与注册表清理均通过；旧采集器未替换 |
| Linux 构建 | amd64 / arm64 静态交叉编译通过；不等于 NAS 宿主运行测试 |

浏览器联调发现并修复了同一 localhost 不同端口 Cookie 冲突；新会话名称按 origin 隔离。重试期间的唤醒保护和缺失值独立计数均加入回归测试。

## NAS 与云端补充验证

2026-11-05，在独立 fnOS x64 测试项目验证：用户 OAuth 授权、镜像导入、非 root 服务、只读根、丢弃 capabilities、HTTPS 与专用 CA、宿主 CPU / 内存、指定卷容量、默认关闭主机名／进程、历史保存和 Hub 重启恢复。暂停后不采样、恢复上传与撤销后停止上传均通过。该测试未替换现有 NAS 采集器。

首个完整云端验证 [run 41873816670](https://github.com/Jimmy-xuzimo/Chronos-Agents/actions/runs/41873816670) 在构建 `5aacda7` 全部通过：Windows、Linux（含 race detector）、macOS ARM / Intel（含 Keychain helper 往返）、Android 严格依赖验证及 Docker 双架构构建。macOS 桌面 GUI 和 ARM 实机仍不由编译／CI 代替。

后续仅涉及安装器升级处理、保守容器能力语义、隐私过滤和发行审查；发行前再次执行相应检查，最终 run 另见 Release。

## 短时资源采样

隔离 Windows headless Agent 在正常十秒采样周期下，单次约 10 秒观察：Working Set 23.93 MiB、Private Bytes 57.42 MiB。该窗口累计 CPU 低于计时分辨率，**不能据此声称零 CPU**。这是程序进程的短时记录，不包含打开管理页的浏览器、系统 WMI 服务或所有峰值。

NAS 短时容器平台读数：Agent 约 39.93 MiB、Hub 约 38.25 MiB；同样不把瞬时 CPU 读数当作长期结论。

仍需在约定硬件上完成空闲／断网／暂停／高进程数量及多小时运行对比；Hub 三／二十／一百台模拟设备的持续吞吐、存储增长和内存峰值尚未形成性能结论。

## 待验证或发行限制

- NAS 系统重启、混合磁盘和长时间运行；基本非 root / 只读根容器、宿主采样、TLS、独立卷和 Hub 重启持久化已完成真机验证。
- macOS Intel／Apple Silicon 本机托盘、Keychain 与登录启动；由各自 CI 构建后仍需用户实机验证。
- Windows 安装、升级、卸载完整闭环、不同用户、系统启动和睡眠恢复；未替换已有软件进行测试。
- Android 真机 Doze、厂商省电、长期后台、密钥失效、切网和低电量；模拟器测试不覆盖厂商行为。
- iOS Safari、Android Chrome 真实设备、屏幕阅读器和 200% 字体完整巡检。
- 生产工作台适配与逐节点迁移未执行；旧历史与现有凭据保持原样。
- Windows 无商业签名、macOS 无公证；Android 发行 APK 使用独立私钥签名，私钥与 DPAPI 保护的密码留在本机受限目录，未进入公开仓库。
- 管理页当前提供 CPU 历史曲线和全部能力快照；没有逐磁盘长期图表、多用户 RBAC、自动更新或远程命令。

CI 的真实结果以仓库 Actions 为准。工作流排队、运行中或仅有配置文件，都不表示构建成功。发布记录应另附对应 commit 和 run。

## 复验命令

```sh
python scripts/generate_protocol.py --check
python scripts/check_public.py
go test ./...
go vet ./...
python -m unittest discover -s adapters/chronos-monitoring -p 'test_*.py'
```

Android：`./gradlew :app:assembleDebug :app:lintDebug :app:connectedDebugAndroidTest`，需要在线模拟器或测试设备。Linux CI 另外运行 race detector。截图由 `scripts/screenshots.cjs` 使用虚构响应生成；README 中的截图不包含真实设备或配对数据。


## 1.0.0 正式版验证说明

本版增加 Windows WinForms 原生管理窗口和 macOS SwiftUI 原生管理窗口，Linux / NAS 网页与 Android App 保留。桌面窗口通过 loopback API 复用认证、配对与采集配置，不使用 WebView。

发布时附上最终 CI run、安装包与 SHA256。macOS 原生窗口由两种架构的 macOS CI 编译，不能冒称为本机物理交互验收；Android 厂商后台行为等边界继续有效。首次正式版本是发行阶段，不代表所有硬件验证完成。
