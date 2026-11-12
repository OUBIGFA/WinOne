# WinOne

Windows 工具集，提供截图、OCR、翻译、划词助手、快捷助手、互传、学习、窗口置顶和程序管理。

main 的 AI 服务仅保留常用、自定义供应商、账号供应商、使用偏好、本地网关。Chat、OneBot、闪聊及其他 AI 服务已移除；完整版本保存在 [`codex/archive-full-20261112`](https://github.com/OUBIGFA/WinOne/tree/codex/archive-full-20261112)。实施和审核记录见[模块精简验收](docs/module-removal-20261112.md)。

只维护一套 WinOne 正式版，当前版本为 **0.0.1，正式发布验收仍在进行**。使用方法见[使用说明](docs/使用说明.md)，已验证范围见[开发状态](docs/development.md)。

- 仓库内固定安装器：`dist/release/WinOne-setup.exe`。
- 后续迭代只更新此安装器及 SHA-256，由用户自行安装验证。
- 项目调试不修改电脑上的真实安装、真实配置或其他软件文件；自动化验证使用仓库内隔离目录。

## 本地构建

需要 Windows 11、Go 1.26.2、Node.js 24 和 .NET 10 SDK。运行：

```powershell
./build/build.ps1
```

Go 未加入 PATH 时使用 `-GoExe <go.exe路径>`。构建还需传入已准备的 OCR 运行时与锁定的 Magpie、账号适配器源码等参数，见[打包与安装](docs/打包与安装.md)。输出位于 `_temp/build/current/`，验证后更新仓库内的固定安装器。

本机已整理的复用资源位于 `dist/resources/`。先运行 `. ./build/use-local-resources.ps1`，再用 `./build/build.ps1 @WinOneBuildParameters` 构建；资源目录与版本见[目录约定](docs/repository.md)。

运行需要 Windows 11 x64 和 WebView2 Runtime。构建、测试和候选包放在 `_temp/<任务>/`，不进入 Git。文档从[按需索引](docs/README.md)选择，不要求每次通读。

只检查源码时运行 `./build/test-source.ps1`（Go 未加入 PATH 时传入 `-GoExe`）。此入口供 Windows CI 和本地使用，验证 Host、根 Go 模块、WinTray、systray 与保留的第三方来源；不需要预装 OCR/Magpie 运行时。原生 UI、AI、截图、整包和安装器仍由完整构建及对应验收入口验证。源码检查通过不代表正式包验收通过。

- [架构与开发计划](docs/WinOne架构与开发计划.md)
- [调研依据与来源](docs/调研依据.md)
- [目录与发布约定](docs/repository.md)
- [开发状态](docs/development.md)

已确定：Windows 11 优先，截图操作界面及新增模块采用原生控件与 WinOne 统一风格，性能优先；置顶和程序管理保持现状。按适用 GPL 要求开源发布。
