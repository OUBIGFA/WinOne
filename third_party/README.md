# 原生来源

版本固定在 `sources.json`。源代码 checkout、NuGet/vcpkg 恢复和编译产物放 `_temp/<任务>/`，不将整套上游仓库和缓存提交。

`archiveSha256` 是本项目记录的下载文件校验值。构建前核对版本与校验值；发布时提供对应源码与补丁。

PowerToys 补丁仅修正 AlwaysOnTop 的资源生成命令，使带空格的仓库路径可构建。应用补丁后用 Visual Studio MSBuild 构建 AlwaysOnTop.vcxproj，选择 Release/x64，并将 RestoreRepositoryPath 指向 checkout 根目录的 packages 绝对路径。

ShareX 当前用于 `tests/capture` 的真实截图验证。正式 Capture Worker 尚未接入。
