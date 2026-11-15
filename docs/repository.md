# 仓库目录与发布约定

| 路径 | 内容 | Git 跟踪 |
| --- | --- | --- |
| `apps/` | Host、现有桌面设置界面及原生操作界面 | 是 |
| `internal/` | Go 公共协议与本地服务；测试与实现同目录 | 是 |
| `modules/` | 各功能模块及其测试，接入时创建 | 是 |
| `contracts/` | 跨语言协议说明与样本 | 是 |
| `build/` | 可复现构建、验证、打包脚本 | 是 |
| `.github/` | CI、PR 和 Issue 模板 | 是 |
| `docs/` | 按主题组织的架构、维护及用户文档；由 `docs/README.md` 按需导航 | 是 |
| `third_party/` | 复用代码版本和许可证，接入时创建 | 是 |
| `_temp/<任务>/` | 临时脚本、构建、候选包、测试输出、旧备份、一次性工具与下载、截图和日志 | 否 |
| `dist/release/` | 唯一正式安装器、SHA-256 与 release-info.json | 否 |
| `dist/reports/` | 可直接查看的交付报告及筛选后的验收证据；HTML 优先自包含，报告标明对应版本和验证范围 | 否 |
| `dist/logo/` | 反复使用的 Logo 设计资源与多尺寸导出 | 否 |
| `dist/resources/` | 当前使用的本地工具链、源码依赖、OCR 运行时、图标及复用缓存入口 | 否 |
| `node_modules/`、`bin/`、`obj/` | 依赖与编译缓存 | 否 |

根目录仅保留项目入口、许可证与工具配置。正式回归测试必须入库；个人运行数据、凭据与真实路径不得成为测试夹具。

前端官方组件统一放在 `apps/desktop/frontend/src/components/ui/`，公共样式放 `src/styles/`，业务页面放 `src/features/`。旧 `src/ui/` 入口迁移期间只保留兼容适配和业务专用组件，不再新增另一套通用控件。

文档先从 `docs/README.md` 按任务选择；根 `AGENTS.md` 只保留常驻边界，功能细则在 `docs/working-rules/`。不要求每次对话加载全部文档、开发状态或历史记录。

产品路线集中在 `docs/WinOne架构与开发计划.md`；UI 依据集中在 `docs/统一体验与设计依据.md`；模块、配置与安装边界集中在 `docs/模块与配置契约.md`。`docs/development.md` 只记录当前状态，历史阶段证据放 `docs/archive/`，不再逐轮追加互相过时的结论。

项目 Logo 源图统一放在 `internal/branding/logo.png`。更换此文件后运行统一构建，即可更新界面与 Windows 图标。

图标构建会生成包含 16、20、24、32、40、48、64、96、128、256 像素的 ICO，以及原生菜单使用的 64 像素 PNG。两者均不入库。需要独立 PNG 时运行 `go run ./build/icon -native-png internal/branding/logo-native.png -png-dir dist/logo/generated`，另含 512、1024 像素版本；源图保留透明背景和原始比例。

默认运行文件构建到 `_temp/build/current/`，压缩包输出到 `_temp/package-build/output/`，两者分开。需要并行保留候选版本时，使用 `build/build.ps1 -OutputDirectory _temp/<任务>/build`；输出目录必须位于本仓库根目录 `_temp/<任务>/`，且已被 Git 忽略。

单独构建程序管理使用 `build/build-wintray.ps1 -OutputDirectory <候选目录>`；加 `-RunConsoleChecks` 验证真实控制台的输入输出、显示和隐藏。三模块联动使用 `build/test-module-lifecycle.ps1 -BundleDirectory <完整候选目录> -OutputDirectory _temp/<新目录> -RunSystemChecks`，只操作专用测试窗口和生成图片。

程序管理中依赖前台激活的完整原生启动流程仅在显式设置 `WINTRAY_STARTUP_NATIVE_TEST=1` 后运行；依赖 Explorer 的任务栏流程继续使用 `WINTRAY_STARTUP_TASKBAR_TEST=1`。默认源码检查不选择这些系统交互测试。运行时仍须遵守私有桌面与用户前台授权约定；未运行的前台／任务栏流程不能记录为已通过。

默认源码检查和完整构建不执行联网测试，包括本地 HTTP/TCP/UDP 服务、网关、登录回调和互传。Go 测试使用 `WINONE_NETWORK_TESTS=1` 作为显式选择条件；只有用户要求联网验证后才设置，其他专用测试开关仍然有效。FileShare 的真实端口测试默认忽略，其余不创建网络连接的路由、文件与协议检查照常运行。离线检查通过不能记录为联网或真实供应商验收通过。

本机连接与消息边界可单独运行 `build/test-transport.ps1 -GoExe <go.exe> -OutputDirectory _temp/<任务>/transport`。该入口检查真实命名管道的慢回复、业务错误、取消、空闲连接和帧大小，不创建网络监听或桌面窗口。

依赖使用锁文件；CI 从干净 checkout 构建。每次提交前检查 `git status --short`、`git diff --cached` 和 `git ls-files`。不要使用无差别暂存来掩盖文件分类问题。

开发分支采用 `feat/<主题>`、`fix/<主题>`、`chore/<主题>`。发行版本采用 `v主版本.次版本.补丁`，预发布加 `-alpha.N` 或 `-beta.N`。发布 tag 只能指向验证通过的提交；本地调试不得创建发布 tag。

远端创建、推送和正式发布按用户授权执行。发布时一起提供压缩包、SHA-256、对应源码与第三方声明；包内不含本地测试资料和开发工具链。

## 交付检查范围

交付前按改动范围核对源码测试、编译与静态检查、来源与许可证、版本、包清单、安装器/卸载器对应关系及 SHA-256，并检查干净提交与实际用户授权。

Windows 相关验收保留为本地测试能力，可按任务影响范围使用；将其设为发布阻断条件的策略已彻底移除，具体见[Windows 本地验收与发布策略](working-rules/architecture-performance.md#windows-本地验收与发布策略)。提交、推送、固定安装器更新或发布不要求 Windows 版本/硬件/权限矩阵、真实桌面、干净机器、安装生命周期或 Defender 的通过报告，也不要求 SmartScreen 信誉、Windows 代码签名或微软样本复核。未运行、未覆盖、环境不可用和未通过的验收结果如实记录，不自动转为发布阻断或必做补验。

### Windows 本地测试能力

这些入口可用于本地排查与回归，结果保留在各自报告中，不作为发布准入凭据。系统交互和联网测试继续遵守现有显式开关、隔离及授权约定。

| 测试范围 | 现有入口 |
| --- | --- |
| 原生窗口关闭与进程回收 | [test-desktop-mygo-close.ps1](../build/test-desktop-mygo-close.ps1) |
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

以下约定针对 AI 生成和整理文件的操作，不限制用户手动放入 `dist/` 的资料。不得设置 `dist/` 的目录或文件白名单，也不得仅凭位置、名称、类型或是否登记来移动、覆盖或删除资料。来源不明时保留，清理前核对归属及本次授权。

- AI 的临时脚本、构建与候选包、测试输出、日志、截图及一次性下载放根目录 `_temp/<任务>/`。正式测试源码和脱敏夹具仍放源码目录或 `tests/` 并入库。
- 可复用的工具、依赖、运行时、素材和缓存放在 `dist/` 下用途明确、便于再次调用的固定子目录。优先复用现有位置，新增时记录用途和调用方式，不需要修改允许目录清单。下方资源表记录当前布局，不限制新增目录或用户资料。
- 正式安装器继续使用 `dist/release/WinOne-setup.exe`，同步校验文件和发布元数据；候选安装器默认输出到 `_temp/installer-build/output/`，完成与改动范围相称的检查后再更新固定安装器。此约定不限制用户在相同目录保存其他资料。
- 当前 `dist/logo/` 保存设计原稿及多尺寸导出；正式构建源图仍为 `internal/branding/logo.png`，不创建第二份构建来源。
- 构建、测试和准备脚本复用 `build/assert-temporary-path.ps1` 检查其生成目标。普通临时输出使用 `_temp/<任务>/`；显式 `-ReusableCache` 允许可复用缓存使用 `dist/` 下选定的固定子目录。这些检查不扫描或清理用户已有资料。必要的编译器标准目录（如 `bin/`、`obj/`、前端 `dist/`）继续按工具链规则生成并忽略；本条的 `dist/` 指仓库根目录。

仓库检查：`./build/verify-repository.ps1`；临时输出边界回归：`./tests/repository-hygiene.ps1`。两者只检查，不自动删除文件。清理先核对真实路径和来源；旧证据可以迁到 `_temp/<任务>/` 并记录原路径，删除则进入回收站。

## 可重复使用的本地资源

内置 Anki 26.09.4 的固定资源为 `dist/resources/sources/anki-26.09.4/`（锁定的 Python 包、官方源码、卡片渲染包与对应第三方源码归档）和 `dist/resources/runtimes/anki-26.09.4/`（校验后的库、渲染资源与源码）。版本、下载来源与摘要见 `third_party/anki/source.json`。用 `python build/prepare-anki.py --archives dist/resources/sources/anki-26.09.4 --output dist/resources/runtimes/anki-26.09.4` 准备；用 `python build/verify-anki.py --runtime dist/resources/runtimes/anki-26.09.4` 只读核对。

对应第三方源码使用 `anki-third-party-sources.tar.xz`，由 `build/prepare-anki-sources.py --inputs <锁定源码目录或旧源码ZIP> --output <输出目录>/anki-third-party-sources.tar.xz` 重现。先核对全部原始 gzip 归档的 SHA-256，再完整保留解压后的 tar 字节，用一个 XZ 流统一压缩；不删源码、许可或构建资料。归档内 `SOURCES.json` 记录原始归档和 tar 字节摘要，`verify-anki.py` 逐项回读。修改压缩方式时先在 `_temp/<任务>/` 生成并验证，再更新来源摘要、固定资源及运行文件清单；旧资源保留为回退副本。

统一构建经 `build/copy-anki.ps1` 将这些文件和 WinOne 适配代码放入 `shared/anki/`。Python 解释器复用包内 `capture/Ocr/python/`，不安装系统 Python，不调用外部 Anki。新增运行文件后用 `build/update-anki-inventory.py` 更新完整载荷清单；整个集合服务在首次使用时启动，空闲释放，数据在 WinOne 的学习档案中持久化。

用户要求无人值守且不允许系统/联网测试时，可显式使用统一构建的 `-CompileOnly`。该选项保留编译、静态、来源、许可和包清单检查，关闭自动运行的交互和服务测试；不能把编译通过记为运行测试通过。学习功能另用 `build/test-learning-offline.ps1` 检查隔离真实引擎与原生组件。实际安装、真实账号同步、麦克风/扬声器与外部网络验证按各自隔离和授权约定单独进行；Windows 本地验收不列为发布前置条件。

英语学习词库固定放在 `dist/resources/assets/learning/`，包括 `ecdict.db`、`manifest.json` 和 `ECDICT-LICENSE.txt`。锁定原始 CSV 保存在 `dist/resources/sources/ecdict-39cfb96/`，来源与摘要见 `third_party/learning-data.json`。`build/prepare-learning.ps1 -PythonExe <已有 Python> -Downloader <aria2-next>` 可校验或重建；统一构建自动调用，`build/copy-learning.ps1` 将验证过的三件套放入 `shared/ui/learning/`。数据与工具不入库，生成器、测试、许可和来源记录入库。缓存不一致时脚本报错，先在任务临时目录重建并验证，再更新固定资源。

目前整理的复用资源位于 `dist/resources/`，不入库。这是现有调用位置，不是允许目录白名单。版本以源码锁、运行时清单和工具自报版本核对；更新这些资源时同步这里及 `build/use-local-resources.ps1`。

下载、安装依赖或生成资源前，先查下表、源码中的已有资源和 `build/use-local-resources.ps1`。版本及摘要一致时直接使用；不能因为新建了任务目录就再下载、再安装或再生成一份。只有所需版本缺失、校验失败或明确更新版本时才补齐资源。新增的可复用内容在本次任务内归入固定目录，并同步来源、版本、校验依据和调用方法。

| 固定子目录 | 用途与维护方式 |
| --- | --- |
| `tools/go-1.26.2`、`tools/go-1.27.3` | 分别用于 Host/模块和 MyGo 设置/结果窗；AI 与 Go 互传复用后者；旧 `go-1.27.1` 保留用于回退 |
| `tools/rust-1.99.0`、`tools/nsis-3.13`、`tools/bun-1.4.2` | Rust 编译、安装器编译、AI 账号适配器运行时；Bun SHA-256 按 `third_party/ai-plugins.json` 校验 |
| `tools/playwright-1.58.2` | 可复用的浏览器测试依赖；版本及完整性由 `build/browser-tools/package.json`、`package-lock.json` 锁定。入口设置 `WINONE_PLAYWRIGHT_MODULE`，现有布局检查直接使用；不包含浏览器安装包 |
| `tools/node-24.16.0` | 构建脚本与保留网页前端使用 Node 24.16.0 / npm 11.13.0；`build/use-local-resources.ps1` 经 `NodeExe` 传入统一构建。官方 win-x64 ZIP SHA-256：`edaca9bd58ec8e92037dac4e877d52f6b8f430b81c18b57e264b4e2fb111cd56`；不修改系统 Node |
| `assets/provider-icons` | 供应商图标的 128×128 PNG、`manifest.json` 和 `inputs.sha256`；由 `build/build-provider-icons.ps1` 校验并复用，源图或生成器变化后重建 |
| `sources/PowerToys` | 锁定提交的置顶引擎及其恢复的编译依赖；由准备脚本维护 |
| `sources/ShareX-21.0.0`、`sources/ShareX-v21.0.0.zip` | ShareX 准备脚本的固定源码和归档 |
| `sources/ShareX-native` | 现用原生截图引擎的独立源码；需要测试改写时再复制到 `_temp/<任务>/` |
| `sources/magpie-ce2ce86`、`sources/ai-plugins-a410afe` | 本次核对并冻结的 AI 基座与账号适配器来源；构建核对 HEAD、锁文件及原始许可。旧来源保留用于回退；测试输出回到根 `_temp/`。更新对照见 `docs/ai-base-update-20261113.md` |
| `sources/mygo-winone-v0.9.0` | MyGo 正式 v0.9.0的仓库管理源码副本，按 `third_party/mygo-winone/source.json` 与 `build/prepare-mygo-filedrop.ps1` 校验原模块和窄适配；设置窗口、结果窗及nativeui使用该副本，不修改用户Go module cache。旧版本留作历史/回退参考 |
| `runtimes/ocr` | OCR Python、离线依赖与模型，供反复打包；以 OCR 清单校验 |
| `cache/cargo`、`cache/go`、`cache/npm` | 分别通过 `CARGO_HOME`、`GOCACHE`、`npm_config_cache` 复用 Cargo 依赖、Go 编译和 npm 下载缓存；不放测试报告 |
| `cache/nuget-notices/microsoft.web.webview2.1.0.3800.47.nupkg` | Microsoft WebView2 SDK 原包，供 `build/verify-rust-notices.ps1 -Update -WebView2Package <路径>` 核对 Loader 与原始许可证；来源为 NuGet 官方包，SHA-256 `56c9f26bdd07916a2d1949fb58a5c7e434dfa1173577dca879206050c4e718db` |
| `cache/rust-desktop`、`cache/rust-diagrams`、`cache/rust-selection` | 三个现用 Rust 项目的增量编译缓存；不混入其他实验 target |
| `cache/rust-recording` | 屏幕录制 DLL 及正式录制探针的增量编译缓存，由 `build/build-recording.ps1` 管理；测试视频、GIF、截图与报告不放此处，使用 `_temp/<任务>/` |

从仓库根目录运行以下命令，只选择资源，不启动构建：

```powershell
. ./build/use-local-resources.ps1
```

需要构建时，再执行 `./build/build.ps1 @WinOneBuildParameters -OutputDirectory _temp/<任务>/build`。安装器调用传入 `-NsisExe $WinOneNsisExe`。脚本只设置当前 PowerShell 会话，不修改系统环境变量；核心构建资源缺失时明确报错。Playwright 是可选测试工具，存在时检查固定版本并设置其模块路径。

普通新进程原生设置窗口资源/启动的可选诊断入口分别为 `build/benchmark-desktop.ps1 -Frontend MyGo` 和 `build/benchmark-native-startup.ps1`；可按性能排查和回归需要运行，不作为发布前置条件。默认只在仓库私有桌面测量真实生产EXE，不切换前台；完整进程及正常退出被记录。旧WebView测量默认仍要求实际renderer，不把无WebView的MyGo误判缺进程。可选 `WINONE_DESKTOP_STARTUP_TRACE=1` 只产生实际Host/UI回调/绘制后的就绪标记，不预热常驻、不改窗口保护。2026-11-07当前启动300ms目标未达标且获准不阻断本次交付，具体数据见 [本次验收](2026-11-07-native-local-delivery.md)。

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
