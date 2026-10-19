# AI 基座功能对照

2026-10-15 选择 Magpie `8b89b2e36c4ae5b98e6672e86ec0ab1cdd7af77f`，对照前版 `bc470c6b63ba6449bd7071016313f3e7451a8d41`。构建读取本次选择的来源 HEAD，来源和依赖分别记录在 `third_party/sources.json`、`modules/ai-magpie/upstream.go.mod` 与 `upstream.go.sum`。

AI 服务按功能拆分接入。路由只承载路由插件。完整 Magpie 应用的全局导航、供应商页、会话页、用量页及启动脚本不进入路由文档。

| Magpie 变更 | WinOne 所属功能与接入位置 | 本轮验证 |
| --- | --- | --- |
| `provider` 的配置读取保护、账号切换、额度与余额历史保护、插件更新状态保护 | 自定义供应商、账号供应商；`modules/ai-magpie/accounts.go`、`settings.go`、账号插件快照及 Host 账号接口，继续调用新版真实实现 | AI 适配器、OAuth、账号快照测试通过 |
| `routes`、`rotation`、`groups` 拆分，顺序、轮询、用量、周配速、权重、分组执行与失败保护 | 路由；`routing.go`、`routing_gui.go.txt`、`build/prepare-ai-routing.mjs`；完整原版 Routing 插件、模板、条件、嵌套、手动选择、冷却与实时轨迹 | 8 个中英/明暗/宽窄布局场景；模型选择、添加保存、命名设置、打码、取消丢弃；真实分组请求、密钥故障切换、冷却、访问控制、重启持久化均通过 |
| 隐私净化、请求中间件和轮换移入 builtin plugin runtime | 高级设置与所有实际 AI 请求；`settings.go` 注册上游 builtin 插件并刷新运行时，继续使用上游失败关闭的净化检查 | 请求净化、回复恢复、模型 wire name、隐私规则和搜索配置测试通过 |
| Gemini、Code Assist、Codex、嵌入、图片/视频/语音、搜索及协议中继修复 | 默认 AI 服务、本地网关及对应任务调用；`gateway.go`、`stream.go`、`settings.go` 接入同一 Magpie 后端 | AI 适配器测试通过；最终整包流程仍按发布检查执行 |
| 统计共享文件、额度历史和损坏状态保护 | 用量统计；`usage.go`、`usage_embed.go.txt` 使用新版记录与计算；路由账户行仅调用所需的额度只读接口 | 适配器及路由测试通过；用量界面保留独立入口 |
| 外部应用会话的子代理树、链接识别和会话删除保护 | Magpie 的外部应用会话管理；WinOne 的会话统计只读取已登记的 Pi 历史，来源边界见 `configureUsage` | 不启动外部应用发现，不把该会话页面加入路由；WinOne Chat 的统计继续独立验收 |

## 路由接入边界

`prepare-ai-routing.mjs` 从所选 Magpie 版本读取实际路由插件、上下文卡片，以及被调用的公共控件。原版模板、选择器、菜单、确认框、排序、打码、滚动保护和样式均保留。构建生成独立 HTML，并记录提取的来源与校验值；如路由重新依赖整应用页面，构建报错要求重新核对适配。

宿主适配限于：私有认证地址、WinOne 供应商快照、路由需要的数据刷新、主题与语言、页面高度、命名偏好的持久化，以及退出保护。命名偏好保存后更新原子快照，正在运行的请求不会读取半次更新。

## 验证入口

- `build/build-ai.ps1 -Test`：实际适配器、OAuth 和账号快照。
- `build/test-ai-routing.mjs`：只注册路由插件，原版卡片、选择器和确认框的真实浏览器操作。
- `internal/ai/routing_windows_test.go`：使用显式指定的隔离 AI 产物，验证请求执行、保存、重启和权限边界。

上述检查不代表已完成真实账号联网或最终安装器验收。安装器只在整包验证后更新。
