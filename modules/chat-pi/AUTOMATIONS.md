# Cindy 自动化接入与验证

来源为 `makecindy/cindy@a152497ac24d155dd46142b73dda4fa10ad08ad0`。排期引擎、类型、六个内置模板和英文模板文案保存在 `third_party/cindy/scheduler/`。脚本协议与前置检查由原实现适配，来源及摘要见 `scheduler-source.json`。

WinOne 使用 Pi 作为执行器；供应商、模型、权限和凭据仍由 WinOne 管理。任务的思考强度与 Fast 选择会传入真实请求。供应商是否接受这些参数取决于其模型能力。绑定会话未指定这些选项时，保留会话现有选择。

## 功能对照

| 上游能力 | WinOne 入口与执行 | 已执行的验证 |
| --- | --- | --- |
| Cron、时区、月末日期、预设频率 | 编辑 → 执行时间；原 scheduler/cron/monthlyClamp | 表单预设往返；运行时排期回归 |
| 相对间隔、一次执行、仅手动 | 时间方式；onceAt 适配 | 绝对时间、恢复过期任务、手动运行真实 ID |
| 新会话、持续会话、绑定会话 | 运行会话；Host prepare → Pi | 真实 Pi 两次复用、已删除绑定暂停、持续会话重建 |
| 项目目录、隔离 Git 工作树 | 项目目录与工作树选项 | 真实 detached 工作树；源分支/提交不变；忽略串仓库环境变量 |
| Agent/script、命令、超时 | 执行与检查；独立 Host Job → script-runner.js | 真实脚本、中文结果、环境变量隔离、超时及回收 |
| cindy-script/1 与旧协议 | 原脚本 JSONL 协议适配 | 新旧协议、无 complete、无效 JSON、重复 complete |
| 能力默认拒绝、能力发现 | 全部四项能力选择；host.capabilities | 未授权拒绝、未配置明确不可用、完整七方法目录 |
| Jira get/search/comment | 连接器 → 既有 MCP 服务和工具映射 | 真实本地 MCP 调用；字段、分页、ADF、out_file、参数失败回归 |
| 飞书最近会话/消息 | 连接器 → MCP 方法映射 | 能力授权、count 范围、start_time 与调用参数回归 |
| sessions.dispatch | Host 真实 Pi 会话；绑定目标/宿主设置保护 | 完整 script → Cindy → Host → Pi → 已登记会话；拒绝覆盖权限/目录/绑定 |
| preRunHook | 原 stdin 上下文及退出协议 | 实际 exit 0 继续、2 跳过、7 失败；stdout/stderr/耗时 |
| 六模板、分类、参数、编辑 | 模板入口；原模板及参数替换 | 六模板/三分类、英文原文、必填参数、应用后编辑且不提前创建任务 |
| 用户模板保存、删除、JSON 复制/导入 | 模板管理；SQLite templates | 保存、参数应用、删除；UI 导入/复制路径已接 |
| 项目配置、旧目录兼容 | 项目配置；.cindy/automations/schedules.json，兼容 .xdmaker 读取 | 个人→项目、编辑同步、重载、复制、转个人；重载保留暂停状态 |
| 桌面/飞书/企业微信群通知 | 通知选项；桌面入口及显式 MCP 映射 | 注入的桌面投递错误；两外部渠道参数；投递失败保存到运行记录 |
| 无事静默、主动报告 | schedule_silence / schedule_notify；双重当前轮验证 | 正常静默、主动报告、拒绝错误 request ID、通知失败不丢执行结果 |
| 排队、并发、延迟、恢复 | 原引擎 + SQLite 执行锁 + Host | 活跃锁不被另一进程接管；真实 owner 退出后恢复 |
| 结果、Hook 详情、耗时、状态 | 运行历史与前置检查详情 | 真实结果、错误及结构化 Hook 显示；中英/明暗/窄宽布局 |
| 暂停、恢复、删除、删除后恢复 | 任务操作与已删除任务入口 | 运行中取消；删除保留可恢复历史；恢复保持暂停 |
| 删除记录、已读、分页 | 运行历史操作 | 已读后打开真实会话；历史游标与删除 API |
| 模型、思考、Fast、权限 | 运行会话；Pi 请求与权限管线 | reasoning_effort/service_tier 到达实际模型请求；缺省不重置绑定会话选项 |
| 退出、取消、崩溃 | 每命令独立 Windows Job，按句柄终止并确认 ActiveProcesses=0 | 真实超时回收；调度 worker 失联停止自身 Pi 轮次；HTTP 断开不停止后续用户轮次 |

项目文件语法错误会保留文件、暂停已有项目任务并报告错误。旧目录配置读取后写入新位置时保留旧文件。项目任务的暂停是运行状态，重载不会擅自重新启用。

## 连接器配置

先在 Chat 设置中配置 MCP 服务，再在自动化 → 连接器中填写方法映射。该页只保存服务名称、工具名称及参数映射，不另存凭据。OAuth 和服务配置复用 WinOne Chat 的存储。

例如，已配置名为 `jira` 的 MCP 服务，并且它提供 `jira_issues` 工具时：

```json
{
  "methods": {
    "jira.get": {
      "server": "jira",
      "tool": "jira_issues",
      "arguments": { "action": "get" }
    }
  }
}
```

`parameterMap` 可将协议参数名改为工具实际参数名；例如 `{ "issue_key": "issueKey" }`。`arguments` 是用户设定的固定参数。外部通知映射使用 `notify.feishu`、`notify.wecom`，会传入 `text`。在固定参数中配置接收目标，并启用任务对应通知选项。

脚本能力始终默认关闭。`host.capabilities` 同时报告授权和配置状态。未配置的方法返回明确错误。`out_file` 仅允许写入项目内文件；父目录须存在，目录联接不得越出项目。

脚本遵循 `cindy-script/1` JSONL：stdin 首帧为 start，stdout 只能写 call/complete 帧，普通日志写 stderr。保留旧 `xdt-maker-script/1` 协议。`winone-node script.mjs` 和兼容别名 `xdt-node` 使用随 WinOne 打包的 Bun，不需要机器路径。

## 验证范围

本轮完成 41 项打包 Bun 检查、3 项能力测试、21 项自动化前端测试，以及真实 Host 脚本/MCP/Pi、Git 和绑定选项回归。自动化界面检查覆盖中英、明暗主题及 520/1000 像素宽度。

前序证据：`_temp/20261010-chat-completion/automations-complete/verification.json`、`layout/report.json`。2026-10-10 总构建已重新生成最终 worker、scheduler、script-runner，并通过 41 项打包 Bun 检查、3 项能力测试及 9 项 Host 自动化测试；最终摘要见 `_temp/20261010-takeover-6e0f/verification/chat-runtime.json`。总包已通过隔离安装、覆盖、回滚、修复和卸载检查，不再将前序候选摘要作为最终产物摘要。

真实 Jira、飞书及企业微信账号未被连接，本轮没有发送外部消息。外部方法用本地 MCP/调用夹具验证；需要用户配置账号后才能验收其真实服务。桌面通知显示受 Windows 设置控制，本轮测试投递入口及失败反馈，未触发真实系统通知。总包和安装器状态见 `docs/development.md`。
