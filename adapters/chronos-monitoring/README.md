# Chronos Monitoring 只读适配器

此目录是**可选的后端接入层**，不会自动修改正在运行的工作台。需先验证测试设备，然后再由维护者按节点切换。

[Monitoring 源码](https://github.com/Jimmy-xuzimo/Chronos-Monitoring) · [Monitoring 产品页](https://jimmy-xuzimo.github.io/Chronos-Monitoring/) · [Widgets 桌面端](https://jimmy-xuzimo.github.io/Chronos-Widgets/) · [系列接入指南](../../docs/CHRONOS_ECOSYSTEM.md)

## 接入

1. 在 Hub 创建限定设备的只读凭据。不要用设备 Token 或管理 Cookie。
2. 将 `chronos_hub.py` 放入工作台后端可导入目录。
3. 在工作台私有目录建立 `hub-connection.json`，权限 0600，内容例如：

```json
{
  "origin": "https://hub.example.com",
  "reader_token": "REPLACE_WITH_YOUR_SCOPED_READER_TOKEN",
  "bindings": {
    "existing-nas-node-id": "HUB_DEVICE_ID_FROM_YOUR_HUB"
  }
}
```

仅用于说明的占位符不能直接连接。`bindings` 的左侧是工作台原节点 ID，右侧是 Hub 稳定 ID；更名／IP 变化不修改映射。不要依照相同主机名自动合并。映射 server 会替换工作台服务器性能来源，因此应特别谨慎。

4. 在 Flask 路由都注册完后、`app.run` 或 WSGI 导出之前，**显式选择**接入：

```python
from chronos_hub import Hub, install

hub = Hub.from_file('/path/to/private/hub-connection.json')
install(app, authenticated, hub)
```

保留工作台原本的 `authenticated` 装饰器。适配器包装原有 `nodes`、`stats`、`history`、`overview` 四个 view function；如果你的分支重命名了这些函数，先调整映射再使用。只读 Token 不下发前端。

## 行为

- 十秒内共享设备列表内存缓存，无额外采集与历史数据库。
- 映射外的节点、日历、网站、资讯等继续走工作台原逻辑。
- 新历史从 Hub 读取；不会把样本重复存入工作台。原历史文件原位保留，切换边界由维护者记录。
- 网络失败保留旧快照并标记 source_error；凭据 401/403 清空缓存；节点丢失保留绑定占位，不悄悄换成另一个设备。
- 不支持的 CPU、网络或磁盘 I/O 等值为 null，前端必须显示“—”，不能把 null 转成零。
- 磁盘块计数与文件系统卷没有可靠映射时，不伪造逐卷读写数据。

**前端兼容边界：** 旧版前端若直接对值调用数值格式化或排序，需要补充 null 判断。Python 适配器保留字段结构，但无法在后端修复旧前端的空值显示。正式迁移前应验证所用工作台版本。

## 回退

停止该设备旧采集器后再启用新采集器，避免重复上报。先只迁移一台测试设备。回退时移除上述三行接入代码，恢复旧采集器与原设置；不删除旧 SQLite 或撤销原 Token。撤销新 Hub 只读凭据可立即停止新数据读取。

运行映射测试：`python -m unittest discover -s adapters/chronos-monitoring -p 'test_*.py'`。
