# WinOne

Windows 工具集，提供截图、窗口置顶和程序管理。

当前为 **0.0.1 本机试用阶段**。使用方法和验证范围见[试用说明](docs/0.0.1试用说明.md)。

- 日常试用版：`dist/trial-0.0.1/WinOne.exe`。
- 固定测试包：`dist/test/WinOne.exe`，后续迭代更新此目录。
- 两者共用 WinOne 配置；切换版本前，从托盘退出 WinOne。

## 本地构建

需要 Windows 11、Go 1.26.2、Node.js 24 和 .NET 10 SDK。运行：

```powershell
./build/build.ps1
```

Go 未加入 PATH 时使用 `-GoExe <go.exe路径>`。首次构建会下载并校验截图依赖。输出位于 `dist/build/`，运行 `WinOne.exe`；关闭窗口后后台仍运行，在托盘选择“退出 WinOne”结束。

运行需要 Windows 11 x64 和 WebView2 Runtime。测试与构建产物不进入 Git。

- [架构与开发计划](docs/WinOne架构与开发计划.md)
- [调研依据与来源](docs/调研依据.md)
- [目录与发布约定](docs/repository.md)
- [开发状态](docs/development.md)

已确定：Windows 11 优先，截图操作界面及新增模块采用原生控件与 WinOne 统一风格，性能优先；置顶和程序管理保持现状。按适用 GPL 要求开源发布。
