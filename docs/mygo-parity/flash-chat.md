# 闪聊功能对照

更新：2026-11-01。React 来源：`apps/desktop/frontend/src/interconnect-chat-entry.tsx`（页面）、`features/interconnect-chat-frame.tsx`（Host 调用、拖放、离开确认）、`features/interconnect-chat-preferences.ts`（昵称）及 `vendor/chitchatter/core`（房间协议）。MyGo 实现：`apps/desktop-mygo/pages/flashchat`（页面）、`apps/desktop-mygo/flashchat`（Chitchatter 核心的 Go 移植）、`apps/desktop-mygo/flashchat/trystero`（Trystero 0.26.0 线协议的 Go 实现，WebRTC 用 Pion）。

用户 2026-10-31 决定：闪聊原生重写，并与现有 Chitchatter/Trystero 房间互通。来源与许可见 `THIRD-PARTY-NOTICES.md` 闪聊段落、`third_party/import-notices.json`（`chitchatter-flashchat-go`、`trystero-flashchat-go`、`fun-animal-names-flashchat`）和 `third_party/desktop-mygo-notices.json`（Pion、gorilla/websocket 等 Go 模块）。

状态：**已验证** = 有单元测试、互通测试或 `ui.Tester` 渲染；**待验证** = 已实现、还缺证据。

## 验证方法

| 检查 | 命令 | 结果（2026-11-01） |
| --- | --- | --- |
| 单元测试 | `go test ./flashchat/... ./pages/flashchat/...`（在 `apps/desktop-mygo`） | 全部通过 |
| 与 React 页面互通 | `node tests/flash-chat-interop/run.mjs`：无头 Edge 运行已构建的 `interconnect-chat.html`，Go 对端运行同一协议，本机 WebTorrent tracker 代替公共 tracker，两个方向各建一次房间 | 21/21 通过 |
| 渲染 | `WINONE_SHOTS=<目录> go test ./pages/flashchat -run TestShots`：中英 × 深浅 × 1000/520 宽，大厅、网络设置、独自在房间、群聊、成员列表（窄窗）、私聊、房间详情 | 52 张，本地 `_temp/mygo-flashchat/shots/`，不入库 |
| Go 模块许可 | `build/verify-desktop-mygo-notices.ps1 -Executable <WinOne.exe>` | 37 项记录通过 |
| 移植源码来源 | `node build/verify-import-notices.mjs` | 12 项导入、323 个文件通过 |

## 有意改变的地方

| 改变 | 原因 |
| --- | --- |
| 房间协议由 Go 实现，不再在 WebView 中运行 Trystero | 原生窗口没有浏览器 WebRTC。线协议、加密、动作名与分片大小保持上游值，互通测试覆盖。 |
| “离开房间”在房间详情对话框中放在底部操作区 | 与其他对话框的按钮位置一致；行为不变。 |
| 拖放文件由 MyGo 的放置区直接接收 | React 由外层框架接收后转发给 iframe；Host 调用（`interconnect.transfer_files`）不变。 |
| 文件共享页面在对话框中用 MyGo WebView 显示，下载保存到“下载”文件夹 | 与 React iframe 的 `allow-downloads` 行为一致。 |

## 大厅

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| L1 | 标题、说明、房间名称（≤80 字）、昵称（≤80 字，失焦保存，读取前禁用） | `lobby`、`nicknameField`、`textField` | `TestNicknameMustLoadFirst`；渲染图 lobby | 已验证 |
| L2 | 昵称读取失败时提示并可重试 | `loadNickname`、`lobby` 中的重试按钮 | `TestNicknameMustLoadFirst` | 已验证 |
| L3 | 创建房间（随机房间号与密码）；加入房间（房间号、密码必填，回车提交，可返回） | `enter`、`joinOptions` | `TestLobbyValidatesBeforeJoining`、`TestTwoPagesTalkShareAndLeave` | 已验证 |
| L4 | 网络设置：自定义 tracker（仅 `wss://` 或本机 `ws://127.0.0.1`）、WebRTC 配置（JSON 对象） | `dialogs`、`joinOptions`、`trystero.ValidRelayURL`、`trystero.ParseRTCConfig` | `TestLobbyValidatesBeforeJoining`；渲染图 network | 已验证 |
| L5 | 只保存个人昵称，房间名称和密码不进入默认值 | `saveNickname` | 代码审查 | 已验证 |

## 房间

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| R1 | 侧栏：群聊入口（人数）、成员列表（随机动物昵称、校验状态）、自己与离开按钮；窄窗（<600）成员列表可开关 | `sidebar`、`workspace` | 渲染图 group、members | 已验证 |
| R2 | 群聊与私聊；私聊房间在成员加入时就开始收消息 | `selectPeer`、`flashchat.Session` | 互通测试 private message 两个方向 | 已验证 |
| R3 | 消息：作者、时间、Markdown（代码块换行、复制），自己的消息靠右 | `transcript`、`codeBlock` | 渲染图 group | 已验证 |
| R4 | 新消息只在读者位于底部时跟随；切换会话从最新消息开始 | `ui.ListState{FollowEnd}`，每个会话一个列表状态 | 代码审查 | 待验证（实窗滚动） |
| R5 | 发送：Enter 发送、Shift+Enter 换行、输入法组字时 Enter 不发送；草稿按会话保存；未送达时保留草稿并提示；私聊对方离开后不能发送 | `composer`、`send`、`canSend` | `TestEnterSendsAndShiftEnterBreaksTheLine`、`TestTwoPagesTalkShareAndLeave` | 部分验证（物理输入法待测） |
| R6 | “正在输入”提示；独自在房间时提示 | `composer` | 互通测试 typing 两个方向；渲染图 | 已验证 |
| R7 | 空会话：邀请提示与复制房间信息（2 秒内显示“已复制”） | `transcript`、`copyButton`、`copyRoom` | 渲染图 alone | 已验证 |
| R8 | 晚加入的成员收到最近 150 条群聊历史（大于 16 KiB 时分片） | `flashchat.ChatRoom` | 互通测试 history 两个方向 | 已验证 |
| R9 | 昵称修改同步到房间内所有会话 | `updateNickname`、`Session.SetNickname` | 互通测试 nickname 两个方向 | 已验证 |
| R10 | 房间详情：房间名称、昵称、房间号与密码（只读）、复制、历史与加密说明、离开 | `dialogs` | 渲染图 room | 已验证 |
| R11 | 出错提示可关闭；送达失败、停止分享失败、成员通信失败分别说明 | `roomError`、`conversation` | 代码审查 | 待验证 |

## 文件

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| F1 | 选择文件分享到当前会话：Host 启动 `share_server`，等待就绪与中继就绪后把 Tailcat 地址作为 offer | `shareFiles`、`shares.Offer`、`tasks.ready` | 互通测试 file offer 两个方向；`TestTwoPagesTalkShareAndLeave` | 已验证 |
| F2 | 拖放文件到会话即分享（忙碌或不在房间时不接收） | `widgets.DropArea`、`offerDropped` | 代码审查 | 待验证（真实拖放） |
| F3 | 停止分享；离开房间时停止全部分享 | `stopSharing`、`shares.RescindAll` | 互通测试 page stops its offer；`TestTwoPagesTalkShareAndLeave` | 已验证 |
| F4 | 浏览并接收：Host 启动 `share_client`（`127.0.0.1:0`），打开其网页；同一地址复用任务 | `receive`、`openBrowse` | 互通测试 page receives through its Host | 部分验证（WebView 实窗待测） |
| F5 | 页面只停止自己启动的任务；关闭窗口时离开房间并停止全部任务（最多等待 3 秒） | `tasks`、`Close`、`app/window_windows.go` 调用 `shell.Discarded` | `TestTwoPagesTalkShareAndLeave` | 部分验证（实窗退出待测） |

## 外壳

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| A1 | 侧栏底部“闪聊”入口 | `shell/pages.go` 的 `flash-chat` | 代码审查 | 待验证（实窗导航） |
| A2 | 在房间中或有草稿时离开页面先确认；忙碌时报告忙碌 | `View` 中的 `Activity.Report` | 代码审查 | 待验证 |

## 未验证范围

- 与手机浏览器和另一台电脑跨网络互通（公共 tracker、TURN）未测；互通测试只在本机。
- 物理中文输入法、真实拖放、WebView 文件页面与窗口退出流程需要实窗验证。
