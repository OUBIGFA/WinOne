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
| Magpie | 共享 AI 适配器；本次来源提交 d783df46cbac9021e26a6bd71ca0b55544552eb2；社区插件 0a998a8（32 包） | MIT；构建复制上游 LICENSE，依赖许可见 third_party/ocr-ai-notices.json 与 third_party/ai-plugins.json |
| Cindy | Chat 标题提示词、完整阅读/右栏组件，以及发送、插话、重试、草稿快照和滚动意图适配（交互核对提交 77e2c32d；资源发现、同轮输入与恢复核对 a44ac274） | Apache-2.0；保留 third_party/cindy/LICENSE、NOTICE，来源与原文 SHA-256 见 title-source.json、sidebar-source.json、chat-interaction-source.json、chat-runtime-source.json 及各功能来源清单 |
| Vite / Vitest / TypeScript | 构建与测试 | 各项目许可证 |
| Pi Durable / Pi Code Mode 1.3.0、quickjs-wasi 3.6.2 | Chat 持久执行及独立 QuickJS VM；模型与工具仍经过 WinOne 服务与权限控制 | MIT；原始 Pi、QuickJS-NG、WASI libc 与编译器运行时许可见 `third_party/chat-codemode-notices.json`，构建核对源码与随包副本 |

精确版本见各模块依赖锁文件。引入或升级时同步原始版权、许可全文、传递依赖与来源摘要，并立即做源码级验证；分发前再对实际产物复核，而不是到构建结束才补许可证。

| 本次接入组件 | 用途与来源 | 许可证与即时核对 |
| --- | --- | --- |
| open-claude-ui `02d021fceaf2a907f9e63a8125621bdda1b85d60` | CDS 2.0 控件几何、动效令牌与菜单层级，适配 WinOne 黑白灰设置；不是 Anthropic 官方组件库 | MIT；原文 `third_party/licenses/open-claude-ui-02d021f-LICENSE.txt`，改编文件与摘要见 `third_party/import-notices.json` |
| windows-capture 2.0.1 | 原生 WGC 捕获；Media Foundation 流式适配沿用其公开模式，改用有界输入队列与池化 IBuffer | MIT；完整依赖许可见 `third_party/recording-notices.json` |
| gifski 1.34.0 | 静态链接的 GIF 流式编码器；不启用 CLI、PNG 序列帧或 FFmpeg 功能 | AGPL-3.0-or-later；许可从该发行包记录的精确提交补齐，见 `third_party/license-origins.json` |
| imagequant / gif-dispose | gifski 的量化与帧合成依赖 | imagequant 保留含原始归属的 COPYRIGHT 全文；gif-dispose 5.0.1 的包与精确仓库提交均未带独立许可文件，按原始 Cargo 声明选择 Apache-2.0，随包提供作者声明和 Apache 官方标准全文，不伪造作者版权文字 |

录制模块仍遵守 WinOne 根 GPLv3 许可证；与 gifski 组合分发时，必须同时履行 GPLv3/AGPLv3 第 13 条和对应源码提供义务，保留全部版权及修改说明。此接入不授予自动对外发布权限，也没有把整个 WinOne 根许可证偷偷改成 AGPL。

ShareX 的截图库与贴图代码用于独立 Capture Worker，相关许可保留在 `modules/capture/Licenses/`。录像选区线框复用其 `ScrollingCaptureRegionForm`，适配灰阶描边、不抢焦点和捕获排除；滚动截图继续使用其选区验证与拼接引擎。源码版本固定，下载缓存和编译产物不进入 Git；分发时提供对应源码与接入修改。

OCR 生命周期仅调研参考 SnapFind（MIT，提交 `39416254c48aa7633254ecb79f890802fea2b401`），未引入其代码、模型或运行依赖。低内存说明与本机冷/热调用对照见 `docs/capture-experience-repair.md`；未采用会增加首次识别等待的闲置卸载策略。
