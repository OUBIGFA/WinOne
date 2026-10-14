# 文件与目录链接：来源与功能映射

本模块按 WinOne 的 Go Host、统一设置组件及本地权限协议重新适配。没有启动或分发上游独立 UI，也没有使用拼接 CMD 来执行日常创建操作。可审阅的 CMD 是用户主动导出的计划，不是后端执行路径。

固定参考：

| 项目 | 固定提交 | 许可证 |
| --- | --- | --- |
| [Benisgo/Symbolic11](https://github.com/Benisgo/Symbolic11/tree/b3663c42a6dcc5a44ab0ab0163d5566a05c9ef33) | `b3663c42a6dcc5a44ab0ab0163d5566a05c9ef33`（v1.3） | GPL-3.0；WinOne 根 LICENSE 保留 GPLv3 条款 |
| [arnobpl/SymlinkCreator](https://github.com/arnobpl/SymlinkCreator/tree/6efc382c3f31c45387c3bb3e0571a0c2650134d1) | `6efc382c3f31c45387c3bb3e0571a0c2650134d1`（v2.0.1 后仅 README 变化） | MIT，Copyright 2019–2026 Arnob Paul |
| [clayrisser/mklink](https://github.com/clayrisser/mklink/tree/d6eb377348db59f062ec79a186f5212a9dfba03d) | `d6eb377348db59f062ec79a186f5212a9dfba03d` | MIT，Copyright 2017 Jam Risser |

## 上游用户功能的接入位置

| 上游功能/证据 | WinOne 入口与执行 | 验证 |
| --- | --- | --- |
| Symbolic11、mklink：文件硬链接、目录联接、文件/目录符号链接；`Pages/CreateLink.xaml.cs`、`Application/Home.cs` | 单项/批量创建，`Manager.Preview` 根据真实属性检查类型；`Execute` 调用 Win32/Go 原生文件 API | `TestCreatesAndChecksAllLinkKinds`；符号链接的权限门禁必须在已授权提升环境完成，1314 不当成功 |
| Symbolic11：选择文件/目录、自定义链接名称/位置 | 前端单项创建表单和统一原生 picker；执行前预览两端方向及具体类型 | 前端预览失效/显式创建测试、`TestPlansRefuseConflictsAndRecheckSourceIdentity` |
| Symbolic11 LinkView：指定目录递归查找、取消、打开位置、移除 | 查找已有链接标签，`Scan` / `scan.cancel`、`Reveal`、`Remove`；不沿 junction/symlink 跳出扫描范围 | `TestScanDoesNotTraverseJunctionTargets`、真实回收测试 |
| Symbolic11 LinkView：读取现有 `.lnk` 快捷方式的目标 | 仅发现/检查现有快捷方式，`shortcutTarget` 使用 OS COM 只读 TargetPath；不执行目标 | `TestScanReadsExistingShortcutWithoutExecutingOrChangingIt` |
| SymlinkCreator：多个文件/文件夹到同一目标目录、重复名称与目标冲突检查 | 批量创建标签；`PreviewBatch` 全批检查，`CreateBatchWith` 逐项核验结果，首次失败后停止余项 | `TestBatchCreatesAllEntriesAndRetainsPerItemResults`、`TestBatchStopsAfterActualFailureAndReportsUnattemptedEntries` |
| SymlinkCreator：原生文件/目录多选、资源管理器拖放、逐行文本输入/拖放 | `pickFiles`、`pickDirectories`，统一 native drop 事件，批量源路径输入；多次输入按顺序保留 | 前端多路径事件测试；native picker/drop 由 Rust 宿主集成验收 |
| SymlinkCreator：绝对/相对目标 | 符号链接的相对路径选项；计划展示实际 target，跨卷相对路径明确拒绝 | 四种类型真实检查、计划测试；导出脚本采用同一计划 |
| SymlinkCreator：保留生成的 `.cmd` 供检查 | 单项/批量导出计划，批量“创建时保留命令计划”选项；保存位置由用户选择 | `TestExportedBatchQuotesRealPathsAndCreatesWorkingLinks` 真正执行生成的脚本，覆盖中文、空格、`% ! &` |
| SymlinkCreator：隐藏成功提示、保留错误提示 | 批量“全部成功时不显示结果提示”；部分失败和错误仍显示，结果保留历史 | 前端部分结果测试；`BatchResult.Completed/Total` 来自实际核验 |
| SymlinkCreator：通过快捷方式复用启动选项 | `open/pending` 的 `options` 支持 kind、relative、retainScript、hideSuccess；只排队预填，root 的启动参数解析/激活负责传入 | `TestDirectoryLinksOpenQueuesOnlyPathsAndConsumesInOrder`；宿主启动参数集成测试负责 CLI |
| mklink：文件和目录右键菜单，用选中路径打开界面 | `ExplorerMenu` / `ConfigureExplorerMenu`；仅当前用户的 `*\shell\WinOne.DirectoryLinks` 和 `Directory\shell\WinOne.DirectoryLinks`。命令 `WinOne.exe --directory-links --source "%1"` 不自动创建 | 私有注册表命令/所有权测试；真实 Explorer 菜单默认不开启 |
| 上游主题、语言、错误和完成反馈 | WinOne 统一主题、中英资源、shadcn/Base UI 与 Notice；不保留独立 WPF/WinForms 设置入口 | 前端行为与双宽度视觉检查；全局宿主统一验收 |

上游三项目没有“移动既有目录内容、再用链接替换原目录”的迁移器。SymlinkCreator README 明确不移动、复制或删除源内容；因此本模块不凭该示例新增自动搬迁行为。四种创建类型和上游实际批量/查找/管理能力均保留；WinOne 的目标平台仍按项目约定为 Windows 11 x64。

## 相比上游执行方式的必要适配

- Symbolic11 创建路径没有检查 CMD 退出码即报告成功。这里改为创建后读取真实类型、原始/解析目标、文件身份和可访问性。
- 硬链接不是带有“原源地址”的重解析点。原源路径缺失时，File ID 与链接数大于 1 才说明仍有其他别名；只剩一个名称显示“数据可用·原关联已消失”。相同字节但不同 File ID 显示“与源文件不一致”。
- 创建使用一次性预览 token 和路径/对象身份复核。提权通过固定 Host helper、同用户 pipe、双方 PID/程序路径、提升状态和 nonce 验证；单批次仅一个授权 helper。
- 移除改为回收选中入口，最终复核身份及 reparse 数据。它不递归删除链接目标；原生回收 sink 拒绝对链接目标/子项的回调。
- 操作历史是本机状态，独立于可迁移的全局偏好。打开其他页面不会启动后台扫描；查找只在明确操作时执行。

原生 API 依据：[CreateSymbolicLinkW](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-createsymboliclinkw)、[CreateHardLinkW](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-createhardlinkw)、[hard links and junctions](https://learn.microsoft.com/en-us/windows/win32/fileio/hard-links-and-junctions)、[FILE_ID_INFO](https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_id_info)、[BY_HANDLE_FILE_INFORMATION](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/ns-fileapi-by_handle_file_information)。

## MIT notices

Copyright (c) 2019-2026 Arnob Paul

Copyright (c) 2017 Jam Risser

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
