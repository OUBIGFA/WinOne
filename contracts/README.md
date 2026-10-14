# 本地协议 v1

每条消息由 4 字节小端长度和 UTF-8 JSON 组成，大小限制 1 字节至 1 MiB。一个连接处理一条请求；超时 5 秒，修改请求不自动重试。

请求字段：`version`、`id`、`method`、可选 `params`。响应返回同一 `id` 和 `result` 或 `error`。当前命令：

| 命令 | 参数 | 结果 |
| --- | --- | --- |
| `snapshot` | 无 | 设置及模块状态 |
| `settings.save` | 完整设置，包含 revision | 保存后的设置 |

设置 revision 不匹配返回 `conflict`。未知版本或命令拒绝执行。尚未接入的模块返回 unavailable，界面不得显示为已启用。

诊断命令使用 `module` 选择 `host`、`capture`、`always-on-top` 或 `wintray`，不接受文件路径，也不启动模块：

| 命令 | 结果 |
| --- | --- |
| `diagnostics.read` | `folderAvailable`、`entries`；每项包含 `name`、`available`、`truncated`、`text`，最多读取各日志末尾 64 KiB |
| `diagnostics.folder` | 已校验的现存日志文件夹 `path`，由界面按用户操作打开 |

没有日志时返回空内容；文件夹不存在时不能打开。未知模块、非普通日志文件或越界链接均返回错误。

后续 Worker 协议沿用分帧与错误约定，具体功能命令随模块接入添加。本版只支持请求/响应，增量事件在模块生命周期接入时增加。

## WinTray

Host 将 `wintray.*` 转发到 WinTray Worker，保留请求 ID 并移除命名空间前缀。

| 命令 | 参数 | 结果 |
| --- | --- | --- |
| `snapshot` | 无 | 配置、内容修订号、恢复错误及后台错误 |
| `settings.save` | revision、settings | 异步任务 ID |
| `launch` | 程序 id | 异步任务 ID |
| `jobs.get` | 任务 id | running / completed / failed 及结果 |
| `icons` | 无 | 原生图标稳定 ID、名称和 PNG 字节 |
| `icon.activate` | 图标 id、right | 原生回调发送结果 |
| `shutdown` | 无 | 开始恢复并退出，Host 继续等待进程结束 |

保存和启动的请求返回不代表任务完成，页面必须查询任务最终结果。独立测试数据目录禁止修改真实登录触发器。

`recover` 恢复未完成的设置事务，返回异步任务。存在恢复记录时禁止新保存和程序启动，损坏记录不自动覆盖。

## Capture

Host 将 `capture.*` 转发到按需启动的 Capture Worker。首次启动允许 15 秒，其余消息仍有长度和连接期限限制。

命令：`snapshot`、`settings.save`、`capture.start`、`capture.unified`、`draft.resume`、`edit.last`、`copy.last`、`pin.last`、`pins.close`、`jobs.get`、`shutdown`。

`capture.start` 的 mode 为 region、window、fullscreen 或 scroll。图像操作返回任务 ID，状态为 running、completed、cancelled 或 failed。图像经本地 PNG 文件交付，管道只传递元数据；复制失败后仍保留已保存文件。

`capture.unified` 的普通和滚动截图共用原生画布；重复调用唤起正在进行的原生截图。`draft.resume` 接收 `expectedId` 和可选 `language`，只恢复匹配的草稿；旧格式验证并转换成功后继续恢复，失败保留原文件，缺失或过期标识返回错误。截图编辑没有整体等待时限，单次管道请求仍受超时约束；`cancelled` 必须按取消处理，不能显示完成。

最近截图操作携带显示时的 `expectedId`，`save.last` 另带 `path`。`edit.last` 使用原生编辑窗口，已有草稿时拒绝覆盖；编辑后的复制不改写原图，未完成编辑可通过原生草稿恢复。

原生滚动编辑器的重截属于当前截图任务：采集期间任务仍为 running，`scroll.stop` 使用该任务的 ID。取消或写入失败不替换原稿；替换提交后，后续编辑和恢复使用新草稿标识。整个任务在截图窗口确认或取消后结束。

正在截图或编辑时，普通 shutdown 请求会被拒绝，提示先完成或取消。空闲且没有贴图时 Worker 自动退出，下次显式操作重新启动。

## Updates

更新是 Host 控制服务，不启动模块引擎。来源和当前版本由构建提供，用户配置及 RPC 参数不能覆盖发布者。

| 命令 | 参数 | 结果 |
| --- | --- | --- |
| `updates.info` | 无 | version、configured；只读，不检查网络 |
| `updates.check` | id | 当前检查状态；同窗口、同 id 的重复请求复用结果 |
| `updates.status` | id | checking / available / up_to_date / not_configured / failed |
| `updates.cancel` | id | 请求取消后的状态，继续读取至结束 |
| `updates.release` | id | 该检查已确认的发布页 url |

检查绑定实际设置进程，状态、取消和发布页仅供原进程读取；进程退出会取消检查并释放观察句柄。同时只运行一个检查，其他 id 在检查期间被拒绝。完成状态携带当前版本，发现更新时还携带最新版本和发布页；失败携带 errorCode。

取消优先于迟到的网络结果。失败、取消或新检查不得沿用旧发布页；界面只在用户点击时打开当前检查确认的链接。HTTP 响应限制 1 MiB、检查超时 15 秒，不跟随重定向，不下载或执行更新包。
