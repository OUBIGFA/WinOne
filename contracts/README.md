# 本地协议 v1

每条消息由 4 字节小端长度和 UTF-8 JSON 组成，大小限制 1 字节至 1 MiB。一个连接处理一条请求；超时 5 秒，修改请求不自动重试。

请求字段：`version`、`id`、`method`、可选 `params`。响应返回同一 `id` 和 `result` 或 `error`。当前命令：

| 命令 | 参数 | 结果 |
| --- | --- | --- |
| `snapshot` | 无 | 设置及模块状态 |
| `settings.save` | 完整设置，包含 revision | 保存后的设置 |

设置 revision 不匹配返回 `conflict`。未知版本或命令拒绝执行。尚未接入的模块返回 unavailable，界面不得显示为已启用。

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

命令：`snapshot`、`settings.save`、`capture.start`、`edit.last`、`copy.last`、`pin.last`、`pins.close`、`jobs.get`、`shutdown`。

`capture.start` 的 mode 为 region、window、fullscreen 或 scroll。图像操作返回任务 ID，状态为 running、completed、cancelled 或 failed。图像经本地 PNG 文件交付，管道只传递元数据；复制失败后仍保留已保存文件。

正在截图或编辑时，普通 shutdown 请求会被拒绝，提示先完成或取消。空闲且没有贴图时 Worker 自动退出，下次显式操作重新启动。
