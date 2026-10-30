# 第三方组件

| 组件 | 用途 | 许可证 |
| --- | --- | --- |
| Wails v2 | 设置窗口与 WebView2 桥接 | MIT |
| go-webview2 | Windows WebView2 绑定 | MIT |
| go-winio | Windows 命名管道 | MIT |
| fyne systray | 主托盘入口 | Apache-2.0（含原项目归属） |
| Go x/sys | Windows 系统调用 | BSD-3-Clause |
| WinTray | 程序启动、窗口与托盘管理；核对 484e382 及 2026-10-22 Esc 修复工作树，来源与改编见 third_party/wintray-source.json | MIT，全文见 modules/wintray/LICENSE |
| React / React DOM | 设置界面 | MIT |
| Lucide messages-circle（7faf876） | 闪聊入口及页面图标，直接复用官方 SVG 路径；来源与摘要见 `third_party/import-notices.json` | ISC；全文见 `third_party/licenses/lucide.txt` |
| MyGo v0.7.3 | MyGo 原生设置窗口（`apps/desktop-mygo`，迁移进行中）：窗口、组件与 GPU 绘制；同时链接 purego、go-text/typesetting、golang.org/x/image、x/net、x/text、x/sys、go-winio、go-ole | MIT（MyGo）；依赖许可原文与 SHA-256 见 `third_party/desktop-mygo-notices.json`，`build/verify-desktop-mygo-notices.ps1` 按实际链接的模块校验 |
| Lucide React 1.27.0 | 官方 AppWindow / LogOut 托盘菜单图标及既有界面图标；由锁定 npm 包生成透明模板，不自绘替代 | ISC，Feather 派生 LogOut 同时保留 MIT；原文 `internal/branding/menu-icons/LICENSE.txt`，生成器与 SHA-256 见 `third_party/sources.json` |
| ChatGPT-Desktop-UI / chatgpt-desktop-kit 0.6.0 | Chat 阅读内容、右栏工作区与项目栏 | MIT；包含发行包自带的第三方许可全文 |
| Magpie | 共享 AI 适配器；本次来源提交 42ec2700bd2a4cf8d4ceb76a95767ac221e283e5；社区插件 e513644（41 包） | MIT；构建复制上游 LICENSE，依赖许可见 third_party/ocr-ai-notices.json 与 third_party/ai-plugins.json |
| Magpie 用量图表 | 同一 `42ec2700` 的柱状／平滑堆叠折线、速度分段曲线、排名与统计口径；`build/sync-magpie-usage.mjs` 可复现，React 仅适配生命周期／导航／主题 | MIT；原始许可与源码、生成文件 SHA-256 见 `third_party/magpie-usage-source.json`；其他旧版用量组件继续使用各自来源清单 |
| Magpie 代理字段 | 同一 `42ec2700` 的代理地址／登录信息拆分、转义和地址校验；`build/sync-magpie-proxy.mjs` 直接抽取原函数 | MIT；原始许可和逐文件 SHA-256 见 `third_party/magpie-proxy-source.json` |
| OpenAI Privacy Filter / LocalAI q8 GGUF / pf-serve | 高级隐私设置的本地模型；用户开启时按固定 SHA-256 下载，不包含在安装器中；来源、版本与完整原文见 `third_party/privacy-model.json` | 模型 Apache-2.0；privacy-filter.cpp、ggml 为 MIT；保留模型卡的 OpenAI/LocalAI 归属与原生下载包许可 |
| Cindy | Chat 标题提示词、完整阅读/右栏组件，以及发送、插话、重试、草稿快照和滚动意图适配（排队列表与编辑控制核对 b1d73b1f；交互核对提交 77e2c32d；资源发现、同轮输入与恢复、计划卡、侧栏活动、独立气泡/动作布局、输入区重试提示、文件链接菜单、文件拖放／粘贴与错误分类核对 a44ac274；自动化完整页面、排期引擎与例行任务、前置检查生成器、中英文案按 b7e46bcb 原样同步，build/sync-cindy.mjs 可复现） | Apache-2.0；保留 third_party/cindy/LICENSE、NOTICE，来源与原文 SHA-256 见 scheduler.sync.json、title-source.json、sidebar-source.json、chat-interaction-source.json、chat-runtime-source.json、chat-resilience-source.json、chat-links-source.json、chat-queue-source.json 及各功能来源清单 |
| Vite / Vitest / TypeScript | 构建与测试 | 各项目许可证 |
| Cindy OneBot 本地基础服务 | `b7e46bcb` 的伙伴 Home、档案版本/能力选择/模型路线语义、Maker Memory/FTS 与编辑服务、技能存储/按需检索与挂载投影、待办、夜间学习状态与计划校验；按 `third_party/cindy/onebot.sync.json` 机械同步，SQLite/日志/进程宿主由 WinOne 适配。此批源码尚不表示 OneBot 界面或 IM 已接通 | Apache-2.0；复用 `third_party/cindy/LICENSE`、`NOTICE`；全部来源与改编摘要见同步清单，直接/传递运行依赖见 `third_party/onebot-notices.json` |
| Cindy IM / WeChat iLink | 直接同步锁定 Cindy `b7e46bcb` 的 lizi-im、wechat-ilink、连接记录与媒体核验；不另写 Telegram/飞书/企业微信/钉钉/Discord/iLink 协议。宿主派发仍在接入中 | Apache-2.0；来源与 SHA-256 见 `onebot.sync.json`，保留 Cindy LICENSE/NOTICE |
| Cindy OneBot 设置组件 | `b7e46bcb` 的记忆编辑、生命周期/删除/历史、关联项目与能力选择页面、档案自动保存与能力列表合并逻辑；宿主档案版本、资源选择及草稿接线由 WinOne 适配。关联项目服务复用 `botLinkedProjects`、`botWorkbenchService`；完整产品入口仍在接入中 | Apache-2.0；保留 `third_party/cindy/LICENSE`、`NOTICE`；原文/生成摘要及定点适配见 `third_party/cindy/onebot-renderer.sync.json` 与 `onebot.sync.json`，用 `build/sync-cindy.mjs` 复现和校验 |
| IM SDK 依赖 | `@larksuiteoapi/node-sdk` 1.71.1、`@wecom/aibot-node-sdk` 1.0.7、`dingtalk-stream` 2.1.5（包内版本写作 v2.1.5）、`discord.js` 14.27.0 及传递依赖 | MIT / Apache-2.0 等按 `onebot-notices.json` 的 91 项闭包逐项声明。按用户 2026-10-26 要求沿用 Cindy：WeCom SDK 和两个 Sapphire 包缺独立许可全文，原样保留 package.json/README 的许可声明及 Cindy metadata-only 附录证据，不将其称为独立 LICENSE，也不伪造全文 |
| Cindy OneBot 分工工作树 | 直接复用 `b7e46bcb` 的 WorktreeManager 创建流程、prepareHandoffWorktree、新鲜基底、目录登记与运行租约；存储及进程目录由 WinOne 适配 | Apache-2.0；保留 Cindy LICENSE/NOTICE，原文和产物摘要见 `third_party/cindy/onebot-worktree.sync.json` |
| Cindy OneBot 语音 | 复用 `b7e46bcb` 的通话、试听、朗读队列、语音输入核心、语音设置/本地词典及模型目录。系统音频静音沿用原版所有权保护，Windows Core Audio 调用由 WinOne Host 承接，不分发 loudness 的原生辅助程序 | Apache-2.0；保留 Cindy LICENSE/NOTICE，原文和产物摘要见 `third_party/cindy/onebot-voice.sync.json` 与 `onebot-renderer.sync.json` |
| js-yaml 4.3.0、zod 4.3.6、argparse 2.0.1 | OneBot 技能 YAML 与学习计划 schema；版本与 Cindy 锁文件一致，未修改包源码 | MIT（js-yaml/zod）、Python-2.0（argparse）；完整原始许可与 SHA-256 见 `third_party/onebot-notices.json`，通过 `node build/verify-onebot-notices.mjs` 校验 |
| gray-matter 4.0.3 及其传递依赖 | OneBot 记忆 frontmatter 与原文导入；不另写 YAML 解析器 | MIT/BSD/Python-2.0 等按锁定包逐项核实；原始许可、全部 OneBot npm 依赖与摘要见 `third_party/onebot-notices.json` |
| Pi / Pi Durable / Pi Code Mode 1.4.0、quickjs-wasi 3.6.2 | Chat 持久执行及独立 QuickJS VM；模型与工具仍经过 WinOne 服务与权限控制 | MIT；原始 Pi、QuickJS-NG、WASI libc 与编译器运行时许可见 `third_party/chat-codemode-notices.json`，构建核对源码与随包副本 |

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
# FileShare · 互联助手局域网服务

FileShare 1.0.13, commit `7228df100f687cd33332331e9c590606623408e4`,
is distributed under MIT. The Rust HTTP engine and browser assets are reused
directly; WinOne adapts the desktop management, data paths, authentication and
shutdown boundaries. Original license: `third_party/file-share/LICENSE`.
Source and changes: `third_party/file-share/WINONE.md`. Transitive dependency
licenses: `third_party/interconnect-lan-notices.json`.
# croc file transfer

Interconnect directly reuses [croc v11.5.5](https://github.com/schollz/croc/tree/8a806c36956533f9c003c55f16e31fcf14133f8e) (MIT), including the original transfer and encrypted storage clients. Adaptations are documented in third_party/croc/WINONE.md. Original dependency licenses and their SHA-256 values are listed in third_party/interconnect-transfer-notices.json; upstream notices remain in third_party/croc/THIRD_PARTY_NOTICES.md.

# Tailcat · 持续分享与 TCP 通道

直接使用官方 Tailcat v0.7.0，提交 `15ab9e68bfc6534a61797d7af28cedd42b54a3a5`，BSD-3-Clause。TCP 转发生命周期参考其 `cmd/tailcat/forward.go`，WinOne 适配进程监督与 FileShare 的标准 HTTP 网关。原始许可、Go 工具链及传递依赖记录见 `third_party/interconnect-peer-notices.json`，改编文件与摘要见 `third_party/import-notices.json`。Tailcat-Box 仅用于调研，未复制其 PolyForm Noncommercial 实现。

# Chitchatter · 点对点聊天

完整无 UI 核心来自 [Chitchatter](https://github.com/jeremyckahn/chitchatter/tree/4093c21dad24af133c3c5c12042d3eb6112eece1)，提交 `4093c21dad24af133c3c5c12042d3eb6112eece1`，按上游 package.json 的 GPL-2.0-or-later 声明接入。原始许可见 `third_party/licenses/chitchatter-4093c21-LICENSE.txt`，源码、原始测试及适配说明保存在 `apps/desktop/frontend/src/vendor/chitchatter/`，来源摘要见 `third_party/import-notices.json`。Trystero 等前端依赖许可由 `third_party/runtime-notices.json` 记录。

房间、身份、群聊、私聊、消息及 file offer 协议直接复用核心。文件适配器使用 FileShare + 官方 Tailcat 地址，双方需使用 WinOne；不宣称与原 Chitchatter WebTorrent 文件客户端互通。WinOne 以 GPLv3 与该 GPL-2.0-or-later 核心组合，分发时保留归属、原始许可、修改说明及对应源码提供义务。
