# Chat 后端与 Magpie 验收

范围：Chat 标题生成、工作区链接浏览，以及共享 AI 适配器的上游更新。本记录不替代主窗口、安装包和真实账号的整体验收。

## 来源与构建

- 2026-10-13 实查 `https://github.com/yetone/magpie.git` 的远端 HEAD：`0bb0781ea065b5236a9a0014bceb84cb84324895`，提交时间 `2026-10-13T09:59:15+08:00`。
- 使用 `dist/resources/sources/magpie-0bb0781` 的独立源码目录，保留此前 `magpie` 等来源及其修改。`build/use-local-resources.ps1` 已指向新来源。构建仍读取所选来源的 HEAD，不恢复阻塞式版本锁。
- `go.mod`、`go.sum`、MIT LICENSE 与此前登记的 `ede1e32` 相同。依赖快照继续保存在 `modules/ai-magpie/upstream.go.mod`、`upstream.go.sum`。新增当前提交的许可条目，保留前端旧来源许可。
- 适配上游 `CompactFor`，保留模型输出上限、压缩百分比和显式阈值。Codex 账号现在按上游默认用 zstd 发送 `/responses`；测试解码真实压缩请求并核对模型，不关闭压缩。
- 已运行 `build/build-ai.ps1 -Test`，指定上述来源、现有账号插件和 Bun，以 Go 1.27.1 构建。适配器、OAuth 及账号快照测试全部通过。输出为 `_temp/20261013-chat-backend/ai/WinOne.AI.exe`。
- Go 构建信息记录 `v0.1.1220-0.20261013015915-0bb0781ea065+dirty`，其中 dirty 来自本仓库的嵌入补丁。当前 AI 文件 SHA-256 为 `ff1a9487e95b5ab6c1dda5909112f3caacc71e6bbd1ed25fc1c9e571fd1957ac`。编译后 24 个 Go 依赖均有许可记录，全部许可摘要校验通过。

## 功能对照

| 功能 | 接入方式与可观察结果 |
| --- | --- |
| 自动标题 | 直接翻译 Cindy `buildAutoTitlePrompt`；200 字素材放入转义后的 `user_message` 引用边界，完整提示词作为独立请求 |
| AI 重命名 | 直接翻译 `buildRegenerateTitlePrompt`；Host 读取保存的 Pi 当前分支，取开场及最近 8 条 user/assistant 文本，单条上限 300/400 字；不采用前端传入的历史字符串 |
| 标题校验 | 直接翻译 Cindy 输出规则：40 个 Unicode 字符、单行、去包裹引号；拒绝回答角色、解释前缀、Markdown 标题/代码块及提示词回显；失败不截断伪装成功 |
| 标题存储 | 保留 WinOne 标题模型配置、独立 Magpie 请求、手动改名优先和显式重命名冲突处理 |
| 文件树链接 | Windows junction 及目录 symlink 按实际目录分类，返回工作区内的链接别名，进入和返回路径保持可追踪 |
| 文件预览 | 用 Windows 句柄解析目标，解决 `EvalSymlinks` 对当前 junction 文件路径的失败；链接可读，工作区外目标及附件副本只读 |
| 访问边界 | 工作区外的直接路径继续拒绝；HTTP 授权同时记录链接别名和实际目标，换目标后旧 URL 返回 403 |

Cindy 来源为已登记的 `a152497ac24d155dd46142b73dda4fa10ad08ad0`；文件、摘要、许可和 Go 移植说明见 `third_party/cindy/title-source.json`。自动标题仍遵守 WinOne 已有触发及错误策略，不复制 Cindy 的静默失败策略。字符截断采用 Unicode code point，避免切断代理对。

## 验证和限制

- `go test ./internal/chat -count=1` 已通过。新增测试覆盖标题引用边界、当前分支过滤、开场与最近窗口、输出拒绝规则、真实 junction 导航/读取、外部直接路径拒绝、只读和预览授权失效。
- Host 使用真实新构建 Magpie 与本地 HTTP 测试服务，已验证已保存标题模型、凭据和 Cindy 提示词贯通。不会访问真实外部模型服务。
- `go test ./apps/host -run 'TestTitleGenerationUsesSavedTitleModelThroughRealMagpie|TestProjectPermissionThroughHost' -count=1 -v` 已通过；标题测试同时覆盖自动标题和从磁盘取材的显式重命名。
- `TestWorkspaceFileSymlinkPreservesAlias` 是真实 Windows 文件 symlink 测试；当前进程缺少创建权限，明确跳过。目录 junction 测试实际执行并通过。没有修改开发者模式、系统权限或真实工作区链接。
- 本轮 `internal/chat` 默认运行还跳过 6 项需显式 Pi/Bun 包路径的原生进程、迁移、附件及导出集成测试；主代理完整包验收需单独运行这些已有测试。
- 此阶段不宣称真实账号在线调用、完整上游功能、前端所有交互或安装器已验证。全部测试使用隔离目录；未修改真实安装、账号配置，未提交或推送。

过程日志暂保存在 `_temp/20261013-chat-backend/`，供本次整合核查；主代理完成整体交付后按任务清理约定处理。
