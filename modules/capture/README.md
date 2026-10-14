# 截图模块

使用 ShareX v21.0.0 的选区、标注、滚动拼接和贴图实现；Worker 运行于 .NET 10。

`build/prepare-sharex.ps1` 校验源码归档并准备依赖。复用源码缓存留在 `dist/resources/sources/`，版本与校验值见 `third_party/sources.json`；测试输出放在 `_temp/<任务>/`。

接入修改：

- 为编辑器增加分享入口开关，并使用 WinOne 标题
- 开放滚动捕获管理器供本地预览窗口使用
- 贴图在 Worker 的 STA 中运行，关闭全部贴图时使用稳定快照
- 保存、复制和取消结果通过命名管道返回 Host

`Upstream/` 保留贴图源码及许可头；`Licenses/` 随构建输出。发布时还需提供对应 ShareX 源码和本仓库的接入修改。

默认不常驻，空闲且没有贴图时退出。编辑或截图未结束时，普通退出请求会提示先完成或取消当前操作。
