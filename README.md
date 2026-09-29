# WinOne

Windows 聚合工具，计划整合截图、窗口置顶和 WinTray 全部功能。

目前处于**开发验证阶段**。设置窗口、独立后台、主题与主托盘可运行；WinTray 正在接入，已提供程序配置和启动入口。截图、置顶及完整兼容性验收尚未完成。

## 本地构建

需要 Windows 11、Go 1.26.2 和 Node.js 24。运行：

```powershell
./build/build.ps1
```

Go 未加入 PATH 时使用 `-GoExe <go.exe路径>`。输出位于 `dist/`，运行 `WinOne.exe`；关闭窗口后后台仍运行，在托盘选择“退出 WinOne”结束。

首次使用需要 WebView2 Runtime。测试与构建产物不进入 Git。

- [架构与开发计划](docs/WinOne架构与开发计划.md)
- [调研依据与来源](docs/调研依据.md)
- [目录与发布约定](docs/repository.md)
- [开发状态](docs/development.md)

已确定：Windows 11 优先，React + WebView2 界面、Go/C++ 后台，按适用 GPL 要求开源发布。
