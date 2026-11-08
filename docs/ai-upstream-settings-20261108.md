# 上游 Settings 逐字段核对（2026-11-08）

固定上游 `691a10446ec2b66142cee2203edf72d73b12d76d` 的 `internal/settings/settings.go` 共185个JSON字段。本表区分AI数据、WinOne自有窗口职责和用户明确排除的外部客户端功能。字段存在仅是接入位置证据，真实执行与测试结论见 [完整接入记录](ai-full-integration-20261108.md)。

| 上游字段 | 归属或处理 | 定位 |
| --- | --- | --- |
| `Theme` / `theme` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `Colorblind` / `colorblind` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `ReduceMotion` / `reduceMotion` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `NoBird` / `noBird` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `Lang` / `lang` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `Tray` / `tray` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `PanelShortcut` / `panelShortcut` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `SessionTerminal` / `sessionTerminal` | 外部客户端会话/配置或其专属展示，本次排除 | 用户明确范围；modules/ai-magpie/settings.go 的 SessionReads / NoWSLAgents |
| `Currency` / `currency` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ChineseUnits` / `chineseUnits` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `Dock` / `dock` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `DockWindow` / `dockWindow` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `DockNever` / `dockNever` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `DockSettled` / `dockSettled` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `Proxy` / `proxy` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ProxyCustom` / `proxyCustom` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `OTel` / `otel` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `GitHubToken` / `githubToken` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `Redact` / `redact` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactModelScore` / `redactModelScore` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactShape` / `redactShape` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactModel` / `redactModel` | AI设置或供应商数据，传入私有运行时 | internal/ai/privacy_windows.go；modules/ai-magpie/settings.go |
| `RedactModelName` / `redactModelName` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactModelOff` / `redactModelOff` | 本地隐私模型反向开关 | modules/ai-magpie/settings.go |
| `RedactModelAuto` / `redactModelAuto` | 本地模型显式下载边界 | modules/ai-magpie/settings.go |
| `RedactPersonal` / `redactPersonal` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactKinds` / `redactKinds` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactWords` / `redactWords` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactWordsAnyCase` / `redactWordsAnyCase` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactWordsWhole` / `redactWordsWhole` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactRules` / `redactRules` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactActions` / `redactActions` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactAllow` / `redactAllow` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RedactRouteTo` / `redactRouteTo` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `LAN` / `lan` | AI设置或供应商数据，传入私有运行时 | internal/ai/gateway_network.go；modules/ai-magpie/settings.go |
| `LANKey` / `lanKey` | Host 主密钥与独立访问密钥，DPAPI保存 | internal/ai/gateway_windows.go；gateway_access.go |
| `CORSOrigins` / `corsOrigins` | AI设置或供应商数据，传入私有运行时 | internal/ai/gateway_network.go；modules/ai-magpie/settings.go |
| `AllowIPs` / `allowIPs` | AI设置或供应商数据，传入私有运行时 | internal/ai/gateway_network.go；modules/ai-magpie/settings.go |
| `FetchPrivate` / `fetchPrivate` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RouterNote` / `routerNote` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RouterAllTools` / `routerAllTools` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `Port` / `port` | AI设置或供应商数据，传入私有运行时 | internal/ai/gateway_windows.go；modules/ai-magpie/settings.go |
| `LANKeyID` / `lanKeyId` | Host 主密钥与独立访问密钥，稳定身份 | internal/ai/gateway_windows.go；gateway_access.go |
| `RequestArchive` / `requestArchive` | 旧上传归档选项排除，本机正文归档独立提供 | RequestArchiveLocal；modules/ai-magpie/patches/usage_embed.go.txt |
| `RequestArchiveMaxMB` / `requestArchiveMaxMB` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RequestArchiveLocal` / `requestArchiveLocal` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RequestArchiveLocalMB` / `requestArchiveLocalMB` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `GatewayConversations` / `gatewayConversations` | 网关本机记录，独立同意与私有数据 | modules/ai-magpie/patches/routing_gui.go.txt；gateway_history.go；TestGatewayRecordingConsentPersistsWithoutExternalHistory |
| `SessionReads` / `sessionReads` | 外部客户端会话/配置或其专属展示，本次排除 | 用户明确范围；modules/ai-magpie/settings.go 的 SessionReads / NoWSLAgents |
| `CodexWarmup` / `codexWarmup` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ClaudeWarmup` / `claudeWarmup` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexWarmAt` / `codexWarmAt` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ClaudeWarmAt` / `claudeWarmAt` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexWarmAts` / `codexWarmAts` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ClaudeWarmAts` / `claudeWarmAts` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexWarmAtOf` / `codexWarmAtOf` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexAutoReset` / `codexAutoReset` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexAutoResetOnly` / `codexAutoResetOnly` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `CodexResetLead` / `codexResetLead` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexNoCredits` / `codexNoCredits` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexCreditsLast` / `codexCreditsLast` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexCreditsFirst` / `codexCreditsFirst` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `WorkBuddyCheckin` / `workbuddyCheckin` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `TraeCheckin` / `traeCheckin` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `MiniMaxCheckin` / `minimaxCheckin` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `QoderCheckin` / `qoderCheckin` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `PluginCheckins` / `pluginCheckins` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `PluginOptions` / `pluginOptions` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CheckinOff` / `checkinOff` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `ReplyModel` / `replyModel` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `MemberModel` / `memberModel` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `NoLoopGuard` / `noLoopGuard` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ClaudeLeanPrompt` / `claudeLeanPrompt` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ClaudePrintMode` / `claudePrintMode` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ClaudeRunsMost` / `claudeRunsMost` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RefusalRetries` / `refusalRetries` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `RetryAfterCap` / `retryAfterCap` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `NoAccountDiscovery` / `noAccountDiscovery` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `SignInLinkOnly` / `signInLinkOnly` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `NoStats` / `noStats` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `NoUsageStats` / `noUsageStats` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `NoUpdatePill` / `noUpdatePill` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `UpdateSkip` / `updateSkip` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `NoAutoUpdate` / `noAutoUpdate` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `UpdateEvery` / `updateEvery` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `NoAutoDownload` / `noAutoDownload` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `AutoRestart` / `autoRestart` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `UpdateMirror` / `updateMirror` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `UpdateChannel` / `updateChannel` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `Vision` / `vision` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `VisionEffort` / `visionEffort` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `ImageGen` / `imageGen` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ImageTimeout` / `imageTimeout` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ClassifyTimeout` / `classifyTimeout` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `GroupSink` / `groupSink` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `GroupFirstToken` / `groupFirstToken` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `GroupPatience` / `groupPatience` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `BusyLoad` / `busyLoad` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `Searcher` / `searcher` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `SearcherThen` / `searcherThen` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `SearcherEffort` / `searcherEffort` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `SearchFirst` / `searchFirst` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `SearchAll` / `searchAll` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `SearchCount` / `searchCount` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `TrayUsages` / `trayUsages` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `TrayUsage` / `trayUsage` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `TrayUsageEvery` / `trayUsageEvery` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `TrayNoLogos` / `trayNoLogos` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `TrayNoBird` / `trayNoBird` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `TrayNoNumber` / `trayNoNumber` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `TrayPace` / `trayPace` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `TrayNoCost` / `trayNoCost` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `TrayWindows` / `trayWindows` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `Lightweight` / `lightweight` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `QuitNoAsk` / `quitNoAsk` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `KeepAwake` / `keepAwake` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `NoWSLAgents` / `noWSLAgents` | 外部客户端会话/配置或其专属展示，本次排除 | 用户明确范围；modules/ai-magpie/settings.go 的 SessionReads / NoWSLAgents |
| `KeepAwakeDisplay` / `keepAwakeDisplay` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `GatewayMode` / `gatewayMode` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `QuotaLeft` / `quotaLeft` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `QuotaReads` / `quotaReads` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `UsageAlert` / `usageAlert` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `BalanceAlert` / `balanceAlert` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ResetReminder` / `resetReminder` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `TurnDoneAlert` / `turnDoneAlert` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `MaskAlert` / `maskAlert` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `PlainNames` / `plainNames` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `PlainOwnNames` / `plainOwnNames` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `BareModelIDs` / `bareModelIDs` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `CodexAgentsV1` / `codexAgentsV1` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexTitles` / `codexTitles` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexDescriptions` / `codexDescriptions` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `CodexPredictions` / `codexPredictions` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `CodexAutoReview` / `codexAutoReview` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexSubagentModel` / `codexSubagentModel` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `DesktopSubagentModel` / `desktopSubagentModel` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `DesktopSubagentEffort` / `desktopSubagentEffort` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `FullContext` / `fullContext` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `DesktopLongest` / `desktopLongest` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `GuideHidden` / `guideHidden` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `CompactAt` / `compactAt` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `AgentCompacts` / `agentCompacts` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `CodexAsyncCompact` / `codexAsyncCompact` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `CodexAsyncCompactPercent` / `codexAsyncCompactPercent` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `ChinaMirror` / `chinaMirror` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `NPMRegistry` / `npmRegistry` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `TextSize` / `textSize` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `UIFont` / `uiFont` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `CodeFont` / `codeFont` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `AgentOrder` / `agentOrder` | 外部客户端会话/配置或其专属展示，本次排除 | 用户明确范围；modules/ai-magpie/settings.go 的 SessionReads / NoWSLAgents |
| `AgentsHidden` / `agentsHidden` | 外部客户端会话/配置或其专属展示，本次排除 | 用户明确范围；modules/ai-magpie/settings.go 的 SessionReads / NoWSLAgents |
| `AgentsShown` / `agentsShown` | 外部客户端会话/配置或其专属展示，本次排除 | 用户明确范围；modules/ai-magpie/settings.go 的 SessionReads / NoWSLAgents |
| `AgentsHere` / `agentsHere` | 外部客户端会话/配置或其专属展示，本次排除 | 用户明确范围；modules/ai-magpie/settings.go 的 SessionReads / NoWSLAgents |
| `UsageOrder` / `usageOrder` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `PanelUsageHidden` / `panelUsageHidden` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `UsageAgentsHidden` / `usageAgentsHidden` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `UsageCardsHidden` / `usageCardsHidden` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ClientAgents` / `clientAgents` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `UsageCleared` / `usageCleared` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `Visible` / `visible` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `HiddenModels` / `hiddenModels` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `DisabledModels` / `disabledModels` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `PickedModels` / `pickedModels` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `OwnModelsHidden` / `ownModelsHidden` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `AgentEfforts` / `agentEfforts` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `OrderedModels` / `orderedModels` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `FastPicks` / `fastPicks` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `UltraPicks` / `ultraPicks` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_client_settings.go；modules/ai-magpie/settings.go |
| `ModelNames` / `modelNames` | AI设置或供应商数据，传入私有运行时 | internal/ai/configuration.go；modules/ai-magpie/settings.go |
| `ModelEfforts` / `modelEfforts` | 模型设置与上游运行时映射 | internal/ai/model_metadata.go；modules/ai-magpie/settings.go / installModelPreferences |
| `ModelReasonless` / `modelReasonless` | 模型设置与上游运行时映射 | internal/ai/model_metadata.go；modules/ai-magpie/settings.go / installModelPreferences |
| `ModelImages` / `modelImages` | 模型设置与上游运行时映射 | internal/ai/model_metadata.go；modules/ai-magpie/settings.go / installModelPreferences |
| `ModelPrices` / `modelPrices` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ModelOutputs` / `modelOutputs` | 模型设置与上游运行时映射 | internal/ai/model_metadata.go；modules/ai-magpie/settings.go / installModelPreferences |
| `ModelCompacts` / `modelCompacts` | 模型设置与上游运行时映射 | internal/ai/model_metadata.go；modules/ai-magpie/settings.go / installModelPreferences |
| `ModelWires` / `modelWires` | 模型设置与上游运行时映射 | internal/ai/model_metadata.go；modules/ai-magpie/settings.go / installModelPreferences |
| `ModelIDs` / `modelIDs` | 模型设置与上游运行时映射 | internal/ai/model_metadata.go；modules/ai-magpie/settings.go / installModelPreferences |
| `ModelAPIs` / `modelAPIs` | 模型设置与上游运行时映射 | internal/ai/model_metadata.go；modules/ai-magpie/settings.go / installModelPreferences |
| `ModelSameAs` / `modelSameAs` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `UsageAliases` / `usageAliases` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `ModelHeaders` / `modelHeaders` | 模型设置与上游运行时映射 | internal/ai/model_metadata.go；modules/ai-magpie/settings.go / installModelPreferences |
| `ModelKinds` / `modelKinds` | AI设置或供应商数据，传入私有运行时 | internal/ai/magpie_settings.go；modules/ai-magpie/settings.go |
| `Window` / `window` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `WindowMaximised` / `windowMaximised` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
| `WindowAt` / `windowAt` | Magpie独立应用窗口/托盘/系统生命周期，由WinOne自有界面与模块管理 | apps/desktop-mygo/shell；apps/host；不启动Magpie外壳 |
