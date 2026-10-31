# 互联助手页（interconnect）功能对照

更新：2026-10-31。React 来源：`apps/desktop/frontend/src/features/interconnect.tsx`、`interconnect-general.tsx`、`interconnect-transfer.tsx`、`interconnect-peer.tsx`、`interconnect-intents.ts`，以及 `main.tsx` 的挂载方式。MyGo 实现：`apps/desktop-mygo/pages/interconnect`；Explorer“使用 WinOne 互传”预填由 `shell.ReadIntents` 读取（`interconnect.pending`）。闪聊（`interconnect-chat-*`）是独立页面，不在本表。

状态：**已验证** = 有单元测试或真实窗口截图证据；**待验证** = 已实现，还缺证据。截图保存在本地 `_temp/mygo-frontend-20261030/ic-*.png`（不入库）。

## 有意改变的地方

| 改变 | 原因 |
| --- | --- |
| “已复制”等提示显示为页内状态文字，保存成功为 Toast | 与其他页面一致。 |

## 页面级

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| I1 | 读取 `interconnect.get`（共享状态、地址）与 `general.get`；读取失败显示错误与“重新读取” | `page.reload` | 测试 | 已验证 |
| I2 | 剪贴板识别：设置开启时，打开页面或窗口重新获得焦点时读取剪贴板；识别持续分享地址（tc…）、云端分享（croc-store / getcroc / gofile 链接）、croc 口令；只提示，用户点“使用此口令”后填入对应标签的接收框 | `page.readClipboard` + `env.Focuses` | 测试 `TestClipboardCodeIsOfferedNotUsed` | 已验证 |
| I3 | Explorer 预填：切到文件互传，发送方向，加入路径（或接收口令） | `env.TakeIntents("interconnect")` + `transferPanel.Apply` | 测试 `TestSendingFilesStartsOneShare`（预填路径去重） | 已验证 |
| I4 | 保存（共享设置与常规设置一起）：只保存改动的部分，各自成功后各自确认；端口须为 0–65535 整数；撤销还原两份草稿 | `page.save` / `page.undo`；活动来源 `interconnect` | 测试 `TestSettingsSaveOnlyTheChangedPart` | 已验证 |
| I5 | 四个标签切换经页面的未保存保护：有未保存修改时询问放弃或继续编辑 | `kit.Tabs(..., env.RequestChange)` | 测试 `TestTabsAskTheGuard` | 已验证 |

## 文件互传 / 云端分享（同一面板，云端为 stored）

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| T1 | 操作：发送/接收；发送内容：文件与文件夹/文本；云端：GoFile / croc 加密分享，粘贴识别服务 | `transferPanel.composer` | 截图 ic-1 | 已验证 |
| T2 | 发送文件：拖入、选择文件、添加文件夹、粘贴文件（剪贴板文件列表）；已选列表、单项移除、清空；路径不区分大小写去重 | `sendInput` + `widgets.DropArea` + `Services().ReadClipboardData(FileList)` | 测试 | 已验证 |
| T3 | 发送到正在运行的分享（继续当前分享/新建独立分享）；新分享启动 `share_server`（含跨网口令发送配置）；成功后清空草稿并显示新任务 | `transferPanel.start` | 测试 | 已验证 |
| T4 | 接收：口令或链接、粘贴口令；tc… 走 `share_client`；其他走 `transfer_start`（croc 接收时 transport=auto）；GoFile 接收打开下载页 | `transferPanel.start` | 代码审查 | 待验证 |
| T5 | 高级选项：云端服务地址、下载次数、有效期、保存目录；网络设置（DERP、监听、中继/IPv6 中继/密码、发送端 IP、组播、中继端口列表、SOCKS5、HTTP 代理、仅局域网、不启动局域网中继）；允许接收方写入；传输选项（自定义口令、传输方式、同名处理、曲线、摘要、限速、ZIP、gitignore、不压缩、关闭多路、核对对方、排除规则） | `transferPanel.advanced` | 代码审查 | 待验证 |
| T6 | 任务列表：进度条与速度、等待提示、二维码/链接/口令与复制、打开下载页、到期与下载次数、原生口令、已撤销/已过期、错误与警告、确认问题（是/否/回复）、清单与接收、收到文本（打开/复制全文）、取消/撤销/移除/再次使用、过程详情；新任务就绪后滚动到它 | `transferPanel.taskView` | 代码审查 | 待验证 |
| T7 | 每 750 毫秒刷新任务（仅当前标签） | `transferPanel.View` 中 `c.After` | 代码审查 | 待验证 |

## 持续分享任务与端口穿透

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| P1 | 端口穿透：分享端口/映射端口、目标端口或连接口令、网络设置（DERP、监听、远端端口）、开始/连接；服务列表每秒刷新 | `peerPanel` | 代码审查 | 待验证 |
| P2 | 发布的分享：接收方式（扫码接收 / 局域网直连）、局域网二维码、复制局域网链接、选择访问地址、访问说明；WinOne 分享地址与跨网状态；中继失败详情 | `peerTask.routes` / `View` | 代码审查 | 待验证 |
| P3 | 跨网口令发送：二维码、复制跨网链接、原生口令提示、口令、进度、错误、确认问题、复制口令、停止/再次发送、详情 | `peerTask.codeTransfer` | 代码审查 | 待验证 |
| P4 | 管理共享内容：发布文本、条目（复制文本/复制下载链接/另存为/移除）、添加文件或文件夹；另存为对话框（路径、选择目录、替换已有） | `peerTask.manage` / `saveDialog` | 代码审查 | 待验证 |
| P5 | 复制地址、二维码开关、浏览并接收（任务下方内嵌分享页，高 480，可下载到“下载”文件夹）、复制访问链接、停止/恢复/移除 | `peerTask.View` + `web.Surface` | 代码审查 | 待验证 |

## 常规

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| G1 | 共享设置：访问范围（全部网卡/仅本机/各网卡地址）、端口、接收目录与选择 | `page.generalView` | 截图 ic-2 | 已验证 |
| G2 | 常规：默认接收目录（只读+选择）、完成通知、剪贴板识别（存入草稿）；悬浮收件入口、资源管理器发送入口（立即生效，冲突/不完整提示）；后台说明 | `generalPanel` | 截图 ic-2 | 已验证 |
