# AI 服务多密钥实施计划

> 按项目约定由当前 Agent 在本会话执行，不使用子代理，不创建提交或对外发布。

**目标：** 同一自定义供应商提供多个独立密钥输入、启停/删除、轮询/顺序策略及失败切换选项。

**架构：** 公共配置保存密钥 ID、启用状态与策略；现有 DPAPI 存储保存逐项密文，并继续保留单密钥投影。Host 的共享 Store 管理跨请求调度和冷却，magpie 保留协议适配。请求固定供应商和模型，收到任何流式增量后不自动重试。

**技术：** Go Store/Host、magpie 进程协议、现有 React/Base UI/Lucide 设置页。

**设计：** [AI服务多密钥](../AI服务多密钥.md)。

## 约束

- 每个供应商最多 16 个密钥；每项仍遵守现有 16384 字节密钥上限与 1MiB 配置总量限制。
- 策略为 `round-robin` 或 `ordered`，默认轮询；自动换钥默认开启、最多 3 次不同密钥尝试、失败冷却 60 秒。
- 所有尝试受同一请求截止时间控制；401/403/408/429/5xx 且未输出时才允许换钥，400、取消和已输出的流不重试。
- Retry-After 使用受控数值传递，不传递供应商原始响应或密钥；冷却不依赖常驻定时器。
- 旧调用入口和配置可读；新快照不返回密钥明文，未知密钥 ID 和跨供应商写入必须拒绝。

## 1. 存储与调度

文件：`internal/ai/configuration.go`、`configuration_windows.go`、`store_windows.go`；新增 `credentials_windows.go` 和对应测试。

接口：`KeySlot{ID, Enabled, HasKey}`、`KeyPolicy{Strategy, Failover, MaxAttempts, CooldownSeconds}`；`CredentialUpdates map[string]map[string]*string`；`Store.SaveConfigurationWithKeys(c, legacy, edits)`。原 `SaveConfiguration(c, legacy)` 委托给新方法。增加 `ForTaskKeys(task, override)` 与 `ProviderKeysForModels(id)` 返回只在服务内部使用的 `KeyPlan`。`Store.WithKeys(ctx, plan, run func(context.Context,string)(bool,error)) error` 的回调布尔值表示本次已产生流式增量。

- [x] 写入并运行失败测试：旧 key 投影成首项；新增两项后重读得到三个密钥；删除/停用不再被选择；重命名或改模型不丢密钥。
- [x] 实现逐项 DPAPI、ID/数量/策略校验、原子保存与 revision 检查，拒绝无效或跨供应商编辑。
- [x] 用真实 Store 测试轮询：回调依次记录 `first, second, third, first`；并发请求合计各取相同数量。
- [x] 用可注入时钟测试故障：首项返回受控 HTTP 429，次项成功；冷却中跳过首项，时间越过冷却后可再次使用；更新密钥不继承旧密文的冷却。
- [x] 测试已输出/400/取消不换钥，限制总尝试次数，停用全部密钥时明确失败，本机无密钥服务仍可调用。

验证：`go test ./internal/ai -count=1`；测试文件和日志不得包含真实凭据。

## 2. 错误协议与调用接入

文件：`modules/ai-magpie/main.go`、`stream.go`、对应测试；`internal/ai/stream_windows.go`、`models_windows.go`；`apps/host/ai_windows.go`、相关调用回归。

- [x] 为 Worker 失败事件增加受控 `status` 与 `retryAfterSeconds`，HTTP 状态可用时保留，其他错误维持受控分类。
- [x] 在流读取边界恢复 `AttemptFailure`，确保部分文本和思考已发出后不能被另一个密钥接续。
- [x] Host 生成/识别/翻译调用通过 `WithKeys`；模型获取使用同一密钥集合和策略；账号模式保持原有串行登录隔离。
- [x] 回环服务按 Authorization 记录真实请求，验证三个协议的轮询和失败切换；断流后第二个密钥请求数应为零。

验证：统一 `build/build-ai.ps1 -Test`；相关 Host/AI 回归及实际 Worker 模拟服务，检查进程退出与取消。

## 3. 设置界面

文件：`ai-settings.tsx`、`ai-configuration.ts`、`locales/ai.ts`；新增共享设置组件 `ai-provider-keys.tsx` 及交互测试。

- [x] 保留原有单密钥读取，归一化到稳定首项；未修改旧密钥时不提交明文更新。
- [x] “+”按钮添加行；每行密码输入、启停与删除图标。已保存密钥使用“留空保留”提示，删除明确修改该行。
- [x] 多密钥显示策略、失败切换、次数和冷却选项；保存失败保留输入，切换供应商不混淆各自草稿。
- [x] 验证新增/删除/停用、配置保存请求、失败保留、模型获取保存条件及中英文无障碍标签。

验证：前端针对性 Vitest、类型检查及中英/深浅色/窄宽窗口检查。全部使用现有组件和图标。

## 4. 完整验收与更新

- [x] 用模拟的三个密钥完成截图翻译、OCR/对话和模型发现，核对真实授权头、候选顺序、流式边界与取消。
- [x] 统一构建、依赖/文件校验，检查旧配置迁移和备份可恢复性。
- [x] 在维护锁内备份并更新试用版及已安装版，核对配置和管理员任务，复验实际安装位置并重新打开程序。
- [x] 在 `docs/development.md` 写入实际通过范围、失败记录和仍未完成的正式版门槛。
