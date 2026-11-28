# 仓库目录与发布约定

| 路径 | 内容 | Git 跟踪 |
| --- | --- | --- |
| `apps/` | Host、MyGo 设置与结果窗、保留的兼容宿主 | 是 |
| `internal/` | Go 公共协议与本地服务；测试与实现同目录 | 是 |
| `modules/` | 功能模块、共享原生组件及其测试 | 是 |
| `tests/` | 正式集成测试、探针与脱敏夹具 | 是 |
| `contracts/` | 跨语言协议说明与样本 | 是 |
| `build/` | 可复现构建、验证、打包脚本 | 是 |
| `.github/` | CI、PR 和 Issue 模板 | 是 |
| `docs/` | 按主题组织的架构、维护及用户文档；由 `docs/README.md` 按需导航 | 是 |
| `third_party/` | 复用代码、来源锁定与许可证 | 是 |
| `_temp/<任务>/` | 临时脚本、构建、候选包、测试输出、旧备份、一次性工具与下载、截图和日志 | 否 |
| `dist/release/` | 唯一正式安装器、SHA-256 与 release-info.json | 否 |
| `dist/reports/` | 可直接查看的交付报告及筛选后的验收证据；HTML 优先自包含，报告标明对应版本和验证范围 | 否 |
| `dist/logo/` | 反复使用的 Logo 设计资源与多尺寸导出 | 否 |
| `dist/resources/` | 当前使用的本地工具链、源码依赖、OCR 运行时、图标及复用缓存入口 | 否 |
| `node_modules/`、工具链生成的 `bin/`、`obj/`、`target/` | 依赖与编译缓存；Rust 的 `src/bin/` 源码入口仍入库 | 否 |

根目录仅保留项目入口、许可证与工具配置。正式回归测试必须入库；个人运行数据、凭据与真实路径不得成为测试夹具。

当前设置页与共享控件位于 `apps/desktop-mygo/pages/`、`kit/` 和 `widgets/`，结果界面位于 `apps/result-ui/`，跨宿主原生组件位于 `modules/nativeui/`。`apps/desktop/frontend` 仅维护保留的兼容网页路径，不作为新设置页的默认入口。

有效约定从 [AGENTS.md](../AGENTS.md) 的主题索引读取；设计、实现和历史资料从 [docs/README.md](README.md) 查找。旧架构计划、迁移步骤和一次性交付决定不作为当前规则。`docs/development.md` 记录状态与验证范围，不重复维护另一套约定。

## 构建与测试入口

项目 Logo 源图统一放在 `internal/branding/logo.png`。更换此文件后运行统一构建，即可更新界面与 Windows 图标。

图标构建会生成包含 16、20、24、32、40、48、64、96、128、256 像素的 ICO，以及原生菜单使用的 64 像素 PNG。两者均不入库。需要独立 PNG 时运行 `go run ./build/icon -native-png internal/branding/logo-native.png -png-dir dist/logo/generated`，另含 512、1024 像素版本；源图保留透明背景和原始比例。

默认运行文件构建到 `_temp/build/current/`，压缩包输出到 `_temp/package-build/output/`，两者分开。需要并行保留候选版本时，使用 `build/build.ps1 -OutputDirectory _temp/<任务>/build`；输出目录必须位于本仓库根目录 `_temp/<任务>/`，且已被 Git 忽略。

单独构建程序管理使用 `build/build-wintray.ps1 -OutputDirectory <候选目录>`；加 `-RunConsoleChecks` 验证真实控制台的输入输出、显示和隐藏。三模块联动使用 `build/test-module-lifecycle.ps1 -BundleDirectory <完整候选目录> -OutputDirectory _temp/<新目录> -RunSystemChecks`，只操作专用测试窗口和生成图片。

程序管理中依赖前台激活的完整原生启动流程仅在显式设置 `WINTRAY_STARTUP_NATIVE_TEST=1` 后运行；依赖 Explorer 的任务栏流程继续使用 `WINTRAY_STARTUP_TASKBAR_TEST=1`。默认源码检查不选择这些系统交互测试。运行时仍须遵守私有桌面与用户前台授权约定；未运行的前台／任务栏流程不能记录为已通过。

默认检查不运行联网测试；`WINONE_NETWORK_TESTS=1` 仅在用户要求联网验证后启用，专用开关、隔离和未覆盖范围见 [默认测试范围](working-rules/architecture-performance.md#默认测试范围)。源码检查入口为 `build/test-source.ps1`，离线通过不等于真实供应商验收通过。

本机连接与消息边界可单独运行 `build/test-transport.ps1 -GoExe <go.exe> -OutputDirectory _temp/<任务>/transport`；加 `-DesktopGoExe <设置窗口所用 go.exe>` 同时检查设置客户端与后台的等待期限。该入口检查真实命名管道的慢回复、业务错误、超时、取消、空闲连接和帧大小，不创建网络监听或桌面窗口。

依赖使用锁文件；CI 从干净 checkout 构建。每次提交前检查 `git status --short`、`git diff --cached` 和 `git ls-files`。不要使用无差别暂存来掩盖文件分类问题。

沿用当前分支，创建新分支须获用户授权；开发分支命名为 `feat/<主题>`、`fix/<主题>`、`chore/<主题>`。发行版本采用 `v主版本.次版本.补丁`，预发布加 `-alpha.N` 或 `-beta.N`；发布 tag 只指向验证通过的提交，本地调试不创建发布 tag。

远端创建、推送和正式发布按用户授权执行。发布时一起提供压缩包、SHA-256、对应源码与第三方声明；包内不含本地测试资料和开发工具链。

第三方来源、依赖清单和摘要保存在仓库，构建仍核验完整记录。`build/bundle-notices.mjs` 根据实际二进制、运行库和采用的资源选择随包声明，保留原文并按内容去重，输出 `licenses/THIRD-PARTY-NOTICES.txt` 与 `licenses/THIRD-PARTY-LICENSES.txt`。精简步骤发生在校验之后，运行时使用的模型、插件和安装清单继续独立校验。

## 交付检查范围

交付前按改动范围核对源码测试、编译与静态检查、来源与许可证、版本、包清单、安装器/卸载器对应关系及 SHA-256，并检查干净提交与实际用户授权。

Windows 专项验收与代码签名不作为发布前置条件；本地测试按影响范围选用并如实报告结果，完整边界只在 [Windows 本地验收与发布策略](working-rules/architecture-performance.md#windows-本地验收与发布策略) 维护。仅修改文档或归档资料不重建安装器。

### Windows 本地测试能力

以下入口用于本地排查与回归，遵守显式开关、隔离及授权约定。

| 测试范围 | 现有入口 |
| --- | --- |
| 原生窗口关闭与进程回收 | [test-desktop-mygo-close.ps1](../build/test-desktop-mygo-close.ps1) |
| AI 服务离线原生窗口、主题与尺寸 | [test-ai-native-windows.ps1](../build/test-ai-native-windows.ps1) `-OfflineUI -OutputDirectory _temp/<任务>/native`；真实渲染使用脱敏 Host 夹具，不启动 AI 网关或访问供应商 |
| 模块启停、快捷键与资源恢复 | [test-module-lifecycle.ps1](../build/test-module-lifecycle.ps1) |
| 安装、覆盖、回退、修复与卸载 | [test-installer.ps1](../build/test-installer.ps1)，用法见[打包与安装](打包与安装.md) |
| Defender 本地扫描 | [verify-windows-security.ps1](../build/verify-windows-security.ps1)；构建时也可显式使用 `-VerifyWindowsSecurity` |
| 设置窗口启动与资源测量 | [benchmark-native-startup.ps1](../build/benchmark-native-startup.ps1)、[benchmark-desktop.ps1](../build/benchmark-desktop.ps1) |

## 外部项目引入与即时许可检查

每次引入或更新外部项目都把“能力接入 + 来源锁定 + 直接/传递依赖许可 + 即时验证”当成同一项工作。仅用于调研且未复制的资料也记录准确来源；复制的实现、设计令牌、图标、字体、模型与二进制不能只写一个链接。

1. 修改依赖锁/复制源码前核对上游许可、版权、NOTICE 及分发条件；锁定版本、提交与校验摘要。GPL/AGPL 等须保留对应源码与组合分发义务说明，不擅自改写 WinOne 根许可证。
2. 同次改动更新 `third_party/sources.json` 或对应主题来源文件、许可全文、依赖清单与 `THIRD-PARTY-NOTICES.md`。缺失必须立即查证；禁止伪造、用“未知”放行或等全量构建才发现。
3. 前端依赖运行 `node build/verify-frontend-notices.mjs --update` 后立即不带 `--update` 复验。Rust 子模块使用 `build/verify-diagram-notices.ps1 -Runtime <diagram|selection|recording> -CargoExe <cargo> -Update`，再只读复验；桌面 Rust 继续用 `verify-rust-notices.ps1`（含 SDK 原包核对）。新复制项目使用 `node build/verify-import-notices.mjs` 校验归属、许可全文与源文件摘要。Go/npm 源码依赖继续运行 `build/verify-notices.ps1`；OCR/AI/结果窗按各自清单与实际锁更新，产物阶段的校验只是第二道门，不替代接入时核对。
4. 升级后保留本地改编说明。移除依赖时只更新已确认不再使用的记录，不删除来源不明的旧资料。

## AI 产物的存放与资料保护

- 按顶部目录表存放 AI 生成内容；可复用资源优先使用现有位置，新增时记录用途和调用方式。该表不是用户资料白名单：来源不明的文件保留，不因位置、名称、类型或未登记而移动、覆盖、删除。
- 候选安装器默认放 `_temp/installer-build/output/`，验证后更新固定安装器及校验/发布元数据，旧包保留在任务临时目录。Logo 设计导出放 `dist/logo/`，构建源仍只有 `internal/branding/logo.png`。
- 脚本用 `build/assert-temporary-path.ps1` 验证生成目标：普通输出为 `_temp/<任务>/`，显式 `-ReusableCache` 允许仓库根 `dist/` 下的固定缓存子目录。只验证生成目标，不扫描或清理用户资料；工具链必需的标准输出目录继续生成并忽略。

仓库检查运行 `./build/verify-repository.ps1`，其中已包含 `./tests/repository-hygiene.ps1` 的输出边界回归，不必重复执行。两者不自动删除文件；清理先核对真实路径、来源和授权，删除进入回收站。

## 可重复使用的本地资源

需要仅编译时使用统一构建的 `-CompileOnly`，保留静态、来源许可和包清单检查，不能将其记为运行测试通过。学习离线测试用 `build/test-learning-offline.ps1`；真实安装、账号同步、音频设备和外网验证单独选择并隔离。

英语学习词库固定放在 `dist/resources/assets/learning/`，包括 `ecdict.db`、`manifest.json` 和 `ECDICT-LICENSE.txt`。锁定原始 CSV 保存在 `dist/resources/sources/ecdict-39cfb96/`，来源与摘要见 `third_party/learning-data.json`。`build/prepare-learning.ps1 -PythonExe <已有 Python> -Downloader <aria2-next>` 可校验或重建；统一构建自动调用，`build/copy-learning.ps1` 将验证过的三件套放入 `shared/ui/learning/`。数据与工具不入库，生成器、测试、许可和来源记录入库。缓存不一致时脚本报错，先在任务临时目录重建并验证，再更新固定资源。

资源入口为 `build/use-local-resources.ps1`，版本和摘要以源码锁、运行时清单及工具自报版本核对。下载、安装或生成前先查已有资源；只有版本缺失、校验失败或明确升级时才补齐，不因新任务重复准备。新增资源归入固定目录，同步来源、版本、摘要、调用方法及下表。

2026-11-28 新一轮 AI 基座更新锁定 Magpie `0d310af00f2934b4331609251340e7336a1f9d0c`、账号适配器 `46ba1e4eb4b26c4b4993709e78e7841562fa5c41`，见 `docs/performance-ai-20261128.md`。2026-11-20 的固定来源 `f65fe92`、`ae9dba9` 留作回退；WinTray `89c246a0b0c2808d7d594ab91758468517d61fdd`、MyGo 正式 `v0.10.0`（`ebde9399baf2734b3361856a53f4036859973d1d`）本轮不变。后续构建只核对本轮锁定源码、许可、编译和产物，不在构建时自动刷新上游。

| 固定子目录 | 用途与维护方式 |
| --- | --- |
| `tools/go-1.27.3` | 本机统一构建 Host、模块、MyGo 设置/结果窗、AI 与 Go 互传；旧 `go-1.27.1` 保留用于回退。运行声明保留 Go 1.26.2 的兼容记录 |
| `tools/rust-1.99.0`、`tools/nsis-3.13`、`tools/bun-1.4.2` | Rust 编译、安装器编译、AI 账号适配器运行时；Bun SHA-256 按 `third_party/ai-plugins.json` 校验 |
| `tools/playwright-1.58.2` | 可复用的浏览器测试依赖；版本及完整性由 `build/browser-tools/package.json`、`package-lock.json` 锁定。入口设置 `WINONE_PLAYWRIGHT_MODULE`，现有布局检查直接使用；不包含浏览器安装包 |
| `tools/node-24.16.0` | 构建脚本与保留网页前端使用 Node 24.16.0 / npm 11.13.0；`build/use-local-resources.ps1` 经 `NodeExe` 传入统一构建。官方 win-x64 ZIP SHA-256：`edaca9bd58ec8e92037dac4e877d52f6b8f430b81c18b57e264b4e2fb111cd56`；不修改系统 Node |
| `assets/provider-icons` | 供应商图标的 128×128 PNG、`manifest.json` 和 `inputs.sha256`；由 `build/build-provider-icons.ps1` 校验并复用，源图或生成器变化后重建 |
| `sources/PowerToys` | 锁定提交的置顶引擎及其恢复的编译依赖；由准备脚本维护 |
| `sources/ShareX-21.0.0`、`sources/ShareX-v21.0.0.zip` | ShareX 准备脚本的固定源码和归档 |
| `sources/ShareX-native` | 历史截图引擎的独立源码，用于旧格式回归；生产截图不再编译或分发此引擎 |
| `sources/Tools3000-v1.4.11`、`cache/capture-native` | 现用 C++ 截图固定源码与 vcpkg 依赖；来源由 `third_party/tools3000/source.json` 锁定。`prepare-capture-native-resources.ps1 -BuildDependencies` 准备依赖，适配副本与构建目录置于 `_temp/<任务>/` |
| `tools/dotnet-10.0.404`、`cache/nuget-native-capture` | 仅供 OCR 的 NativeAOT 编译；用户不需要安装 .NET。OCR 和测试项目各自锁定 SDK，不改变机器或其他项目 SDK |
| `sources/magpie-e296995`、`sources/ai-plugins-bbd3a2e` | 本轮核对并冻结的 AI 基座与账号适配器来源；构建核对 HEAD、锁文件及原始许可。旧 `magpie-f65fe92`、`ai-plugins-ae9dba9` 及其他历史来源保留用于回退；测试输出回到根 `_temp/`。更新对照见 `docs/performance-ai-20261128.md` |
| `sources/mygo-winone-v0.10.0` | MyGo 正式 v0.10.0 的仓库管理源码副本，按 `third_party/mygo-winone/source.json` 与 `build/prepare-mygo-filedrop.ps1` 校验原模块和窄适配；设置窗口、结果窗及 nativeui 使用该副本，不修改用户 Go 模块缓存。旧版本留作历史/回退参考 |
| `runtimes/rapidocrlite-5.0.0a16` | OCR 官方原生 DLL、PP-OCRv6 small 检测/识别模型和字典，共四个文件。`build/prepare-ocr.ps1 -OutputDirectory _temp/<任务>/ocr -Downloader <aria2-next>` 准备并核对 `third_party/ocr-runtime.json` 的摘要；`copy-ocr.ps1` 只复制清单文件。识别入口为 `WinOne.Ocr.exe` 原生程序。Python 仅用于构建校验、词典重建或生成测试图片，由构建者单独提供 |
| `cache/cargo`、`cache/go`、`cache/npm` | 分别通过 `CARGO_HOME`、`GOCACHE`、`npm_config_cache` 复用 Cargo 依赖、Go 编译和 npm 下载缓存；不放测试报告 |
| `cache/nuget-notices/microsoft.web.webview2.1.0.3800.47.nupkg` | Microsoft WebView2 SDK 原包，供 `build/verify-rust-notices.ps1 -Update -WebView2Package <路径>` 核对 Loader 与原始许可证；来源为 NuGet 官方包，SHA-256 `56c9f26bdd07916a2d1949fb58a5c7e434dfa1173577dca879206050c4e718db` |
| `cache/rust-desktop`、`cache/rust-diagrams`、`cache/rust-selection` | 三个现用 Rust 项目的增量编译缓存；不混入其他实验 target |
| `cache/rust-recording` | 屏幕录制 DLL 及正式录制探针的增量编译缓存，由 `build/build-recording.ps1` 管理；测试视频、GIF、截图与报告不放此处，使用 `_temp/<任务>/` |

从仓库根目录运行以下命令，只选择资源，不启动构建：

```powershell
. ./build/use-local-resources.ps1
```

需要构建时，再执行 `./build/build.ps1 @WinOneBuildParameters -OutputDirectory _temp/<任务>/build`。安装器调用传入 `-NsisExe $WinOneNsisExe`。脚本只设置当前 PowerShell 会话，不修改系统环境变量；核心构建资源缺失时明确报错。Playwright 是可选测试工具，存在时检查固定版本并设置其模块路径。

原生设置窗口的资源/启动诊断分别使用 `build/benchmark-desktop.ps1 -Frontend MyGo` 和 `build/benchmark-native-startup.ps1`，在私有桌面测量真实程序及完整进程组，记录正常退出。`WINONE_DESKTOP_STARTUP_TRACE=1` 只增加实际就绪标记，不预热常驻或改变窗口保护；预算见 [性能约定](working-rules/architecture-performance.md#性能与生命周期)。

### 供应商图标

SVG 原稿已经保存在源码目录 `apps/result-ui/assets/providers/`，来源、版本、许可证及摘要见 `third_party/provider-icons.json`。需要图标时先使用这些本地文件。它们不是每次构建要下载的资源；`dist/resources/assets/provider-icons/` 保存的是从这些 SVG 生成的 PNG。

统一构建会自动调用图标脚本。只准备图标时，在选择本地资源后运行：

```powershell
./build/build-provider-icons.ps1 `
    -CargoExe $WinOneBuildParameters.CargoExe `
    -GoExe $WinOneBuildParameters.GoExe `
    -CacheDirectory $WinOneProviderIconCacheDirectory `
    -WorkDirectory _temp/<任务>/provider-icons
```

脚本核对 SVG、生成器源码、构建脚本、Cargo 锁文件及 PNG 的 SHA-256。缓存有效时直接复用，不调用 Cargo，也不改写内容相同的嵌入文件。缓存缺失或失效时才生成到本次临时目录，成功后更新固定缓存和 `apps/result-ui/assets/provider-bitmaps/`；后者是供 Go/Rust 嵌入的忽略目录。缓存专用摘要不进入应用的资源清单。

准备缓存后运行 `./tests/provider-icon-cache.ps1 -OutputDirectory _temp/<任务>/cache-tests`，可在禁用编译器的隔离副本中检查缓存恢复、重复复用、损坏恢复和失效判断。测试不下载资源。

### 浏览器测试依赖

执行资源入口后，现有布局检查通过 `$env:WINONE_PLAYWRIGHT_MODULE` 使用 `dist/resources/tools/playwright-1.58.2/node_modules/playwright`，不需要在每个 `_temp/<任务>/` 中执行 `npm install playwright`。浏览器可执行文件沿用测试的 `WINONE_CHROMIUM` 参数；页面输出、浏览器临时配置和截图仍放本次 `_temp/<任务>/`。

仅当固定目录缺少依赖时，从入库锁文件恢复；已准备的目录直接使用，不重复执行下面的安装命令：

```powershell
$browserTools = 'dist/resources/tools/playwright-1.58.2'
New-Item -ItemType Directory -Path $browserTools -Force | Out-Null
Copy-Item -LiteralPath build/browser-tools/package.json,build/browser-tools/package-lock.json -Destination $browserTools
npm ci --prefix $browserTools --cache dist/resources/cache/npm --prefer-offline --ignore-scripts --no-audit --no-fund
. ./build/use-local-resources.ps1
```

该命令先使用已有 npm 缓存，只补充缺少的锁定依赖；不运行依赖安装脚本。前端现有 `node_modules/` 和工具链要求的 `bin/`、`obj/` 等目录继续按各自锁文件和标准位置复用。

## 历史贴图翻译构建依赖

旧 C# 截图路线使用 [OpenCV 4.12.0 官方源码归档](https://codeload.github.com/opencv/opencv/tar.gz/refs/tags/4.12.0) 和 `WinOne.PinTranslation.dll`。保留的 `build/test-capture-native.ps1 -Suites pin-translation` 仅用于历史回归。现用原生截图直接编译固定 Tools3000 的原位翻译实现，依赖随 C++ 截图统一准备；入口和验收见 [原生截图迁移](native-capture-20261125.md)。
