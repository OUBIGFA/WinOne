# Cindy 自动化接入与验证

来源为 `makecindy/cindy@b7e46bcbbe41eeee2f16381fef00df61a31a3b8b`（Apache-2.0）。Chat 的自动化页面直接复用 Cindy 的完整渲染端功能 `features/scheduler`（总览、任务列表、运行历史、新建/编辑表单、计划与模型胶囊、模板、删除确认、项目自动化提示），以及 `packages/maker-scheduler`（排期引擎、类型、例行任务、模板）和前置检查生成器。WinOne 只在接缝层替换宿主能力。

## 复用方式

`build/sync-cindy.mjs` 按 `third_party/cindy/scheduler.sync.json` 从固定提交读取原文件（`git show`，不受工作区换行影响），只做三类机械改动：改写导入路径、把 `window.electronAPI` 指向接缝、执行登记过的文本补丁。每个文件都记录上游与落地 SHA-256。

```powershell
node build/sync-cindy.mjs --spec third_party/cindy/scheduler.sync.json --write   # 升级时重新同步
node build/sync-cindy.mjs --spec third_party/cindy/scheduler.sync.json --check   # 构建时校验
```

没有 Cindy 源码副本时，`--check` 只核对已登记摘要，手工改动仍会失败。升级到新提交时修改 `revision` 后执行 `--write`，再查看报错的接缝与补丁。

登记的补丁只表达宿主事实：新任务默认 Pi 引擎；表单宽度不超过窗口，避免横向滚动；Radix 弹层带上 Cindy 主题作用域。文案子集由同一工具从 Cindy 中英文语言包生成，`{{appName}}` 写为 WinOne，涉及 Cindy 云和其他引擎的提示改为 WinOne 措辞，远程设备错误不收录。

## 用户裁决（2026-10-25）

- 完全按 Cindy：表单不提供权限选择，新建的“每次新建 / 持续任务”以完全访问运行；绑定已有任务时沿用该任务的权限。
- “一次”采用 Cindy 语义：在下一个触发点运行一次，不再提供指定日期时间。
- 已有任务保留原权限和原执行时间：升级时一次性为未记录权限的独立任务写入“询问”；已有的指定时间任务继续按原时刻执行。
- 只用 Pi；不接入 Cindy 云、其他电脑和共享供应商。

## 接缝

| Cindy 宿主能力 | WinOne 实现 |
| --- | --- |
| `window.electronAPI.maker.schedule/projectAutomation` | `features/cindy/host/electronAPI.ts` → Host `chat.automations.*`；事件由 Host 长轮询提供，订阅不重放旧事件 |
| 供应商、模型、能力、思考强度、Fast | WinOne AI 服务；模型选择器沿用 Chat 的分组列表（`host/modelSelector.tsx`） |
| 任务、项目、引用解析、归档/删除 | WinOne Chat 目录与 `conversation.update/delete` |
| 路由 | 打开任务 → 切换 Chat 对话；供应商设置 → AI 服务；IM 设置 → OneBot 消息平台 |
| 飞书机器人、企业微信群通知 | 就绪状态取自 Chat 设置的自动化连接器（`notify.feishu`、`notify.wecom`），OneBot 消息平台接入后改由其提供 |
| 脚本能力就绪状态 | 连接器方法映射（`capabilityStatus`） |
| 前置检查测试/AI 生成 | Host 的命令执行路径（Windows Job、随包 Bun）；生成器原样复用，模型调用经 Host AI 服务 |
| worktree 探测、打开目录/项目配置 | Host `detectCwd`、`open`（只允许自动化或项目目录及其 `.cindy/automations/schedules.json`） |
| 侧栏未读索引、全部已读 | Bun 调度存储的 `sidebarIndexRuns`、`markAllRead`，语义对应 Cindy storage |

Host 新增方法同时登记在 Rust 组件桥、Host 只读白名单和 Wails 入口。

## 能力对照

| 上游能力 | 入口 | 验证 |
| --- | --- | --- |
| 总览、空态、模板库、筛选与排序、拖动列表宽度 | Cindy SchedulerPage | 浏览器检查中英明暗、1000/520 宽、空态；组件测试 |
| 新建/编辑、执行方式、计划胶囊（Cron/间隔/手动/一次）、运行会话三态、绑定任务、工作目录与 worktree、模型/思考/Fast、静默、通知、前置检查（测试、AI 生成、浏览）、脚本能力 | ScheduleFormDialog | 上游表单逻辑测试；浏览器表单检查；Host 方法测试 |
| 立即运行、暂停/恢复（运行中确认）、重命名、删除及生成任务的保留/归档/删除 | 列表与详情 | 集成测试；Bun 后端测试 |
| 运行历史、前置检查详情、打开任务、删除记录、未读与失败标记 | RunHistoryPane | 浏览器检查；Bun 索引测试 |
| 项目自动化：重载、编辑写回、提升、克隆为个人、移除或转个人、变更提示 | 列表分组菜单 | Bun 项目条目与 reconcile 事件测试 |
| 引擎：DST 修正、例行任务、删除不存在任务报错、运行位置字段 | maker-scheduler | 原有 18 项与新增 5 项 Bun 测试 |

## 连接器配置

在 Chat 设置的 MCP 页底部填写“连接器”方法映射，复用已配置的 MCP 服务，不另存凭据。

```json
{
  "methods": {
    "jira.get": { "server": "jira", "tool": "jira_issues", "arguments": { "action": "get" } }
  }
}
```

`parameterMap` 把协议参数改名为工具参数。`notify.feishu`、`notify.wecom` 接收 `text`。脚本遵循 `cindy-script/1` JSONL，保留旧 `xdt-maker-script/1`；`winone-node`/`xdt-node` 使用随包 Bun。

## 验证与未覆盖

2026-10-26：前端类型检查无错误；Cindy 上游排期逻辑测试 127 项、宿主集成测试、Chat 相关 250 项通过；Bun 自动化测试 23 项通过；Host Go 测试与 Rust 编译检查通过；浏览器检查 8 个场景无错误、无横向滚动，Chat 外观、工作台与面板布局检查通过。

真实 Jira、飞书、企业微信账号未连接，未发送外部消息。前置检查 AI 生成的真实模型调用、拖入脚本文件取路径（WebView2 不提供文件路径，需用“浏览”）需在实机验证。
