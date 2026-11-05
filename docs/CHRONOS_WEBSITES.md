# Chronos Pages 统一设计与维护

三个介绍页：[Monitoring](https://jimmy-xuzimo.github.io/Chronos-Monitoring/) · [Widgets](https://jimmy-xuzimo.github.io/Chronos-Widgets/) · [Agents](https://jimmy-xuzimo.github.io/Chronos-Agents/)。

## 共同设计层

- `website/style.css`：共同的基础色、字体、间距、浮动玻璃导航和减速动效；各产品保留自己的体验示意和模块。
- `website/suite.css` / `suite.js`：三个仓库维护完全相同的系列组件。上方三项目切换、下方项目卡片、可选接入关系和页脚互链；当前项目同时用文字和选中态标识。
- 颜色沿用马卡龙渐变，明暗模式使用相同的 `chronos-site-theme` 偏好键；页面同属 GitHub Pages 域名，跨项目访问保持手动明暗选择。配色演示不改变已安装软件。
- 三个项目标识用内联 SVG 绘制，不依赖图标 CDN。文档、仓库与产品页分别提供明确链接。
- 悬停轻微上浮、渐变光晕与分区淡入；尊重系统减少动态效果，切至后台时暂停背景漂移。
- 手机三列紧凑项目切换、项目详情卡片改为单列；不设置固定正文高度。

## 内容与发布

README、产品页、主要入门文档都链接 [三项目接入指南](CHRONOS_ECOSYSTEM.md)。旧 Windows / NAS 采集方式留在 Monitoring 作为兼容资料；新的多端统一采集引导至 Agents，明确新旧协议不相同。

Pages 的静态截图与示意使用虚构内容，不连接私人服务器。应用版本和下载状态以各项目公开 Releases 为准，更新文档不等于重新发行应用。

推送 `website/` 或检查脚本后，由各仓库现有的 Pages Actions 发布；不要创建新的域名或替代部署。

## 检查

运行各项目的 `scripts/check_site.py` 和公开源码检查，并验证所有三个页面的明暗模式、项目链接、移动端溢出与控制台错误。本轮浏览器检查结果见 [系列网页验证记录](CHRONOS_WEBSITE_VALIDATION.md)。浏览器模拟不能代替手机或平板实机检查。
