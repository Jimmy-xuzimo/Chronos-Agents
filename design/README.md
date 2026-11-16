# Chronos 桌面视觉参数

`desktop-design.json` 同时供 Widgets 的 WPF 与 Windows Agent 的 WinForms 读取，并作为资源嵌入安装程序。字体、主次文字、强调色、渐变、圆角、控件高度与线宽使用同一版本。Widgets 保留深浅主题和工业废土覆盖；采集器保持轻量彩色外观。

修改时同步两个仓库的 JSON 与 SHA256 文件，运行 `python scripts/check_desktop_design.py`。检测到同目录的配套仓库时，脚本也逐字比较内容。安装后不联网读取设计参数。
