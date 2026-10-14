# AI 账号登录完整性

2026-10-08 核查。账号登录与调用继续共用 magpie，WinOne 仅适配登录状态、交互和本地生命周期。

## 上游版本

- magpie：[`c2020e0bb3060b9b69a2af65d427c6a6cb3e1a90`](https://github.com/yetone/magpie/commit/c2020e0bb3060b9b69a2af65d427c6a6cb3e1a90)，2026-10-08 再次查询时主分支最新提交。
- 社区插件：[`96c5b0f9920d006bde3bcdc298fd63d5353384ce`](https://github.com/magpie-community/plugins/commit/96c5b0f9920d006bde3bcdc298fd63d5353384ce)。15 个插件包，19 个插件账号类型，与内置目录合并为 25 个账号选项；中间件不作为账号列入。
- Cline 0.1.5、WorkBuddy 0.1.10、ZCode 0.1.12。Bun 保持 1.4.2；全部资源按 `third_party/ai-plugins.json` 校验。

相对上一版 magpie，AI 相关更新包括 Gemini 3 最低思考级别及 Google 拒绝 minimal 时的处理、WorkBuddy 额度耗尽错误、ZCode 用量窗口与额度。另有插件发现、便携目录识别和 Hermes 连接修复。WinOne 不启用上游 GUI、网关共享、自动安装客户端或其他客户端配置改写；上游这些入口的更新不等于 WinOne 新增对应功能。

本次复核在 `ed4476b` 后发现 15 个新提交，继续更新到上述固定版本。新增适用修复包括 OpenCode Go / Zen 按实际模型补全 Responses、Anthropic 与 Chat 协议入口，DeepSeek Responses 请求回传思考内容，以及 SiliconFlow 已停用余额接口的明确说明。上游登录实现和账号方法未变；社区插件的新提交只增加配额查询技能，没有新增账号插件，也未改变已锁定插件文件。WinOne 不将该技能作为账号或产品入口安装。

## 登录方式核查

| 账号或方式 | 用户操作 | WinOne 呈现 |
| --- | --- | --- |
| Cline、Factory、MiniMax 国内/国际、内置 Copilot | 在供应商网页输入或核对设备码 | 独立只读设备码、复制按钮、登录链接；自动等待完成 |
| Claude Code 与返回 `method: code` 的插件协议 | 从网页取得授权码 | 授权码输入框，Enter 或按钮提交 |
| Codex、ChatGPT API、Gemini CLI、Antigravity，以及支持回填的插件 | 在浏览器授权，必要时回填完整回调地址 | 自动等待、复制链接、打开页面、回调输入 |
| Command Code 内置流程 | 浏览器授权或粘贴授权密钥 | 保留授权密钥页面与密码输入；插件另有 API 密钥方式 |
| Cline、Cursor、Factory、Kiro、Command Code | 填写供应商 API 密钥 | 按选中方式直接显示密码框，无需先保存供应商 |
| ZCode | 选择 Z.ai / BigModel 浏览器登录；API 密钥方式先选地区 | 显示前置选择问题、选项说明，然后输入密钥 |
| OpenCode Zen Free | 按插件说明填写 `public` | 保留上游明确要求的 API 方法与说明，不伪造无密钥登录 |
| Qoder 国内/国际、WorkBuddy 国内/国际、Trae 国内/国际、MiMo、Zed、Cursor、Devin、Kiro | 网页授权或扫码，按供应商返回步骤完成 | 展示登录链接、上游说明与实际状态；没有设备码时不显示空设备码框 |
| Grok Build | 通过官方客户端完成设备登录 | 显示上游说明；缺少客户端时明确提示安装要求，不隐式安装 |
| 插件提供的 CLI / IDE 现有登录方式 | 用户明确选择该方式 | 保留方式和说明；运行于 WinOne 私有配置环境，不自动读取或切换其他客户端账号 |

设备授权方向依据 [RFC 8628 §3.3](https://www.rfc-editor.org/rfc/rfc8628#section-3.3)：设备码供用户在验证网页输入，轮询凭据 `device_code` 不应作为用户设备码展示。Cline 返回 `the code is …`，magpie 原正则只识别 `code …`；WinOne 在状态边界归一化，不按供应商名称写特殊分支。仅在非回填的等待状态补设备码，优先保留供应商明确字段；URL 后备只读取 HTTPS 的 `user_code`。

## 交互与生命周期

登录准备、前置问题、等待授权、保存授权、取消和失败分别提示。输入被拒绝时保留原值；提交后等待校验或下一步，禁止重复提交。设备码和登录链接支持复制及手动选中。浏览器自动打开失败仍可复制链接或重试打开。

登录归属于连接与设置面板；关闭或切换连接取消任务，包括面板关闭后才返回的 start 请求。父组件回调身份变化不取消任务。终止状态移除设备码及密钥输入。账号落盘、子进程退出和操作锁释放完成后才显示最终成功。

## 验证边界

回归覆盖设备码句式和输入方向、Host 提交校验、界面复制/回填/取消/切换连接，以及打包插件方法、模型协议、流式调用和账号隔离。完整包的构建、安装与实际窗口证据写入 `docs/development.md` 对应记录。

模拟授权服务与空白私有配置验证协议和本地生命周期；不代表真实供应商账号已认证成功。用户账号、外部授权页面、套餐及服务端可用性仍需实际登录验证。
