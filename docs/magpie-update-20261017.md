# Magpie 2026-10-17 增量功能对照

来源：674b6ca2881f1289c4ccdec158762978a463320c → 7660399a121af67db4efd56ecb6bd4fb04dc53f1。共 75 提交、310 变化文件。MIT 许可证未变。此表不是完成声明。

各页分别承接对应功能，路由页只承接路由。外部客户端的配置须映射到 WinOne 自有 Chat 和资源功能，不自动写入用户机器。默认构建来源、来源记录与上游 go.mod/go.sum 已同步所选提交；正式安装器尚未切换。

## 逐提交对照

| 提交 | 上游变化 | WinOne 功能与接入位置 | 当前验证与待办 |
| --- | --- | --- | --- |
| `7597c13` | gui: a curl example pasted into a provider's editor fills it in (@mark_li21 on X) | 自定义供应商；`ai-settings.tsx / AI 模型与预设组件` | cURL 草稿解析、密钥不回显及原生 UI 保存通过；不执行示例命令 |
| `a15e576` | agent: OpenPencil uses magpie's models (@JC_be_better on X) | Chat 资源 / 模型 / 上下文；`modules/chat-pi / internal/chat / Chat 页面` | 待逐项核对；不写入外部客户端配置 |
| `107fbbc` | provider: moved Factory users get opencode-factory-auth 0.1.31, whose OpenAI models' requests go without temperature and top_p, not 400 (sweanng424 on Discord; magpie-community/plugins#154) | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `8b84dc9` | gateway: a gateway key held to some models is told of those image and video models alone in /v1/models (塔哥 on Discord) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `d9e87eb` | gateway: clients opt into Claude subscription cache marks and TTLs (#1717) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `e72b6a3` | gateway: preserve cache splits and known 1h writes across updates (#1717) | 用量统计；`usage.go / ai-usage.tsx` | 积分、服务档位、首字时间已接线，验证中；密钥筛选待做 |
| `6b3ee5a` | library: a repository that keeps its skills again under a plugin's folder lists each once (碳碳双键 on Discord) | Chat 资源 / 模型 / 上下文；`modules/chat-pi / internal/chat / Chat 页面` | 待逐项核对；不写入外部客户端配置 |
| `63be477` | gui: TestPanelJoinsFullScreenSpaces checks key status only where the session gives it (#2450) | 上游测试 / 维护；`上游对应测试保留` | 无新增用户开关；相关测试待运行 |
| `e733b07` | plugins: a provider plugin says which service tiers a model serves, and Codex offers them (#2514) | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 新版 31 包已锁定；Efforts/Tiers/Price 真实模型元数据传递已修正并测试，供应商账号实连仍待验收 |
| `ea3baba` | library: TestSkillRelinkKeepsUnreadableCopy holds under load, where the library skill's hash is remembered past its chmod | 上游测试 / 维护；`上游对应测试保留` | 无新增用户开关；相关测试待运行 |
| `19e9fc4` | provider: moved Cursor users get opencode-cursor-auth 0.2.17: a multi-step turn's last step reports its own prompt, a step of many long calls isn't failed as an incomplete batch, and a continuation in a Run's last minute rebuilds instead of a 502 (magpie-community/plugins#156, #155, #157) | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `ef6d5ca` | provider: a model's listed APIs, Cline/Kilo free mark and remote price are looked up, not the whole list copied for each, so 2,500 models' /v1/models is 0.8s again (K.Liu on Discord) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 真实 Worker 2,500 项目录测试与生产原生获取/全选/保存/重读/末项可达通过；端到端获取约 2,766ms，不作上游 0.8s 保证 |
| `e243937` | gateway: a fast pick can be sent as Ultrafast where the account offers it (#1954) | 默认 AI 服务 / 高级设置；`magpie_settings.go / ai-magpie-settings.tsx` | 精简提示和内网白名单已接线，验证中；其他增量待做 |
| `b2764f7` | provider: a check-in sent through a plugin's fetch that got no reply retries in a minute, not half an hour (plugins #149) | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `c5f15ca` | provider: Amazon Bedrock's Mantle endpoint (@marktoown on X) | 自定义供应商；`ai-settings.tsx / AI 模型与预设组件` | Mantle 17 地区 × 3 协议地址及保存生产实窗通过；缺 AWS 凭据，AWS 实连未验收 |
| `686e8a9` | gateway: a moved subscription's failed tool result reaches its plugin marked | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `7ba1b49` | plugin: a plugin's usage resets carry the team's and each window's | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `ee1df7e` | provider: a Factory API key moves to the plugin as its own key account | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `5b6fe16` | provider: a plugin sign-in whose page can't post back takes a pasted key (#2468) | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `53009f8` | provider: every mover's min is the plugin that closes its gap with the built-in | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `ba132d6` | plugin: the GitHub plugin list reads every page of the search, not the top 50 (@HoGee_xxl on X) | 第三方插件管理 / 中间件；`modules/ai-magpie/plugin-host.js / plugins.go` | 管理实窗通过；新版最终回归待做 |
| `fa5295d` | panel: a Manual routing group's model is switched from the tray panel (#2489) | 路由；`prepare-ai-routing.mjs / routing.go` | 新版已抽取；规则和等待专项验收待做 |
| `75245b7` | gui: a plugin's page in the desktop window names wails:// in its policy, so its bridge loads (#2501) | 第三方插件管理 / 中间件；`modules/ai-magpie/plugin-host.js / plugins.go` | 管理实窗通过；新版最终回归待做 |
| `f54e76d` | gateway: a request builds the providers once, and plugin-auth.json and the discovery setting are parsed once a version (#2502) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `9e5fd84` | gui: the tray panel's Hide accounts covers its model picker too (#2509, PaulT on Discord) | 隐私显示；`账号与模型选择器 / ai-magpie-settings.tsx` | 设置已有；新增选择器场景待验收 |
| `d9b9435` | agent: a key of the gateway's own in place of magpie's still reaches magpie (#2510, PaulT on Discord) | Chat 资源 / 模型 / 上下文；`modules/chat-pi / internal/chat / Chat 页面` | 待逐项核对；不写入外部客户端配置 |
| `1bc61b6` | provider: DeepSeek's balance says its granted and topped-up parts under the total (#2511) | 自定义供应商；`ai-settings.tsx / AI 模型与预设组件` | 保存的已启用密钥 → KeyBalances → 额度/余额分项贯通；受控 DeepSeek transport 与零值显示通过；缺真实 DeepSeek 凭据，账号余额实连未验收 |
| `7170b5d` | usage: the plan credits a reply says it cost are summed by model, agent and session (#2512) | 用量统计；`usage.go / ai-usage.tsx` | 积分、服务档位、首字时间已接线，验证中；密钥筛选待做 |
| `ea3dee9` | gateway: a URL magpie didn't choose is downloaded from public addresses only, wherever the request came from | 默认 AI 服务 / 高级设置；`magpie_settings.go / ai-magpie-settings.tsx` | 真实 Worker 图像编辑专项通过：默认拒绝内网、允许后仅接受图片、移除允许后重新拒绝；精简提示实际请求仍待验收 |
| `7882795` | routing: with routing groups off, the Routing page says so and turns them on (bigbenniu on X) | 路由；`prepare-ai-routing.mjs / routing.go` | 新版已抽取；规则和等待专项验收待做 |
| `9b04c4e` | plugin: async gateway request hooks, run in Bun before routing and after the reply (#2118) | 第三方插件管理 / 中间件；`modules/ai-magpie/plugin-host.js / plugins.go` | 管理实窗通过；新版最终回归待做 |
| `83f3316` | library: a pasted repo binds a skill copy identical to its own, so it updates | Chat 资源 / 模型 / 上下文；`modules/chat-pi / internal/chat / Chat 页面` | 待逐项核对；不写入外部客户端配置 |
| `61ca392` | library: a pasted "npx skills add owner/repo --skill x" reads as its repo | Chat 资源 / 模型 / 上下文；`modules/chat-pi / internal/chat / Chat 页面` | 待逐项核对；不写入外部客户端配置 |
| `440f667` | gateway: a 429 for native compaction's one account says the compaction pinned it, not that every account is capped (#2517) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `dc1a9cd` | gui: Usage → Requests lists the first token's time in a column of its own (#2522) | 用量统计；`usage.go / ai-usage.tsx` | 积分、服务档位、首字时间已接线，验证中；密钥筛选待做 |
| `2c72563` | usage: Requests splits and filters by gateway key, with "No gateway key" for the calls none made (#2521) | 用量统计；`usage.go / ai-usage.tsx` | 网关密钥筛选、无密钥分类及真实持久计账已有通过证据；新增余额后的 Host/AI 全量回归通过 |
| `a4fa7bb` | gui: "Waiting for a request" is a core word, and app.js's comment no longer says "New group" (#2586) | 路由；`prepare-ai-routing.mjs / routing.go` | 新版已抽取；规则和等待专项验收待做 |
| `a751f52` | gateway: TestRequestBodyProgressRenewsIdleDeadline leaves a loaded runner ten gaps of room in its idle window (#2567) | 上游测试 / 维护；`上游对应测试保留` | 无新增用户开关；相关测试待运行 |
| `1d6f461` | plugins: the Unofficial, on GitHub section shows while GitHub hasn't answered, looking, with cards still to come | 第三方插件管理 / 中间件；`modules/ai-magpie/plugin-host.js / plugins.go` | 管理实窗通过；新版最终回归待做 |
| `9fcf6a6` | provider: every deprecated built-in moves onto its plugin, and there's no way back | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `f9ceac1` | provider: a move onto the plugin takes every account, and no model holds it back | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `7d7dba3` | provider: removing or turning off a moved subscription's plugin moves nothing back | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `e80fc7d` | tests: TestFailOpen cuts only the spin short, not the throw and the string | 上游测试 / 维护；`上游对应测试保留` | 无新增用户开关；相关测试待运行 |
| `1eba266` | gui: a moved Factory's waiting sign-in has one "Use an API key instead" link, its key import's | 账号供应商 / 随包插件；`modules/ai-magpie/plugins.go / accounts.go / plugin_signin.go` | 源锁更新 31 包；原账号基线通过，完整新版本待验收 |
| `138188a` | readme: 零词元 joins the README sponsors, a logo beside APINEBULA's | 上游文档；`README` | 无产品行为，无需新功能；保留来源与许可证 |
| `ee2a7c7` | sessions: OpenCode child rows no longer crowd the parent out of the trace window | 用量统计；`usage.go / ai-usage.tsx` | 积分、服务档位、首字时间已接线，验证中；密钥筛选待做 |
| `51adf96` | agent: a compat switch set by hand on a DSH model stays through a model refresh (#2504) | Chat 资源 / 模型 / 上下文；`modules/chat-pi / internal/chat / Chat 页面` | 待逐项核对；不写入外部客户端配置 |
| `a2c4aab` | gateway: DSH's session header and compaction mark are recorded, also through a remote magpie (#2505) | 用量统计；`usage.go / ai-usage.tsx` | 积分、服务档位、首字时间已接线，验证中；密钥筛选待做 |
| `1978eab` | agent: Codex's background compaction is turned on from Settings › Long conversations (#2490, #2493) | 默认 AI 服务 / 高级设置；`magpie_settings.go / ai-magpie-settings.tsx` | 精简提示和内网白名单已接线，验证中；其他增量待做 |
| `9228bd4` | library: skills are grouped by source when the skills CLI keeps its lock under $XDG_STATE_HOME (@_yukirock on X) | Chat 资源 / 模型 / 上下文；`modules/chat-pi / internal/chat / Chat 页面` | 待逐项核对；不写入外部客户端配置 |
| `81313b9` | gui: the core script budget makes room for 10-16's core work (#2612) | 上游测试 / 维护；`上游对应测试保留` | 无新增用户开关；相关测试待运行 |
| `ff72eed` | source: the last-source budget test reads which context ended instead of racing sleeps (#2608) | 上游测试 / 维护；`上游对应测试保留` | 无新增用户开关；相关测试待运行 |
| `b7d9616` | gateway: TestMaxRPMGroupPrefersMemberWithRoom checks the route never queued, not a fixed 1s (#2520) | 上游测试 / 维护；`上游对应测试保留` | 无新增用户开关；相关测试待运行 |
| `065fc75` | LESSONS: 10-16: a move no user can undo shipped past #2596, one suite per machine, mover mins at landing | 上游测试 / 维护；`上游对应测试保留` | 无新增用户开关；相关测试待运行 |
| `a69cd07` | gateway: a Chat tool call kept with an empty id goes with one made from its place (#2611, RunMintOn) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `72d6103` | gateway: a Chat tool in MCP's or another API's shape goes upstream as a Chat function tool (#2622) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `ec64543` | claude: a switch gives a Claude subscription's run the agent's instructions as its system prompt, in place of Claude Code's | 默认 AI 服务 / 高级设置；`magpie_settings.go / ai-magpie-settings.tsx` | 精简提示和内网白名单已接线，验证中；其他增量待做 |
| `e903d87` | gateway tests: TestUsageKeepsTimeToFirstToken sends each event after the agent got the last, not a fixed gap (#2575) | 上游测试 / 维护；`上游对应测试保留` | 无新增用户开关；相关测试待运行 |
| `ed0335f` | gateway: an assistant turn cut off before it said anything is left out of what a Chat upstream is sent (#2599) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `742d6c0` | gui: on Linux the window and its desktop entry name each other, and the icon is installed (#2626) | Linux 桌面集成；`上游 internal/gui` | 目标 Windows 11 无对应入口，无需移植 |
| `df3734b` | proc: the login shell's variables count only when exported, and Codex's proxy detail names the variable it read (#2627) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `6f95065` | gateway, codex: a model turned away for an image says why magpie takes it for text-only and where Accepts images is (KimYx0207 on X) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `c1e95f9` | gateway: a key held to accounts is told only of the image and video models they serve (#2579) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `d4a1b9b` | gateway: a </think> inside code no longer ends a reply's thinking; a plugin's text keeps its <think> (#2634) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `8bb9375` | library: another app's MCP server in an agent's config is offered to bring in, not taken for the agent's own (#2635) | Chat 资源 / 模型 / 上下文；`modules/chat-pi / internal/chat / Chat 页面` | 待逐项核对；不写入外部客户端配置 |
| `333c167` | routing: a rule can set the reasoning its model is asked for (#2633) | 路由；`prepare-ai-routing.mjs / routing.go` | 新版已抽取；规则和等待专项验收待做 |
| `ebf3f48` | usage: a reported call takes first_text_ms and flow_ms, so a reasoning reply tells its speed and a burst is held (#2637) | 用量统计；`usage.go / ai-usage.tsx` | 积分、服务档位、首字时间已接线，验证中；密钥筛选待做 |
| `dccfefd` | library: a skill brought in from an app's versioned folder follows the app's updates (#2635) | Chat 资源 / 模型 / 上下文；`modules/chat-pi / internal/chat / Chat 页面` | 待逐项核对；不写入外部客户端配置 |
| `7ea0e04` | import: a Gemini-protocol relay's key comes in on the provider's Gemini endpoint (#2413) | 自定义供应商；`ai-settings.tsx / AI 模型与预设组件` | 导入与展示待补齐 |
| `08400f4` | gateway: a byte cap after the stream has begun says request_too_large, so Pi compacts (#2533) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `ae98cda` | gateway: an inner group's own Patience waits out its busy members through an outer group (#2410) | 路由；`prepare-ai-routing.mjs / routing.go` | 新版已抽取；规则和等待专项验收待做 |
| `df341e7` | gateway: a member that answers a stream request with a whole reply is asked once, not again and again (#2455) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `e085d6e` | cli, gateway: magpie heap writes the running magpie's Go heap profile, with its memory numbers | 诊断 / 性能；`WinOne 日志与资源采样` | 堆采样待验收，不暴露整套 Magpie CLI |
| `b566745` | gateway: the model list says each model's reasoning levels as plain names too, for Catwalk clients (#2576) | 后台协议 / 供应商性能；`gateway.go / 上游 gateway、provider、plugin` | 适配器基线通过；新协议专项断言待跑 |
| `7660399` | gui: a provider's Claude model chip says the number Claude Desktop's menu shows beside it (#2578) | 自定义供应商；`ai-settings.tsx / AI 模型与预设组件` | 导入与展示待补齐 |

## 续接验证记录（2026-10-17）

证据位于仓库隔离目录 _temp/resume-20261016-b3e1/。large-catalog-native/large-catalog-result.json、balance-mantle-native/、fetch-private-integration-2.log、balance-adapter-green-2.log、ai-host-full-final.log 与 frontend-full-threads-recovery.log 对应上述新增结论。未提供真实账号的项目保留实连条件，不以 loopback 测试替代实际供应商账号。其余待验收继续保留。

## 完整变化文件

来自选定 Git diff 的完整 310 文件，包含 compare API 前 300 文件以外的内容。

- `AGENTS.md`
- `LESSONS.md`
- `Makefile`
- `README.md`
- `README.zh-CN.md`
- `docs/integrating.md`
- `docs/reference.md`
- `docs/subsystems/README.md`
- `docs/subsystems/agent-wiring.md`
- `docs/subsystems/claude-subscription-bridge.md`
- `docs/subsystems/gateway-middleware.md`
- `docs/subsystems/gateway-request-hooks.md`
- `docs/subsystems/gateway-routing.md`
- `docs/subsystems/gateway-sessions.md`
- `docs/subsystems/library.md`
- `docs/subsystems/provider-plugins.md`
- `docs/subsystems/providers-accounts.md`
- `docs/subsystems/sync-backup.md`
- `internal/agent/agent.go`
- `internal/agent/agents.go`
- `internal/agent/applied.go`
- `internal/agent/codex.go`
- `internal/agent/codex_async.go`
- `internal/agent/codex_async_test.go`
- `internal/agent/codex_proxy.go`
- `internal/agent/codex_proxy_test.go`
- `internal/agent/dsh.go`
- `internal/agent/dsh_compat_test.go`
- `internal/agent/gatewaykey.go`
- `internal/agent/gatewaykey_test.go`
- `internal/agent/hermes_test.go`
- `internal/agent/lists_models_test.go`
- `internal/agent/openpencil.go`
- `internal/agent/openpencil_test.go`
- `internal/agent/qwen.go`
- `internal/agent/reasonix.go`
- `internal/agent/route.go`
- `internal/agent/sync.go`
- `internal/agentenv/agentenv.go`
- `internal/catalog/catalog.go`
- `internal/catalog/live.go`
- `internal/cli/groups_rules_cli.go`
- `internal/cli/groups_rules_cli_test.go`
- `internal/cli/heap_cli.go`
- `internal/cli/heap_cli_test.go`
- `internal/cli/help_cli.go`
- `internal/cli/help_cli_test.go`
- `internal/cli/main.go`
- `internal/cli/plugins_cli.go`
- `internal/cli/quota_cli.go`
- `internal/cli/usage_cli.go`
- `internal/cli/usage_cli_test.go`
- `internal/codexcat/codexcat.go`
- `internal/codexcat/codexcat_test.go`
- `internal/gateway/anthropic.go`
- `internal/gateway/chat.go`
- `internal/gateway/chat_call_ids.go`
- `internal/gateway/chat_call_ids_test.go`
- `internal/gateway/chat_empty_assistant_test.go`
- `internal/gateway/chat_tool_pairing_test.go`
- `internal/gateway/chat_tool_shape.go`
- `internal/gateway/chat_tool_shape_test.go`
- `internal/gateway/chattidy.go`
- `internal/gateway/claude_cache.go`
- `internal/gateway/claude_client_cache_test.go`
- `internal/gateway/claude_lean_prompt_test.go`
- `internal/gateway/claude_resume_test.go`
- `internal/gateway/claude_subscription.go`
- `internal/gateway/codex_backend.go`
- `internal/gateway/codex_titles.go`
- `internal/gateway/continuation.go`
- `internal/gateway/desktop.go`
- `internal/gateway/desktop_test.go`
- `internal/gateway/draw.go`
- `internal/gateway/draw_fetch_test.go`
- `internal/gateway/dsh_marks_test.go`
- `internal/gateway/fallback.go`
- `internal/gateway/gateway.go`
- `internal/gateway/gateway_hook_test.go`
- `internal/gateway/gateway_hooks.go`
- `internal/gateway/gateway_test.go`
- `internal/gateway/groupfast.go`
- `internal/gateway/heap.go`
- `internal/gateway/heap_test.go`
- `internal/gateway/holdcatalog.go`
- `internal/gateway/image_fetch_guard.go`
- `internal/gateway/image_fetch_guard_test.go`
- `internal/gateway/image_input_test.go`
- `internal/gateway/ir.go`
- `internal/gateway/key_models_test.go`
- `internal/gateway/mantle.go`
- `internal/gateway/mantle_test.go`
- `internal/gateway/middleware.go`
- `internal/gateway/native_compaction.go`
- `internal/gateway/native_compaction_test.go`
- `internal/gateway/overload_test.go`
- `internal/gateway/patience.go`
- `internal/gateway/patience_test.go`
- `internal/gateway/plan_credits_test.go`
- `internal/gateway/plugin_parity_test.go`
- `internal/gateway/plugin_tiers_test.go`
- `internal/gateway/remote_magpie.go`
- `internal/gateway/request_builds_once_test.go`
- `internal/gateway/request_idle_test.go`
- `internal/gateway/rpm_test.go`
- `internal/gateway/rule_effort_test.go`
- `internal/gateway/rules.go`
- `internal/gateway/search.go`
- `internal/gateway/search_test.go`
- `internal/gateway/stream_error_passing_test.go`
- `internal/gateway/streamless_member_test.go`
- `internal/gateway/think_code_test.go`
- `internal/gateway/tier_accounts_test.go`
- `internal/gateway/trace.go`
- `internal/gateway/ttft_test.go`
- `internal/gateway/usage_api_test.go`
- `internal/gateway/usage_cache_partial_test.go`
- `internal/gateway/usage_cache_rounds_test.go`
- `internal/gateway/usage_cache_split_test.go`
- `internal/gateway/usage_cache_zero_split_test.go`
- `internal/gateway/usage_ingest.go`
- `internal/gateway/usage_ingest_test.go`
- `internal/gateway/video.go`
- `internal/gateway/video_custom_test.go`
- `internal/gateway/video_test.go`
- `internal/gateway/vision.go`
- `internal/gui/agent_models.go`
- `internal/gui/api.go`
- `internal/gui/app.go`
- `internal/gui/assets/app.css`
- `internal/gui/assets/app.js`
- `internal/gui/assets/i18n.js`
- `internal/gui/assets/index.html`
- `internal/gui/assets/plugins.css`
- `internal/gui/assets/plugins.js`
- `internal/gui/assets/plugins/routing/client.js`
- `internal/gui/assets/plugins/routing/i18n.js`
- `internal/gui/assets/plugins/routing/routing.css`
- `internal/gui/assets/plugins/usage/i18n.js`
- `internal/gui/assets/plugins/usage/page.js`
- `internal/gui/core_budget_test.go`
- `internal/gui/desktop.go`
- `internal/gui/groups.go`
- `internal/gui/groups_test.go`
- `internal/gui/icon-256.png`
- `internal/gui/linuxoptions_test.go`
- `internal/gui/panel_space_darwin_test.go`
- `internal/gui/pluginpage.go`
- `internal/gui/pluginpage_window_test.go`
- `internal/gui/plugins.go`
- `internal/gui/plugins_gateway_test.go`
- `internal/gui/providers.go`
- `internal/gui/providers_desktop_test.go`
- `internal/gui/providers_move_test.go`
- `internal/gui/scheme_linux.go`
- `internal/gui/scheme_linux_test.go`
- `internal/gui/scheme_other.go`
- `internal/gui/scheme_windows.go`
- `internal/gui/settings_keep_test.go`
- `internal/gui/space_darwin.go`
- `internal/gui/tests/README.md`
- `internal/gui/tests/account-mask.test.cjs`
- `internal/gui/tests/balance-parts.test.cjs`
- `internal/gui/tests/claude-lean-prompt-setting.test.cjs`
- `internal/gui/tests/codex-async-compact.test.cjs`
- `internal/gui/tests/fast-ultra.test.cjs`
- `internal/gui/tests/fixtures/curl/anthropic.sh`
- `internal/gui/tests/fixtures/curl/bailian-cmd.bat`
- `internal/gui/tests/fixtures/curl/deepseek.sh`
- `internal/gui/tests/fixtures/curl/moonshot.sh`
- `internal/gui/tests/fixtures/curl/openai-chat.sh`
- `internal/gui/tests/fixtures/curl/openai-responses.sh`
- `internal/gui/tests/fixtures/curl/openrouter.sh`
- `internal/gui/tests/fixtures/curl/sap-ai-core.ps1`
- `internal/gui/tests/fixtures/curl/unsloth-studio.ps1`
- `internal/gui/tests/fixtures/curl/zhipu.sh`
- `internal/gui/tests/mobile-web.test.cjs`
- `internal/gui/tests/model-desktop-number.test.cjs`
- `internal/gui/tests/panel-routing-pick.test.cjs`
- `internal/gui/tests/plan-credits.test.cjs`
- `internal/gui/tests/plugin-gateway.test.cjs`
- `internal/gui/tests/plugin-github-wait.test.cjs`
- `internal/gui/tests/plugin-move.test.cjs`
- `internal/gui/tests/plugin-moved-signin.test.cjs`
- `internal/gui/tests/plugin-other-ways.test.cjs`
- `internal/gui/tests/plugin-release.test.cjs`
- `internal/gui/tests/provider-auth-header.test.cjs`
- `internal/gui/tests/provider-curl-paste.test.cjs`
- `internal/gui/tests/routing-groups-off.test.cjs`
- `internal/gui/tests/routing-rule-effort.test.cjs`
- `internal/gui/tests/signin-paste-plugin.test.cjs`
- `internal/gui/tests/usage-ledger-detail.test.cjs`
- `internal/gui/tests/usage-ledger-key-split.test.cjs`
- `internal/gui/tests/usage-ledger-ttft.test.cjs`
- `internal/gui/tests/usage-ledger.test.cjs`
- `internal/gui/tests/usage-speed.test.cjs`
- `internal/gui/window_peer.go`
- `internal/imagemcp/big_image_test.go`
- `internal/imagemcp/imagemcp.go`
- `internal/library/library_test.go`
- `internal/library/mcp.go`
- `internal/library/skill_from.go`
- `internal/library/skill_from_test.go`
- `internal/library/skill_how_test.go`
- `internal/library/skill_version_test.go`
- `internal/library/skillnew.go`
- `internal/library/skillnew_test.go`
- `internal/library/skills.go`
- `internal/middleware/middleware_test.go`
- `internal/plugin/agent.go`
- `internal/plugin/auth_parsed_once_test.go`
- `internal/plugin/gateway.go`
- `internal/plugin/gateway_test.go`
- `internal/plugin/github.go`
- `internal/plugin/github_test.go`
- `internal/plugin/host.go`
- `internal/plugin/host.js`
- `internal/plugin/providers.go`
- `internal/plugin/store.go`
- `internal/plugin/testdata/fake/index.js`
- `internal/proc/path_other.go`
- `internal/proc/path_other_test.go`
- `internal/provider/account.go`
- `internal/provider/balance.go`
- `internal/provider/balance_test.go`
- `internal/provider/bedrock_mantle_test.go`
- `internal/provider/cards_kept.go`
- `internal/provider/cline.go`
- `internal/provider/codex_models.go`
- `internal/provider/devin.go`
- `internal/provider/devin_models_test.go`
- `internal/provider/factory_key.go`
- `internal/provider/factory_key_test.go`
- `internal/provider/fastpick.go`
- `internal/provider/fastpick_test.go`
- `internal/provider/group.go`
- `internal/provider/grouppick_test.go`
- `internal/provider/grouprule.go`
- `internal/provider/grouprule_useeffort_test.go`
- `internal/provider/import_opencode.go`
- `internal/provider/importapps.go`
- `internal/provider/importapps_test.go`
- `internal/provider/key_allowance.go`
- `internal/provider/kilo.go`
- `internal/provider/listed_apis_cost_test.go`
- `internal/provider/migrate.go`
- `internal/provider/migrate_factory.go`
- `internal/provider/migrate_handover_test.go`
- `internal/provider/migrate_kiro.go`
- `internal/provider/migrate_lapse_test.go`
- `internal/provider/migrate_movers_test.go`
- `internal/provider/migrate_side.go`
- `internal/provider/migrate_side_test.go`
- `internal/provider/migrate_test.go`
- `internal/provider/migrate_ux_test.go`
- `internal/provider/migrate_zcode.go`
- `internal/provider/migrate_zed.go`
- `internal/provider/modelprefs.go`
- `internal/provider/models.go`
- `internal/provider/plugin_usage.go`
- `internal/provider/plugin_usage_test.go`
- `internal/provider/plugins.go`
- `internal/provider/pluginsignin.go`
- `internal/provider/pluginsignin_paste.go`
- `internal/provider/pluginsignin_paste_test.go`
- `internal/provider/presets.go`
- `internal/provider/provider.go`
- `internal/provider/quotas.go`
- `internal/provider/remote_magpie.go`
- `internal/provider/ruleparse.go`
- `internal/provider/signin.go`
- `internal/provider/subscription_usage.go`
- `internal/provider/test.go`
- `internal/provider/workbuddy_checkin.go`
- `internal/provider/workbuddy_checkin_test.go`
- `internal/provider/zcode_startmodels_test.go`
- `internal/sessions/traces_clients_test.go`
- `internal/sessions/traces_opencode.go`
- `internal/sessions/traces_opencode_db.go`
- `internal/settings/accounts.go`
- `internal/settings/accounts_parsed_once_test.go`
- `internal/settings/settings.go`
- `internal/source/source_test.go`
- `internal/tui/pages.go`
- `internal/tui/routing.go`
- `internal/usage/caller_provider_test.go`
- `internal/usage/ledger.go`
- `internal/usage/ledger_test.go`
- `internal/usage/plan_credits_test.go`
- `internal/usage/reasoning_stream_speed_test.go`
- `internal/usage/request_page.go`
- `internal/usage/request_snapshot.go`
- `internal/usage/usage.go`
- `internal/usageapi/usageapi.go`
- `site/install.test.mjs`
- `site/public/docs/integrate.html`
- `site/public/docs/intent.html`
- `site/public/docs/ja/intent.html`
- `site/public/docs/ja/plugins.html`
- `site/public/docs/ja/start.html`
- `site/public/docs/plugins.html`
- `site/public/docs/start.html`
- `site/public/docs/zh/integrate.html`
- `site/public/docs/zh/intent.html`
- `site/public/docs/zh/plugins.html`
- `site/public/docs/zh/start.html`
- `site/public/img/sponsor-zerotoken-dark.svg`
- `site/public/img/sponsor-zerotoken.svg`
- `site/public/install.sh`
- `site/sponsors.js`
