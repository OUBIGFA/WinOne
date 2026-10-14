# 第三方组件

| 组件 | 用途 | 许可证 |
| --- | --- | --- |
| Wails v2 | 设置窗口与 WebView2 桥接 | MIT |
| go-webview2 | Windows WebView2 绑定 | MIT |
| go-winio | Windows 命名管道 | MIT |
| fyne systray | 主托盘入口 | Apache-2.0（含原项目归属） |
| Go x/sys | Windows 系统调用 | BSD-3-Clause |
| WinTray | 程序启动、窗口与托盘管理 | MIT，全文见 modules/wintray/LICENSE |
| React / React DOM | 设置界面 | MIT |
| ChatGPT-Desktop-UI / chatgpt-desktop-kit 0.6.0 | Chat 阅读内容、右栏工作区与项目栏 | MIT；包含发行包自带的第三方许可全文 |
| Magpie | 共享 AI 适配器；本次来源提交 0bb0781ea065b5236a9a0014bceb84cb84324895 | MIT；构建复制上游 LICENSE，依赖许可见 third_party/ocr-ai-notices.json |
| Cindy | Chat 标题提示词与输出校验的 Go 移植 | Apache-2.0；保留 third_party/cindy/LICENSE、NOTICE，来源见 title-source.json |
| Vite / Vitest / TypeScript | 构建与测试 | 各项目许可证 |

精确版本见 go.mod、go.sum 和前端 package-lock.json。分发前应为全部直接与传递依赖生成许可全文清单；当前为开发构建，不发布正式包。

ShareX 的截图库与贴图代码用于独立 Capture Worker，相关许可保留在 `modules/capture/Licenses/`。源码版本固定，下载缓存和编译产物不进入 Git；分发时提供对应源码与接入修改。
