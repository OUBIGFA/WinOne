# main 模块精简与验收

## 目标与边界

保存完整版本后，从 main 移除 Chat、OneBot、闪聊。AI 服务仅保留常用、自定义供应商、账号供应商、使用偏好、本地网关。

完整版本分支：`codex/archive-full-20261112`，起点 `bbd7a2166e41f7787d460a5ead13bcb30d5fa42e`，已推送。该分支保留原代码、锁文件、构建入口与历史记录。

主分支直接移除页面、后台入口、专用实现、独占依赖和打包资源。保留截图、OCR、翻译、划词助手、快捷助手、互传、程序管理、便捷功能、学习及全局设置。账号供应商必需的认证适配器与模型协议转换属于保留功能，不作为通用插件平台开放。

旧配置中的已移除入口须可读取并回到首页；旧服务开关不得重新启动已移除服务。测试只用仓库内隔离目录与配置，不改真实安装及用户数据。升级安装须移走旧程序载荷，保留用户配置与历史数据。

## 实施与核查

| 项目 | 实现范围 | 验证结果要求 |
| --- | --- | --- |
| 完整版本留存 | 本地与远端备份分支 | 分支 SHA 与原 main 一致 |
| 三模块删除 | Host 注册/生命周期、原生与旧网页入口、页面及专用工作进程 | 无可调用入口、无独占依赖、无对应新包载荷 |
| AI 功能收口 | 五个标签、配置、请求分派、路由/统计/脱敏/扩展/外部服务 | 保留服务正常保存和调用；已移除 RPC 与 HTTP 路径不可用 |
| 兼容与解耦 | 旧默认页、旧 AI 开关、供应商/密钥/模型、安装升级 | 旧配置不会阻止启动或启用旧服务，保留功能与数据不受损 |
| 构建和来源 | 统一 build 入口、锁文件、许可及载荷清单 | 干净源码可构建，依赖和包清单与实际内容一致 |
| 第一轮审核 | 代码差异、依赖方向、移除范围、配置迁移 | 修复发现并通过相关回归 |
| 第二轮审核 | 原生页面、保留模块、真实隔离 AI 调用与网关 | 实际行为符合范围，关闭/重开和退出正常 |
| 第三轮审核 | 完整候选包、隔离安装/覆盖/卸载、残留与数据保护 | 候选程序及安装器核查通过，无未解决发现 |
| 交付 | main 提交推送、固定安装器及 SHA-256 | 三轮无问题后更新，产物绑定已推送源码 |

## 执行记录

- [x] 核对干净工作区，保存并推送完整版本。
- [x] 完成删除、配置兼容与必要回归。
- [x] 完成第一轮审核。
- [x] 完成第二轮审核。
- [x] 完成第三轮审核。
- [x] 提交推送 main，更新正式安装器及校验文件。

审核结论只在相应流程实际通过后填写；构建成功不代替行为与安装验证。

## 审核证据

### 第一轮：静态范围、依赖和源码门

- `git diff --check`、`node build/verify-import-notices.mjs` 和 `build/verify-repository.ps1` 通过。
- `build/test-source.ps1 -GoExe dist/resources/tools/go-1.27.3/bin/go.exe -OutputDirectory _temp/remove-modules-20261112/source-checks-final` 通过；Host、内部包、保留模块、WinTray 测试和 vet 均通过。
- 活动源码、构建入口和资源中，Chat、OneBot、闪聊路径以及已删除的 `magpie-proxy-fields.ts` 均无引用。历史文档和旧配置兼容测试中的文字只用于追溯或迁移，不是可调用入口。
- AI 原生导航、旧配置读取和新包载荷检查通过；旧服务不会因旧配置重新启用。

### 第二轮：页面、保留模块和真实行为

- `apps/desktop/frontend` 的 `npm test` 通过：101 个测试文件、579 个测试。
- `apps/desktop/frontend` 的 `npm run build` 通过，TypeScript 检查和生产构建完成。
- `apps/desktop-mygo` 使用 Go 1.27.3 执行 `go test ./... -count=1` 通过；覆盖五个 AI 页面、双语字典、设置、截图、划词、互传、程序管理、WinTray 和其他 MyGo 页面。
- `_temp/remove-modules-20261112/candidate-final-20261113b` 的 `build/verify-built-bundle.ps1` 通过；Host、集成测试、依赖与许可清单、结果页、互传、Diagram、Selection、Recording 和 MyGo 载荷核验通过。
- 网关访问密钥、限额、历史读取、模型选择、协议转换、窗口关闭释放和已移除 AI 服务的 RPC/Worker 拒绝测试通过。

### 第三轮：候选包和隔离安装

- `build/package.ps1` 对最终候选包通过，生成 5,759 个文件；解包后的 `shared/chat`、`shared/onebot`、`shared/flashchat` 及旧 AI 路由、隐私、脱敏、会话、用量、搜索目录均为 0。
- `build/test-installer.ps1` 使用 `-TestRollback -TestRepair -TestOverwrite -TestHistoricalJournal -TestSelfContainedRepair -TestUninstallRecovery` 通过；覆盖首装、故障回退、中断修复、覆盖安装、历史日志、自包含修复、卸载恢复和完整卸载。
- 测试安装使用隔离目录、隔离注册表键和隔离配置；未操作真实 WinOne 安装。当前仓库版本和历史测试包均为 `0.0.1`，因此未伪造不同版本号执行升级计划；覆盖安装和修复路径已实际核验。

三轮审核的证据均来自本次工作区和 `_temp/remove-modules-20261112/`。提交 `4eb51e3ca36fcf0dc51210cad2325b97416fb55a` 已推送到 `origin/main`。正式安装器已从最终候选包生成并写入 `dist/release/`：`WinOne-setup.exe` 为 335,957,242 字节，SHA-256 为 `793e4a0829385e210e3c55fb642ad769ea0cbe575c1105c47597439f520febf4`；卸载程序为 2,893,392 字节，SHA-256 为 `8587c2ffd5804229eada76928a0ce879aac865d92ce5cee90cbc0964ef0b43e7`。`build/verify-repository.ps1` 已核对正式安装器哈希、大小、提交和远端 `main` 指向。

正式安装器未签名、未发布，真实用户安装和当前安装器的 Defender 全量复扫仍按 `release-info.json` 标注为未执行；隔离安装、覆盖、回退、修复、卸载恢复和载荷哈希已在测试安装器上完成。
