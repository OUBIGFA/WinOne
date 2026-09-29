# 第三方组件

| 组件 | 用途 | 许可证 |
| --- | --- | --- |
| Wails v2 | 设置窗口与 WebView2 桥接 | MIT |
| go-webview2 | Windows WebView2 绑定 | MIT |
| go-winio | Windows 命名管道 | MIT |
| fyne systray | 主托盘入口 | Apache-2.0（含原项目归属） |
| Go x/sys | Windows 系统调用 | BSD-3-Clause |
| React / React DOM | 设置界面 | MIT |
| Vite / Vitest / TypeScript | 构建与测试 | 各项目许可证 |

精确版本见 go.mod、go.sum 和前端 package-lock.json。分发前应为全部直接与传递依赖生成许可全文清单；当前为开发构建，不发布正式包。

ShareX 的 GPLv3 截图库目前只用于可选验证项目，尚未包含在主程序中。其验证需提供 v21.0.0 源码路径，源码和编译产物不随本仓库的基础构建提交。
