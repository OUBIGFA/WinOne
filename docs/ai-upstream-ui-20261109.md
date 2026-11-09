# 固定 AI 版本与上游界面接入（2026-11-09）

本次固定已有源码版本，不再追逐上游新提交。内置页面使用 MyGo，第三方插件自带网页保留原页。WinOne 主侧栏保留，AI 页面内部的子导航按用户要求移到顶部。

## 固定输入

| 输入 | 固定版本 | 记录位置 |
| --- | --- | --- |
| Magpie | `c9d253e4a2e7add1e31578244eda52bfdce11258` | `third_party/sources.json` |
| 社区插件 | `d54df279c0ce9ca35a79e72cb2d81ba368b37358` | `third_party/ai-plugins.json` |
| MyGo | `v0.8.3` | 两个 UI 模块的 `go.mod`、`third_party/mygo-winone/source.json` |

构建入口检查来源提交、许可证、原生文案和网关连接示例，拒绝输入与清单不符。`build/use-local-resources.ps1` 选择对应的仓库缓存；不会更新用户真实安装。保留旧版本 OCR 来源的许可证，因为现有来源记录仍引用它。

## 功能对照

下列入口均已接入；“通过”只指后文列出的测试范围，不表示逐个使用真实账号或修改真实客户端。

| 上游功能 | WinOne 对应界面及行为 | 主要实现 |
| --- | --- | --- |
| 内置插件 | 内置、发现、已安装三个标签；内置启停、选项和功能入口 | `plugins_native.go`、`plugins_bundled.go` |
| 发现插件 | 分类、搜索、GitHub、手动安装、来源设置、详情、直接安装和错误重试 | `plugins_upstream.go`、`plugins_details.go` |
| 已安装插件 | 名称排序、版本、更新、启停、移除、设置、权限和提供工具给客户端 | `plugins_upstream.go` |
| 路由 | 路由组、成员、规则、统计、请求追踪及上游编辑分区 | `routing*.go`、`observability_upstream.go` |
| 隐私、搜索、媒体、用量 | 原有选项、状态、记录、汇总和操作入口 | `advanced*.go`、`privacy.go`、`masking*.go`、`usagetab.go` |
| 网关 | 状态与设置、连接示例、访问密钥、并发限制及调用记录 | `gateway*.go`、`gateway_examples.json` |
| 资料库：指令 | 指令集、新建/使用/重命名、附加、导入、共享与同步 | `library_content.go` |
| 资料库：MCP | stdio/HTTP 表单、JSON、批量粘贴、发现、客户端分配、同步、路由器 | `library_content.go`、`library_server.go` |
| 资料库：技能 | 文件编辑、项目、发现/导入、批量操作与客户端分配 | `library_content.go` |
| 资料库：Hooks、子代理、RTK | 创建、编辑、分配、移除；RTK 检查、安装/升级、PATH 和客户端开关 | `library_content.go` |
| 外部客户端会话 | 列表/搜索、内容、导入、导出、转换、恢复/清理及操作回执 | `sessions.go`、`sessions_manage.go` |
| 第三方网页 | 原页面、SDK 操作、权限读取、扩展面板与关闭回收 | 既有隔离页面桥及 `native_guest_windows_test.go` |

上述相对文件名位于 `apps/desktop-mygo/pages/aiservices/`。适配来源、文件摘要及许可记入 `third_party/import-notices.json`；本次最终检查覆盖 16 个引入项目、381 个适配文件。

## 外部客户端边界

- 打开资料库或会话页不自动扫描用户客户端。读取、导入、写入和同步由用户点击触发；运行验收全部使用隔离目录和脱敏夹具。
- 外部操作使用独立 worker 和私有资料库。与普通 AI worker 共用已安装的第三方插件目录及工具清单，避免“提供给客户端”找不到工具。
- 插件工具开关显示真实错误和资料库问题；成功后重新读取插件状态，不在前端假翻转。
- MCP 地址使用实际启动的独立监听端口。已启用的服务按持久状态恢复；查看页面使用的临时连接在离页且无任务、无草稿时释放。
- 禁止工具运行时隐式同步客户端。可变更客户端的操作仍走显式操作入口。

## 同步修复

- 获取模型时解码到新映射，清除已移除模型的旧元数据，修复随后保存时报“未知模型”。列表按模型 ID 的固定规则排序，获取、保存和重新打开保持一致。
- 共享模型菜单增加供应商组间距与标题下细线；常用页恢复上下布局，模型选择区左右留白相等。
- MCP 表单与 JSON 往返保留空参数、前后空白、参数内换行及多行环境变量/请求头。只有用户实际编辑的字段才从表单重新解析。
- 批量 JSON 在发送任何请求前检查规范化后的名称冲突，失败保留完整输入；部分导入失败只保留未处理条目供重试。
- 自定义 HTTP/npm MCP 发现结果进入普通服务器表单；RTK 的 blocked 状态只禁止启用，已启用仍可关闭，休眠客户端禁止操作。
- 请求追踪首次失败后保留错误，等待用户重试，避免逐帧重新请求。
- 图标控件持有当前图片引用，避免可见图标超过共享缓存容量后互相逐出。真实窗口曾出现 9,821 次图标读取；修复后的 68 图标回归只读取 68 次。

## 已验证范围

证据原始目录为 `_temp/ai-upstream-20261109-resume-final/`，精选日志与截图交付到 `dist/reports/ai-upstream-20261109/`。前序模型与结果窗证据来自 `_temp/ai-upstream-20261109/`。

| 检查 | 结果与证据 |
| --- | --- |
| 源码测试、静态检查、Host 编译 | 通过；`source2.log` |
| 桌面原生完整测试 | 通过；`native-complete.log` |
| 结果窗测试 | 通过；前序 `result-suite.log`；共享菜单修复后另跑相关回归 |
| 资料库真实 UI 与 HTTP 回归 | 通过；`library-delivery.log`；原始字面值 16 个往返组合、实际编辑、名称冲突、自定义发现和 RTK 回归有先失败后通过证据 |
| 后端真实 worker 与 Host | 通过；`backend/external-final.log`；含插件工具列举/调用/关闭、四种 stdio MCP、OTel、服务生命周期、恢复、浏览器断连 |
| 第三方原页面 | 私有桌面的真实窗口 SDK 操作与权限读取通过；`native-windows-delivery/tests.log` |
| 原生真实窗口 | 32 场景 × 2 语言 × 2 主题 × 2 宽度，生成 256 张截图；内置页未创建 WebView；同上日志 |
| 界面人工复核 | 查看常用页、模型菜单、插件、路由、隐私、网关、会话及资料库截图；重点核对窄窗换行、控件留白、表单/JSON 与明暗主题 |
| 统一完整候选编译 | 通过；`build-complete.log`；该命令使用 CompileOnly，运行测试由上述独立检查提供 |
| 安装启动握手 | 候选包在私有桌面执行真实 `--check-ui` 通过；维护标记保留、用户输入桌面未切换；`installer-ui-probe.log` |
| 最终包内 AI 程序 | `shared/ai/WinOne.AI.exe` 的后端 FullAdapter 与原生实窗重验通过；`packaged-external.log`、`packaged-native.log` |
| 安装包与产物关联 | 7,571 个文件回读通过；安装器、配套卸载器的 SHA-256 与包清单关联通过；`package.log`、`installer-artifacts.json` |

未执行真实账号付费请求、真实外部客户端配置修改、实际安装/覆盖/卸载及提权测试。私有桌面的安装启动握手不等同于完整安装生命周期验收；历史生命周期结果不冒充本轮结果。

## 交付

固定安装器为 `dist/release/WinOne-setup.exe`，配套卸载器为 `WinOne-uninstall.exe`，两者分别附 SHA-256。最终提交、包清单摘要、产物关联和本轮验证范围由同目录 `release-info.json` 记录。旧五个交付文件保存在本轮临时目录的 `prior-release/`。

本轮安装器 SHA-256：`a3c4206041c0d225b7be19738790d1aa1989f54efb2a626bcb3fe0aac88b3e27`。配套卸载器 SHA-256：`1af12ed3e4475ac5232c6e9281fc2d7a1527465b4da08404fe1fdd02e5f45b8e`。

用户已有的 `Microsoft/` 和 `menu.txt` 保留，不属于本次提交。用户自行运行安装器；本次不更改真实 WinOne 安装或配置，不创建 tag 或对外发布。
