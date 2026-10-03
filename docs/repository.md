# 仓库目录与发布约定

| 路径 | 内容 | Git 跟踪 |
| --- | --- | --- |
| `apps/` | Host、现有桌面设置界面及原生操作界面 | 是 |
| `internal/` | Go 公共协议与本地服务；测试与实现同目录 | 是 |
| `modules/` | 各功能模块及其测试，接入时创建 | 是 |
| `contracts/` | 跨语言协议说明与样本 | 是 |
| `build/` | 可复现构建、验证、打包脚本 | 是 |
| `.github/` | CI、PR 和 Issue 模板 | 是 |
| `docs/` | 架构、开发、维护及用户文档 | 是 |
| `third_party/` | 复用代码版本和许可证，接入时创建 | 是 |
| `_temp/<任务>/` | 本地试验、环境工具、下载、截图和原始日志 | 否 |
| `dist/` | 本地构建与发布产物 | 否 |
| `node_modules/`、`bin/`、`obj/` | 依赖与编译缓存 | 否 |

根目录仅保留项目入口、许可证与工具配置。正式回归测试必须入库；个人运行数据、凭据与真实路径不得成为测试夹具。

前端官方组件统一放在 `apps/desktop/frontend/src/components/ui/`，公共样式放 `src/styles/`，业务页面放 `src/features/`。旧 `src/ui/` 入口迁移期间只保留兼容适配和业务专用组件，不再新增另一套通用控件。

产品路线集中在 `docs/WinOne架构与开发计划.md`；UI 依据集中在 `docs/统一体验与设计依据.md`；模块、配置与安装边界集中在 `docs/模块与配置契约.md`。`docs/development.md` 只记录当前状态，历史阶段证据放 `docs/archive/`，不再逐轮追加互相过时的结论。

项目 Logo 源图统一放在 `internal/branding/logo.png`。更换此文件后运行统一构建，即可更新界面与 Windows 图标。

图标构建会生成包含 16、20、24、32、40、48、64、96、128、256 像素的 ICO，以及原生菜单使用的 64 像素 PNG。两者均不入库。需要独立 PNG 时运行 `go run ./build/icon -native-png internal/branding/logo-native.png -png-dir dist/logo/generated`，另含 512、1024 像素版本；源图保留透明背景和原始比例。

默认运行文件构建到 `dist/build/`，压缩包输出到 `dist/packages/`，两者分开。需要保留运行中的版本时，使用 `build/build.ps1 -OutputDirectory dist/preview-current`；输出目录必须已被 Git 忽略。

依赖使用锁文件；CI 从干净 checkout 构建。每次提交前检查 `git status --short`、`git diff --cached` 和 `git ls-files`。不要使用无差别暂存来掩盖文件分类问题。

开发分支采用 `feat/<主题>`、`fix/<主题>`、`chore/<主题>`。发行版本采用 `v主版本.次版本.补丁`，预发布加 `-alpha.N` 或 `-beta.N`。发布 tag 只能指向验证通过的提交；本地调试不得创建发布 tag。

远端创建、推送和正式发布按用户授权执行。发布时一起提供压缩包、SHA-256、对应源码与第三方声明；包内不含本地测试资料和开发工具链。
