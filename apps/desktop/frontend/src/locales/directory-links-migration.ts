export const directoryMigrationMessages={
 zh:{
  migrationTitle:"搬迁与还原",
  migrationIntro:"将目录复制并校验到新位置，在原位置保留目录联接；以后可从历史记录还原。原数据备份仅移入回收站，清理失败时保留并显示位置。",
  migrationLimits:"仅支持本机 NTFS 普通目录，每次最多 10 万个项目。不跟随目录中的联接或符号链接。请先退出相关程序；占用检查不能保证服务、驱动或内存映射不再写入。",
  migrationSource:"要搬迁的目录",migrationDirectory:"新的存放文件夹",migrationName:"新目录名称",migrationNameHint:"留空沿用原目录名称",migrationNew:"新建搬迁",migrationPreview:"检查并预览",migrationStart:"开始搬迁",migrationRestore:"还原到原位置",migrationPlan:"已核验的操作计划",migrationSize:"{{count}} 个项目 · {{size}}",migrationProgress:"目录任务进度",migrationCancel:"取消任务",migrationReadAgain:"重新读取本次任务",
  migrationUncertain:"请求结果暂未确认，请读取本次任务或查看历史；不要重复提交文件操作。",
  migrationConfirm:"确认搬迁目录？",migrationConfirmHint:"关闭正在使用此目录的程序。完成后原路径仍可使用，但数据位于新位置；复制或校验失败会保留原数据。",migrationAcknowledge:"我已退出相关程序，并了解此目录的风险",migrationConfirmAction:"确认执行",
  migrationCleanupHint:"仅将下面经身份核验的备份移入回收站，不永久删除，也不清理其他目录。",
  migrationRecoverHint:"核对中断记录：若切换已经完成，仅补齐历史；否则在原位置空闲时恢复原入口。不会覆盖现有项目，也不会自动删除副本。",
  migrationHistory:"搬迁记录",migrationRetained:"保留的备份",migrationPlanReady:"预览已准备好，尚未搬迁",
  migrationAction:{restore:"还原目录",recover:"恢复中断任务",cleanup:"备份移入回收站"},
  migrationState:{running:"任务进行中",completed:"任务已完成",failed:"任务未完成",cancelled:"任务已取消"},
  migrationPhase:{checking:"检查目录",copying:"复制数据",verifying:"逐项校验",verified:"校验通过","backed-up":"原入口已备份",committing:"切换入口",committed:"入口已切换",cleanup:"处理备份",complete:"已完成",failed:"未完成","recovery-required":"需要恢复"}
 },
 en:{
  migrationTitle:"Move and restore",
  migrationIntro:"Copy and verify a folder at its new location, leaving a directory junction at the original path. Restore it from history later. Original data backups go only to the Recycle Bin; any backup that cannot be recycled is retained and listed.",
  migrationLimits:"Local NTFS folders only, up to 100,000 entries per operation. Junctions and symbolic links inside the folder are not followed. Close related applications first; occupancy checks cannot guarantee that services, drivers or mapped files have stopped writing.",
  migrationSource:"Folder to move",migrationDirectory:"New parent folder",migrationName:"New folder name",migrationNameHint:"Leave empty to keep the original name",migrationNew:"New move",migrationPreview:"Check and preview",migrationStart:"Start move",migrationRestore:"Restore original location",migrationPlan:"Verified operation plan",migrationSize:"{{count}} entries · {{size}}",migrationProgress:"Folder operation progress",migrationCancel:"Cancel operation",migrationReadAgain:"Read this operation again",
  migrationUncertain:"The outcome is not confirmed. Read this operation or inspect history; do not resubmit the file operation.",
  migrationConfirm:"Move this folder?",migrationConfirmHint:"Close applications using this folder. The original path will keep working, with data at the new location. A copy or verification failure preserves the original data.",migrationAcknowledge:"Related applications are closed and I understand this folder’s risks",migrationConfirmAction:"Confirm operation",
  migrationCleanupHint:"Move only the identity-verified backup below to the Recycle Bin. Nothing is permanently deleted and other folders are not cleaned.",
  migrationRecoverHint:"Inspect the interrupted operation. If the switch completed, finalize its history only; otherwise restore the original entry when its name is free. Existing entries are never overwritten and copies are not automatically removed.",
  migrationHistory:"Move record",migrationRetained:"Retained backup",migrationPlanReady:"Preview ready; no data has been moved",
  migrationAction:{restore:"Restore folder",recover:"Recover interrupted operation",cleanup:"Recycle backup"},
  migrationState:{running:"Operation in progress",completed:"Operation completed",failed:"Operation incomplete",cancelled:"Operation canceled"},
  migrationPhase:{checking:"Checking folder",copying:"Copying data",verifying:"Verifying contents",verified:"Verified","backed-up":"Original entry backed up",committing:"Switching entry",committed:"Entry switched",cleanup:"Handling backups",complete:"Completed",failed:"Incomplete","recovery-required":"Recovery required"}
 }
};
