# AI 基座更新与功能对照（2026-11-25）

本轮按开始时核对的两个上游主分支锁定源码。WinOne 的常用、自定义供应商、账号供应商和本地网关继续共用这些实现；构建不自动推进上游。

| 组件 | 旧提交 | 本轮提交 | 来源与许可 |
| --- | --- | --- | --- |
| Magpie | `f65fe92ef93557e9a2098d2606a4d9c0175a4213` | `e29699527866704ae0a9c6fbee4e2a3acf00b733` | [主分支差异](https://github.com/yetone/magpie/compare/f65fe92ef93557e9a2098d2606a4d9c0175a4213...e29699527866704ae0a9c6fbee4e2a3acf00b733)，243 个提交，MIT |
| 账号适配器 | `ae9dba9f7ff197c0f3a7526af998d2a2d5d806a3` | `bbd3a2e76d7f2e765f0181786a60169b7acf860a` | [主分支差异](https://github.com/magpie-community/plugins/compare/ae9dba9f7ff197c0f3a7526af998d2a2d5d806a3...bbd3a2e76d7f2e765f0181786a60169b7acf860a)，51 个提交，MIT |

- `LICENSE` 与内置 jsonschema、yaml.v3 许可字节不变。`go.mod` 新增 goja、regexp2/v2、go-sourcemap、pprof，并把 x/text 升到 v0.43.0；五项许可原文已入 `third_party/licenses/`，`third_party/ocr-ai-notices.json` 共 117 条记录，与 `WinOne.AI.exe` 的依赖逐项对上。
- 随包账号适配器增至 46 个：新增 Magnific API 0.1.1，更新 agy CLI 0.1.121、Trae 0.2.54、Qoder 0.2.73、Factory 0.1.57、Cursor 0.2.49、ZCode 0.1.69、CatPaw 0.2.3、AutoClaw 0.2.25；已有 Antigravity 连接继续使用锁定的旧适配器。
- 上游代理检查文件新增 Claude 出口复查的 GUI 处理函数。生成器仍只截取纯检查部分，并只去掉该处理函数独用的 `egress`、`provider` import；如果截取段开始用到它们，生成器会报错。

## 接入功能对照

| 上游变化 | WinOne 入口与实现 | 验证 |
| --- | --- | --- |
| 中转倍率读取间隔 `RateReadHours`（1–168 小时，默认 6） | 自定义供应商「计费」填写充值折扣后显示「倍率读取间隔（小时）」。Host 用上游 `RateReadHoursOK` 校验，适配层按完整偏好传入上游 Provider，由上游 `KeepKeyRatesRead` 使用 | Host、适配器、原生设置回归 |
| 密钥结算系数 `KeyRateFactors` | 设置充值折扣后，每个密钥的选项显示「结算系数」。Host 按槽位保存并用上游 `KeyRateFactorOK` 校验；模型、余额、本地网关和辅助桥按密钥指纹换算，与价格倍率同一路径；删除账号时一并清除 | 同上 |
| Bedrock 用 AWS 访问密钥 SigV4 签名，临时密钥需会话令牌 | 「服务」中的供应商专用认证新增「云服务会话令牌」，与 Access Key ID/Secret 同样密封保存、配置中只回 `has` 标记，仅私有执行快照解封，并进入密钥状态脱敏列表 | Host 凭据回归；真实 Bedrock 调用未联网验收 |
| 供应商编辑器勾选可使用它的网关密钥 | 「服务」新增「可以使用此供应商的网关密钥」。本地管理接口原样接入上游 `GET/POST /api/provider-keys`（`access.ProviderKeys`/`SetProviderKeys`），勾选写回各密钥自己的模型列表，与网关密钥页一致；只能使用本供应商的密钥按上游规则不可在此取消 | 适配器编译与测试、原生页面回归；真实勾选写入未在隔离运行时手动操作 |
| 网关、供应商、路由、用量与账号的其余修复（Copilot 推理令牌、Claude 限额休息、agy 配额、GLM/豆包/火山模型目录、嵌入模型定价、按思考档位折叠的中转模型等） | 本地网关与供应商直接执行锁定的上游代码，无需 WinOne 入口 | 上游网关/供应商/插件测试、适配器测试 |
| 账号适配器更新 | 按随包 registry 当前条目执行，登录、请求、用量和会话语义随锁更新 | 386 个运行文件按锁校验；未登录真实账号 |

上游 Library、Artifacts、Memory、Agents、Sessions、MCP、路由组页面及 Settings › Usage 的新开关属于其他产品页面，不在 WinOne 四个 AI 服务入口范围内。

## 交付记录

离线检查与构建日志在 `_temp/ai-base-20261125/`。安装器、匹配卸载器、校验和来源提交以 `dist/release/release-info.json` 的实际记录为准；用户自行运行固定安装器。
