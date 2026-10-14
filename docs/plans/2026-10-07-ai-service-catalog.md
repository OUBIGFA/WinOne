# AI 服务目录实施计划

> 按项目约定由当前 Agent 执行，不使用子代理，不创建提交或对外发布。按步骤完成实现与验证。

**目标：** 接通 magpie 内置账号、社区账号插件及服务预设，修复目录加载误报，并统一未知模型图标和窗口外框。

**架构：** WinOne Host 管理公共配置、任务和逐供应商隔离目录；magpie 的 provider/plugin 层管理实际连接；共用短命 Worker 与共享结果宿主。目录读取无联网和安装副作用，连接使用锁定的本地插件资源。

**技术：** Go Host/Worker、magpie provider/plugin、共享 Bun、React/Base UI/Lucide、MyGo。

**设计：** [AI服务完整目录](../AI服务完整目录.md)。

## 1. 既有加载与外框

- [x] `ai-settings.test.tsx` 用延迟目录响应复现父子组件争锁；`ai-accounts.tsx` 区分父组件传入的空目录与独立加载，真实页面无误报。
- [x] 未知模型的 React 图标及 `build/generate-result-icons.mjs` 均改为 Sparkle，重新生成共享 SVG。
- [x] `reading_surface_test.go` 扩展到 OCR、翻译及划词工具栏，检查四边四角，先失败后通过。
- [x] `window_surface.go` 统一客户端表面；主入口为工具栏和结果窗调用同一 Windows 装饰设置。
- [x] 原生验证工具栏外框、深浅色、拖动、展开为结果窗及缩放；审查其他入口的重复边框并记录实际范围。

## 2. 上游和目录

- [x] 查询最新主分支与社区目录，保存提交、版本和来源证据。
- [x] `build/build-ai.ps1` 升级到 `290c60a3`；兼容新增的账号模型与用量查询，原协议及隔离 OAuth 适配测试通过。
- [x] 更新 `modules/ai-magpie/README.md`、许可清单和构建材料，核对新依赖。
- [x] 新建 `modules/ai-magpie/catalog.go`、`catalog_test.go` 与版本锁定目录；复用上游预设和账号类型，测试 Cline、OpenCode 与目录覆盖，不依赖已登录账号。
- [x] 将社区插件包及共享 Bun 纳入统一构建，校验摘要与许可证；目录查询不运行插件、不在线抓取组件。

## 3. 登录与请求

- [x] 新建 `modules/ai-magpie/plugins.go`，在服务专属目录加载指定本地插件；拒绝未知包、路径和供应商，进程关闭释放 Bun。
- [x] `accounts.go` 接通 `plugin.Providers`、`NextPrompt`、`Validate`、`StartPluginSignIn`、`PluginAPIKey` 和原回调协议；保存旧账号兼容路径。
- [x] `apps/host/ai_accounts_windows.go` 和 `internal/ai` 传递受限的登录方式与输入字段，仍使用现有所有者、超时及互斥锁。
- [x] `main.go`、`models.go` 对插件账号使用实际 magpie Provider；服务预设使用 `FromPreset`，保留多协议模型行为、模型范围及多密钥策略。
- [x] 用本地插件与回环服务验证登录、模型、生成、流式、取消、错误、隔离及退出；真实厂商账号验收单独记录，不用夹具声称已登录成功。

## 4. 页面和交付

- [x] `ai-settings.tsx`、`ai-configuration.ts` 及双语资源接入完整目录；沿用两组标签、自动名称和现有保存机制。
- [x] `ai-accounts.tsx` 显示插件登录方式、问题和授权输入，复用当前字段、选择器与反馈组件。
- [x] 本地打包所有新增供应商图标；模型图标保持模型本身的标识，未知模型使用单枚四角星。
- [x] 完成前端、Host、Worker、共享结果窗与原生流程回归，记录完整进程组和退出证据。
- [x] 统一构建与文件/许可校验，验证安装器和包清单，在维护锁内保留配置、授权及备份后更新试用版与已安装版。
