# 第三方组件

| 组件 | 用途 | 许可证 |
| --- | --- | --- |
| Edge TTS GEC v2（leopold7/edge-tts-go ac5ef46） | 修复微软朗读接口 403；只移植公开协议的令牌计算，沿用 WinOne 语音请求、取消与播放。改编源码为 `apps/result-ui/edge_gec.go`，来源和摘要见 `third_party/import-notices.json` | LGPL-3.0-only；保留 Go 移植与 rany2/edge-tts 7.3.1 完整许可。WinOne 继续使用 GPL-3.0，构建入口可重建改编源码，不分发 Python 语音运行时 |
| ECDICT 39cfb96 | 英语学习离线词典，保留核心词及关联词形；来源、数据摘要和加工说明见 `third_party/learning-data.json` | MIT，Copyright (c) 2025 Linwei；全文 `third_party/licenses/ecdict-39cfb96-LICENSE.txt`。在线词典仅调用 HTTP API，不复制或分发服务端代码；Windows 系统 SQLite 由系统提供 |
| Fluent UI motion（19e2d5c） | WinOne 功能性位移动效标准：`durationNormal=200ms`、`curveDecelerateMin=cubic-bezier(0.33,0,0.1,1)`；MyGo 执行可中断过渡与减少动效，首帧直接就位，不引入弹跳或装饰动画 | MIT，Microsoft Corporation；原始令牌与完整许可见 `third_party/fluent-motion/`、`third_party/licenses/fluent-ui-19e2d5c-LICENSE.txt`，摘要见 `third_party/import-notices.json` |
| Wails v2 | 保留的旧网页设置宿主与 WebView2 桥接；正式设置窗口使用 MyGo | MIT |
| go-webview2 | Windows WebView2 绑定 | MIT |
| go-winio | Windows 命名管道 | MIT |
| fyne systray | 主托盘入口 | Apache-2.0（含原项目归属） |
| Go x/sys | Windows 系统调用 | BSD-3-Clause |
| WinTray | 程序启动、窗口与托盘管理；跟进 89c246a 的已安装应用、商店应用启动与窗口/托盘识别、原图标、文件位置和启动方式迁移；保留 WinOne Host/Worker 协调、统一页面与托盘适配。来源及摘要见 third_party/wintray-source.json，完整能力对照见 docs/wintray-sync-20261119.md | MIT，全文见 modules/wintray/LICENSE |
| React / React DOM | 设置界面 | MIT |
| MyGo v0.10.0 | 原生设置窗口与共享结果窗；保留窗口、组件、Markdown、表格换行、Windows 文件拖入、异步 GPU 初始化和标准复制键接入。来源与摘要见 `third_party/mygo-winone/source.json`，不修改用户的 Go 模块缓存 | MIT；依赖许可原文与 SHA-256 见 `third_party/desktop-mygo-notices.json` 和 `third_party/result-ui-notices.json`，构建核对实际链接模块 |
| Solar Icons Linear | MyGo 原生设置窗口、共享结果窗、截图工具栏及托盘菜单图标；按 `third_party/solar-icons.json` 的映射从锁定源码生成，不自绘替代 | MIT AND CC-BY-4.0；原文分别随图标资产保留，来源、版本、归属和 SHA-256 见 `third_party/solar-icons.json` 与 `third_party/sources.json` |
| Lucide React 1.27.0 | 旧 React 前端仍使用的界面图标依赖；不作为 MyGo、结果窗、截图工具栏或托盘菜单的图标来源 | ISC，Feather 派生图标同时保留 MIT；原文 `third_party/licenses/lucide.txt`，依赖和 SHA-256 见 `third_party/runtime-notices.json` |
| Magpie | 共享 AI 适配器与独立本地网关；锁定 `e29699527866704ae0a9c6fbee4e2a3acf00b733`。账号认证适配器锁定 `bbd3a2e76d7f2e765f0181786a60169b7acf860a`，随包保留 46 个固定账号适配器，包括已有 Antigravity 连接继续使用的旧适配器。对应连接、账号、请求头预设、完整模型与媒体目录、密钥及网关能力见 `docs/ai-base-update-20261125.md` | MIT；适配归属见 `third_party/import-notices.json`，依赖和账号资源清单见 `third_party/ocr-ai-notices.json`、`third_party/ai-plugins.json` |
| Magpie 代理字段 | 同一 `e296995` 的地址校验与纯辅助函数；按原算法拒绝未配置的命名代理，沿用 WinOne 的供应商/账号独立代理 | MIT；原始许可和逐文件 SHA-256 见 `third_party/magpie-proxy-source.json` |
| Magpie 模型规则匹配 | 直接抽取同一版本的计划与模型规则解析，复用 regexp2 v1.11.5 的有时限后行/前行断言匹配 | Magpie MIT；regexp2 MIT，原文与 SHA-256 见 `third_party/runtime-notices.json`，两个原生宿主另有对应清单 |
| Magpie 内置 JSON Schema | 使用该版本 `third_party/jsonschema` 的实际替代源码及原始许可，依赖锁保留其本地替代关系 | Apache-2.0；实际原文及 SHA-256 见 `third_party/ocr-ai-notices.json` 的 `github.com/santhosh-tekuri/jsonschema/v6` v6.0.3 记录 |
| Cindy 共享字体与折叠动效 | 保留设置界面仍使用的字体准备、减少动效判断和折叠控件；源码已独立，不引用已移除模块 | Apache-2.0；独立保存原始 LICENSE 与 NOTICE，具体来源与文件摘要见 `third_party/import-notices.json` 的 `cindy-shared-fonts`、`cindy-shared-motion` |
| Vite / Vitest / TypeScript | 构建与测试 | 各项目许可证 |

精确版本见各模块依赖锁文件。引入或升级时同步原始版权、许可全文、传递依赖与来源摘要，并立即做源码级验证；分发前再对实际产物复核，而不是到构建结束才补许可证。

| 本次接入组件 | 用途与来源 | 许可证与即时核对 |
| --- | --- | --- |
| open-claude-ui `02d021fceaf2a907f9e63a8125621bdda1b85d60` | CDS 2.0 控件几何、动效令牌与菜单层级，适配 WinOne 黑白灰设置；不是 Anthropic 官方组件库 | MIT；原文 `third_party/licenses/open-claude-ui-02d021f-LICENSE.txt`，改编文件与摘要见 `third_party/import-notices.json` |
| windows-capture 2.0.1 | 原生 WGC 捕获；Media Foundation 流式适配沿用其公开模式，改用有界输入队列与池化 IBuffer | MIT；完整依赖许可见 `third_party/recording-notices.json` |
| gifski 1.34.0 | 静态链接的 GIF 流式编码器；不启用 CLI、PNG 序列帧或 FFmpeg 功能 | AGPL-3.0-or-later；许可从该发行包记录的精确提交补齐，见 `third_party/license-origins.json` |
| imagequant / gif-dispose | gifski 的量化与帧合成依赖 | imagequant 保留含原始归属的 COPYRIGHT 全文；gif-dispose 5.0.1 的包与精确仓库提交均未带独立许可文件，按原始 Cargo 声明选择 Apache-2.0，随包提供作者声明和 Apache 官方标准全文，不伪造作者版权文字 |

录制模块仍遵守 WinOne 根 GPLv3 许可证；与 gifski 组合分发时，必须同时履行 GPLv3/AGPLv3 第 13 条和对应源码提供义务，保留全部版权及修改说明。此接入不授予自动对外发布权限，也没有把整个 WinOne 根许可证偷偷改成 AGPL。

ShareX 的截图库与贴图代码用于独立 Capture Worker，相关许可保留在 `modules/capture/Licenses/`。录像选区线框复用其 `ScrollingCaptureRegionForm`，适配灰阶描边、不抢焦点和捕获排除；滚动截图继续使用其选区验证与拼接引擎。源码版本固定，下载缓存和编译产物不进入 Git；分发时提供对应源码与接入修改。

OCR 使用 RapidOCRLite 5.0.0a16 官方 Windows x64 SDK，提交 `d26a04c2ea2479509359391c053771b31ba8048e`。引擎及内含的 Indra 实现采用 Apache-2.0，Clipper 采用 Boost-1.0，x86-simd-sort 采用 BSD-3-Clause。许可全文保存在 `third_party/licenses/rapidocrlite-*.txt`。识别模型为原有 PP-OCRv6 small 检测与识别模型，Apache-2.0，字典按识别模型的 character 元数据原样提取。来源和 SHA-256 见 `third_party/ocr-runtime.json`，本包仅分发这两份 PP-OCR 模型。

文字阅读顺序和中西文间距复用 HushSnap `2d43f0e1fd125484bd132ee6e9cad1db65fb7a8c` 的 GPL-3.0 实现，现已等价移植至 C#，对应源码为 `modules/capture/Ocr/OcrReadingOrder.cs`。自然段衔接复用 Umi-OCR `83173efc4f4453b41223ea96d63d44534bf0505c` 的 GapTree 和两阶段段落分析，MIT，Copyright (c) 2023 hiroi-sora，适配代码为 `OcrGapTree.cs` 和 `OcrParagraphs.cs`。改编记录与校验见 `third_party/import-notices.json`。原位翻译所需 OpenCV 由下文的 Tools3000 接入独立提供。

OCR 生命周期仅调研参考 SnapFind（MIT，提交 `39416254c48aa7633254ecb79f890802fea2b401`），未引入其代码、模型或运行依赖。低内存说明与本机冷/热调用对照见 `docs/capture-experience-repair.md`；未采用会增加首次识别等待的闲置卸载策略。
# FileShare · 互联助手局域网服务

FileShare 1.0.13, commit `7228df100f687cd33332331e9c590606623408e4`,
is distributed under MIT. The Rust HTTP engine and browser assets are reused
directly; WinOne adapts the desktop management, data paths, authentication and
shutdown boundaries. Original license: `third_party/file-share/LICENSE`.
Source and changes: `third_party/file-share/WINONE.md`. Transitive dependency
licenses: `third_party/interconnect-lan-notices.json`.
# croc file transfer

Interconnect directly reuses [croc v11.5.5](https://github.com/schollz/croc/tree/8a806c36956533f9c003c55f16e31fcf14133f8e) (MIT), including the original transfer and encrypted storage clients. Adaptations are documented in third_party/croc/WINONE.md. Transfer and Tailcat roles share one executable, with a separate process for every task. Original dependency licenses and their SHA-256 values are listed in third_party/interconnect-worker-notices.json; upstream notices remain in third_party/croc/THIRD_PARTY_NOTICES.md.

# Tailcat · 持续分享与 TCP 通道

直接使用官方 Tailcat v0.7.0，提交 `15ab9e68bfc6534a61797d7af28cedd42b54a3a5`，BSD-3-Clause。TCP 转发生命周期参考其 `cmd/tailcat/forward.go`，WinOne 适配进程监督与 FileShare 的标准 HTTP 网关。原始许可、Go 工具链及传递依赖记录见 `third_party/interconnect-peer-notices.json`，改编文件与摘要见 `third_party/import-notices.json`。Tailcat-Box 仅用于调研，未复制其 PolyForm Noncommercial 实现。

## 贴图原位翻译

贴图复用 [Tools3000 v1.4.1](https://github.com/yuan278501381/Tools3000/tree/481c20f3444cb7cbcf1aa346896044f04602a376) 的段落分组、内容过滤、字形采样、背景修复和 DirectWrite 排版。上游采用 GPL-3.0。WinOne 接管贴图交互、设置、识别与翻译调用。图像算法使用 [OpenCV 4.12.0](https://github.com/opencv/opencv/tree/49486f61fb25722cbcf586b7f4320921d46fb38e)。对应许可及第三方声明随包提供。见 [Tools3000 许可](third_party/licenses/tools3000-GPL-3.0.txt)、[OpenCV 声明](third_party/licenses/pin-opencv-NOTICES.txt)和[来源清单](third_party/import-notices.json)。
