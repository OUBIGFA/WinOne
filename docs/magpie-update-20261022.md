# Magpie 2026-10-22 更新与插件登录生命周期

锁定 Magpie `7dd965ed22e6619c9084f3bca58e444073e8a41c`（原 `efd1b697`），社区插件 `43ffb4601807497a9a90dfef728a7d7533899cda`（原 `d87dd8f`）。上游查询和差异已核对；go.mod、go.sum、两项目 LICENSE 无变化。浏览器插件复现用 `mu-zi-lee/magpie-chatgptweb2api@6e7b6829af6e636abc6bd26d649ca88cbbf9c1dc`，仅本地测试，不将实验插件作为内置插件分发。

## 修复与验收
- 旧测试固定 executablePath/headless，绕过默认浏览器准备。默认冷启动复现 60 秒授权超时且具体错误丢失。
- 原生完整按钮链配置 Edge 后可以打开专用窗口、完成本地模拟登录；原生窗口可见性检查通过，但 Worker 结束后专用浏览器被 Job Object 终止，违反 keep-open。
- 修复方向：授权准备纳入登录所属 context，保留 Magpie 完整异步状态/取消/保存；第三方插件正常结束后的子进程属于 Host 生命周期，而非单次 RPC，仍由 kill-on-close Job Object 保证 Host 退出回收。
- 验证状态：进行中，不能将上述旧实现诊断当作修复完成。

## 上游变化 → AI 服务功能 → 接入位置 → 验证
| 上游变化 | 对应功能/接入位置 | 验证状态 |
| --- | --- | --- |
| 按敏感信息类型脱敏/警告/阻止、白名单、试运行与记录动作统计 | 高级隐私；MagpieSettings、隐私界面、masking.Preview、maskrec 与真实网关 | 待验证 |
| Responses additional_tools 可配置、延迟工具保持前缀缓存 | 自定义供应商高级配置、provider 与 gateway | 待验证 |
| 插件模型声明 searches 原生搜索、Grok 搜索及 tool_choice none | 模型/搜索设置、catalog/provider/gateway 原执行链 | 待验证 |
| 请求解析缓存/大小边界、无改写请求复用、最近环形记录 | 默认 AI/本地网关原 gateway | 待验证 |
| Chat 收尾、文本工具分组与闭合、tool_choice none、Anthropic 响应、远程模型选择 | 默认 AI/本地网关完整协议转换 | 待验证 |
| 隐私文件名/模型ID白名单、防误匹配、阻止请求 | 高级隐私真实 redact/gateway | 待验证 |
| Claude 退出覆盖全部目录、账号CLI身份、Codex请求、Cursor/Devin/Grok/ZCode账号修正 | 账号供应商完整 provider，在 WinOne 私有 profile 内运行 | 待验证 |
| 用量账本分页快照/索引/阶梯价格、额度显示与历史分离加载 | 用量统计 usage 与 WinOne 额度组件 | 待验证 |
| 路由 client 小幅更新 | 路由独立抽取，不导入整个应用 | 待验证 |
| 社区 CodeArts/Devin/Grok/Kimi Work/MiniMax/Qoder/WorkBuddy 更新；新增 pig agent | 完整 registry/全部资源；插件管理和对应账号，不精简新增包 | 待验证 |
| Zoo Code、ZCode路径、Claude压缩、外部会话缓存/标题、CLI/TUI及原生托盘轻量策略 | 保留上游源码；不接管外部客户端，WinOne Chat/宿主已有生命周期边界不变 | 边界保留 |

不读取个人浏览器或 Cookie，不修改真实安装/配置，不对外推送或发布。安装器须完整验证后才替换。
