# 指标与协议口径

Schema 版本为 1，参考 [`schema.json`](../protocol/schema.json)。时间为 UTC RFC3339，服务端分别保留 `sampled_at` 与 `received_at`。设备 ID 在 Hub 配对时分配；不使用主机名、MAC 或硬件序列号生成身份。

## 能力状态

`supported` 才有数据对象；其余状态数据为 JSON null：

| 状态 | 含义 |
|---|---|
| warming_up | 速率缺少前一有效样本 |
| unsupported | 平台／当前来源不提供 |
| disabled | 本地或中心权限关闭 |
| permission_denied | 当前用户无法读取 |
| error | 读取失败 |

范围分别为 `host`、`container`、`app`、`accessible_storage`。对象内部的速率字段可能为 null；不能用 null 计算出 0。网卡／磁盘重置、接口变化或计数倒退时，重建基线，不产生负吞吐。原生挂起后的速率是两个实际采样时刻间的平均值，不代表休眠期间实时读数。

## 基础字段

| 模块 | 单位与语义 |
|---|---|
| cpu | 相邻累计时间差的整机百分比 0–100；per_core_percent 是每核百分比；首样本 warming_up |
| memory | bytes；used=total−available；百分比由此计算；不同系统 available 的内核定义有差异 |
| swap | bytes 和 percent；不存在 Swap 时总容量可为 0，不能等同采集失败 |
| storage.volumes | 选定可访问卷的 bytes、容量 percent；名称为卷标签，不上报文件正文 |
| storage.io | 逐块设备累计字节和 bytes/second；首速率 null；不把分区和父磁盘相加 |
| network | 逐接口累计字节、bytes/second；总量是已提供接口之和，VPN／虚拟网卡可能重复计算同一业务流量，不等同互联网账单 |
| system | OS、内核、架构、uptime_seconds；hostname 需双重同意；不采公网 IP |
| processes | 可见范围内最多枚举 512 个，按驻留内存选前 20；CPU 按整机容量归一；不采命令行参数、环境变量或路径 |
| battery | percent、on_battery；平台 API 没有数据时 unsupported |
| sensors | 本版默认 unsupported；不伪造温度／GPU 读数 |

进程采样缓存一分钟，模块记录自己的 collected_at；低频扩展与基础速率不使用同一更新时间伪装。网络地址默认关闭，即便开启也不包含凭据；Docker 宿主视角无法可靠读取地址时显式 unsupported。

## Android 差异

Android 的 memory 使用 ActivityManager；storage 仅为应用数据所在分区可访问容量。整机 CPU、Swap、其他应用进程与传感器模块不提供。TrafficStats 可用时上报整机计数，否则退回自身 UID，并标记 app 范围；范围变化时必须重新建立速率基线。电池来自系统粘性广播。

后台周期为 15–1440 分钟并受系统约束，前台最短 10 秒；两者共用互斥上传入口。Hub 对 Android 显示“同步延迟”，不沿用桌面节点十秒心跳误报离线。

## 历史与兼容

设备 + session + sequence 唯一约束避免重试重复写入。RAM 队列按原始采样时刻补报；更旧的样本进入历史，但不能覆盖最新快照。服务端拒绝过旧（24 小时外）或未来超过 10 分钟的时间。

分钟汇总为每个有效指标分别计数，缺失／预热值不参与平均。长时间查询聚合分钟均值；1h / 24h / 7d / 30d 的查询点数受到限制。当前历史曲线覆盖 CPU、内存和网络；逐磁盘、进程与传感器长期曲线尚未提供。进程正文不进入历史。

工作台适配器保留旧端点的数据结构，附加 capabilities 与 source；不把无法映射的指标填成 0。原工作台前端需要将 null 显示为“—”，不能直接用 `Number(null)` 展示为零。旧节点历史继续原位保留，新节点历史由 Hub 提供，不复制入旧 SQLite。
