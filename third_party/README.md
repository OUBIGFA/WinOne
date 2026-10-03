# 原生来源

版本固定在 `sources.json`。源代码 checkout、NuGet/vcpkg 恢复和编译产物放 `_temp/<任务>/`，不将整套上游仓库和缓存提交。

前端组件正迁移至 shadcn/ui Base Nova。官方 CLI 4.21.0 生成的组件位于 `apps/desktop/frontend/src/components/ui/`，文件哈希、样式名称和参考项目提交记录在 `sources.json`；通用样式与字体本地打包。`src/ui/` 中的兼容入口仅映射旧调用参数，不另画按钮或开关。旧业务页面继续按体验重构计划迁移。

`archiveSha256` 是本项目记录的下载文件校验值。构建前核对版本与校验值；发布时提供对应源码与补丁。

PowerToys 适配由 `build/build-always-on-top.ps1` 管理，包含构建路径、WinOne 桥接及配置路径调整，使用 Release/x64 构建。

ShareX 用于 `modules/capture`。准备脚本增加本地编辑器开关、标题适配和滚动管理器访问；贴图源码及其小范围适配保留在模块的 `Upstream/`。`tests/capture` 与 `tests/capture-unit` 提供捕获、输出和窗口生命周期验证。

`runtime-notices.json` 记录 Go 正式构建依赖、前端锁定生产依赖、截图 NuGet 依赖、Go 工具链、.NET 运行时和 PowerToys 的版本及声明文件哈希。`licenses/` 保存声明原文，相同内容共用文件。构建结束前由 `build/verify-notices.ps1` 核对依赖版本、声明内容及随包副本，并生成 `licenses/runtime-notices.txt` 索引。

前端检查包含锁文件中非纯开发依赖的 peer/type 条目，不表示这些包的全部文件都进入程序。手动复用源码仍按 `sources.json` 记录；对应源码归档继续补齐。
