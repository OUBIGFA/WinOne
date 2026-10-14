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

依赖使用锁文件；CI 从干净 checkout 构建。每次提交前检查 `git status --short`、`git diff --cached` 和 `git ls-files`。不要使用无差别暂存来掩盖文件分类问题。

开发分支采用 `feat/<主题>`、`fix/<主题>`、`chore/<主题>`。发行版本采用 `v主版本.次版本.补丁`，预发布加 `-alpha.N` 或 `-beta.N`。发布 tag 只能指向验证通过的提交；本地调试不得创建发布 tag。

远端创建、推送和正式发布按用户授权执行。发布时一起提供压缩包、SHA-256、对应源码与第三方声明；包内不含本地测试资料和开发工具链。

## AI 产物的存放与资料保护

以下约定针对 AI 生成和整理文件的操作，不限制用户手动放入 `dist/` 的资料。不得设置 `dist/` 的目录或文件白名单，也不得仅凭位置、名称、类型或是否登记来移动、覆盖或删除资料。来源不明时保留，清理前核对归属及本次授权。

- AI 的临时脚本、构建与候选包、测试输出、日志、截图及一次性下载放根目录 `_temp/<任务>/`。正式测试源码和脱敏夹具仍放源码目录或 `tests/` 并入库。
- 可复用的工具、依赖、运行时、素材和缓存放在 `dist/` 下用途明确、便于再次调用的固定子目录。优先复用现有位置，新增时记录用途和调用方式，不需要修改允许目录清单。下方资源表记录当前布局，不限制新增目录或用户资料。
- 正式安装器继续使用 `dist/release/WinOne-setup.exe`，同步校验文件和发布元数据；候选安装器默认输出到 `_temp/installer-build/output/`，完整验证后再更新固定安装器。此约定不限制用户在相同目录保存其他资料。
- 当前 `dist/logo/` 保存设计原稿及多尺寸导出；正式构建源图仍为 `internal/branding/logo.png`，不创建第二份构建来源。
- 构建、测试和准备脚本复用 `build/assert-temporary-path.ps1` 检查其生成目标。普通临时输出使用 `_temp/<任务>/`；显式 `-ReusableCache` 允许可复用缓存使用 `dist/` 下选定的固定子目录。这些检查不扫描或清理用户已有资料。必要的编译器标准目录（如 `bin/`、`obj/`、前端 `dist/`）继续按工具链规则生成并忽略；本条的 `dist/` 指仓库根目录。

仓库检查：`./build/verify-repository.ps1`；临时输出边界回归：`./tests/repository-hygiene.ps1`。两者只检查，不自动删除文件。清理先核对真实路径和来源；旧证据可以迁到 `_temp/<任务>/` 并记录原路径，删除则进入回收站。

## 可重复使用的本地资源

目前整理的复用资源位于 `dist/resources/`，不入库。这是现有调用位置，不是允许目录白名单。版本以源码锁、运行时清单和工具自报版本核对；更新这些资源时同步这里及 `build/use-local-resources.ps1`。

下载、安装依赖或生成资源前，先查下表、源码中的已有资源和 `build/use-local-resources.ps1`。版本及摘要一致时直接使用；不能因为新建了任务目录就再下载、再安装或再生成一份。只有所需版本缺失、校验失败或明确更新版本时才补齐资源。新增的可复用内容在本次任务内归入固定目录，并同步来源、版本、校验依据和调用方法。

| 固定子目录 | 用途与维护方式 |
| --- | --- |
| `tools/go-1.26.2`、`tools/go-1.27.1` | 分别用于 Host/模块和结果窗；版本固定 |
| `tools/rust-1.99.0`、`tools/nsis-3.13`、`tools/bun-1.4.2` | Rust 编译、安装器编译、AI 插件运行时；Bun SHA-256 按 `third_party/ai-plugins.json` 校验 |
| `tools/playwright-1.58.2` | 可复用的浏览器测试依赖；版本及完整性由 `build/browser-tools/package.json`、`package-lock.json` 锁定。入口设置 `WINONE_PLAYWRIGHT_MODULE`，现有布局检查直接使用；不包含浏览器安装包 |
| `assets/provider-icons` | 供应商图标的 128×128 PNG、`manifest.json` 和 `inputs.sha256`；由 `build/build-provider-icons.ps1` 校验并复用，源图或生成器变化后重建 |
| `sources/PowerToys` | 锁定提交的置顶引擎及其恢复的编译依赖；由准备脚本维护 |
| `sources/ShareX-21.0.0`、`sources/ShareX-v21.0.0.zip` | ShareX 准备脚本的固定源码和归档 |
| `sources/ShareX-native` | 现用原生截图引擎的独立源码；需要测试改写时再复制到 `_temp/<任务>/` |
| `sources/magpie-0bb0781`、`sources/ai-plugins-bcc94e5` | 本次验证的 AI 适配器来源与账号插件锁定源码；Magpie 构建读取所选来源的 HEAD，不固定阻塞版本；测试输出仍回到根 `_temp/` |
| `sources/mygo-3ad015a` | 结果窗锁定版本的源码参考；正式构建按 Go 锁和 overlay 准备 |
| `runtimes/ocr` | OCR Python、离线依赖与模型，供反复打包；以 OCR 清单校验 |
| `cache/cargo`、`cache/go`、`cache/npm` | 分别通过 `CARGO_HOME`、`GOCACHE`、`npm_config_cache` 复用 Cargo 依赖、Go 编译和 npm 下载缓存；不放测试报告 |
| `cache/nuget-notices/microsoft.web.webview2.1.0.3800.47.nupkg` | Microsoft WebView2 SDK 原包，供 `build/verify-rust-notices.ps1 -Update -WebView2Package <路径>` 核对 Loader 与原始许可证；来源为 NuGet 官方包，SHA-256 `56c9f26bdd07916a2d1949fb58a5c7e434dfa1173577dca879206050c4e718db` |
| `cache/rust-desktop`、`cache/rust-diagrams`、`cache/rust-selection` | 三个现用 Rust 项目的增量编译缓存；不混入其他实验 target |

从仓库根目录运行以下命令，只选择资源，不启动构建：

```powershell
. ./build/use-local-resources.ps1
```

需要构建时，再执行 `./build/build.ps1 @WinOneBuildParameters -OutputDirectory _temp/<任务>/build`。安装器调用传入 `-NsisExe $WinOneNsisExe`。脚本只设置当前 PowerShell 会话，不修改系统环境变量；核心构建资源缺失时明确报错。Playwright 是可选测试工具，存在时检查固定版本并设置其模块路径。

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
