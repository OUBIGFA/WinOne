# 原生来源

版本固定在 `sources.json`。源代码 checkout、NuGet/vcpkg 恢复和编译产物放 `_temp/<任务>/`，不将整套上游仓库和缓存提交。

`archiveSha256` 是本项目记录的下载文件校验值。构建前核对版本与校验值；发布时提供对应源码与补丁。

PowerToys 补丁仅修正 AlwaysOnTop 的资源生成命令，使带空格的仓库路径可构建。应用补丁后用 Visual Studio MSBuild 构建 AlwaysOnTop.vcxproj，选择 Release/x64，并将 RestoreRepositoryPath 指向 checkout 根目录的 packages 绝对路径。

ShareX 用于 `modules/capture`。准备脚本增加本地编辑器开关、标题适配和滚动管理器访问；贴图源码及其小范围适配保留在模块的 `Upstream/`。`tests/capture` 与 `tests/capture-unit` 提供捕获、输出和窗口生命周期验证。
