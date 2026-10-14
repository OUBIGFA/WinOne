# WinOne

Windows 工具集，提供截图、划词助手、窗口置顶、程序管理和统一 AI 服务。

只维护一套 WinOne 正式版，当前版本为 **0.0.1，正式发布验收仍在进行**。使用方法见[使用说明](docs/使用说明.md)，已验证范围见[开发状态](docs/development.md)。

- 仓库内固定安装器：`dist/release/WinOne-setup.exe`。
- 后续迭代只更新此安装器及 SHA-256，由用户自行安装验证。
- 项目调试不修改电脑上的真实安装、真实配置或其他软件文件；自动化验证使用仓库内隔离目录。

## 本地构建

需要 Windows 11、Go 1.26.2、Node.js 24 和 .NET 10 SDK。运行：

```powershell
./build/build.ps1
```

Go 未加入 PATH 时使用 `-GoExe <go.exe路径>`。构建还需传入已准备的 OCR 运行时与锁定的 magpie、插件源码等参数，见[打包与安装](docs/打包与安装.md)。输出位于 `_temp/build/current/`，验证后更新仓库内的固定安装器。

本机已整理的复用资源位于 `dist/resources/`。先运行 `. ./build/use-local-resources.ps1`，再用 `./build/build.ps1 @WinOneBuildParameters` 构建；资源目录与版本见[目录约定](docs/repository.md)。

运行需要 Windows 11 x64 和 WebView2 Runtime。构建、测试和候选包放在 `_temp/<任务>/`，不进入 Git。文档从[按需索引](docs/README.md)选择，不要求每次通读。

- [架构与开发计划](docs/WinOne架构与开发计划.md)
- [调研依据与来源](docs/调研依据.md)
- [目录与发布约定](docs/repository.md)
- [开发状态](docs/development.md)

已确定：Windows 11 优先，截图操作界面及新增模块采用原生控件与 WinOne 统一风格，性能优先；置顶和程序管理保持现状。按适用 GPL 要求开源发布。
