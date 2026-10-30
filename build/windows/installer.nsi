Unicode true
; OCR includes deeply nested files. Opt in to the Windows long-path policy so
; a supported destination does not fail part-way through installation/removal.
; This is an application manifest flag, not a change to the machine policy.
ManifestLongPathAware true
!ifdef WINONE_EXPORT_UNINSTALLER
  !include "export-uninstaller.nsh"
!endif
!ifdef WINONE_TEST_BUILD
  ; Select compression before includes can embed native plug-ins.
  SetCompressor zlib
  ; Integration fixtures check the complete inventory and install control flow.
  ; Avoid recompressing every OCR/model/plugin byte for each failure scenario;
  ; the distributed installer retains solid LZMA in the branch below.
  SetCompress off
!else
  SetCompressor /SOLID lzma
!endif
!include MUI2.nsh
!include LogicLib.nsh
!include WinVer.nsh
!include x64.nsh
!include FileFunc.nsh
!include "maintenance.nsh"
!include "directory.nsh"
!include "pending.nsh"
!include "recovery.nsh"
!include "uninstall.nsh"
!include "${WINONE_FILES_NSH}"
; Tests override destinations, not the installation/removal control flow.
!ifndef WINONE_REGISTRY_KEY
  !define WINONE_REGISTRY_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\WinOne"
!endif
!ifndef WINONE_SHORTCUT
  !define WINONE_SHORTCUT "$SMPROGRAMS\WinOne.lnk"
!endif
!ifndef WINONE_PROFILE_DIR
  !define WINONE_PROFILE_DIR "$LOCALAPPDATA\WinOne"
!endif

Name "WinOne"
OutFile "${WINONE_INSTALLER_OUT}"
InstallDir "$LOCALAPPDATA\Programs\WinOne"
RequestExecutionLevel user
SetFont "Microsoft YaHei UI" 9
BrandingText "WinOne"
ShowInstDetails hide
ShowUninstDetails hide
Var WinOneInstallStarted
Var WinOneRegistrationStarted
Var WinOneShortcutStarted
Var WinOneFailureCode
Var WinOneRepairMode
Var WinOneRegisteredLocation
Var WinOneUninstallRecovery
Var WinOneUpdateMode
Var WinOneUpdateError
!ifdef WINONE_TEST_BUILD
  Var WinOneFailurePoint
!endif
!define MUI_ICON "${WINONE_ICON}"
!define MUI_UNICON "${WINONE_ICON}"
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_LICENSE "${WINONE_PAYLOAD_DIR}\LICENSE.txt"
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE WinOneDirectorySelected
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\WinOne.exe"
!define MUI_FINISHPAGE_RUN_TEXT "$(LaunchNow)"
!define MUI_FINISHPAGE_RUN_FUNCTION WinOneFinishRun
!define MUI_PAGE_CUSTOMFUNCTION_SHOW WinOneFinishShow
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"
SetFont /LANG=${LANG_ENGLISH} "Segoe UI" 9
SetFont /LANG=${LANG_SIMPCHINESE} "Microsoft YaHei UI" 9

LangString NeedWindows ${LANG_ENGLISH} "WinOne requires Windows 11 (x64)."
LangString NeedWindows ${LANG_SIMPCHINESE} "WinOne 需要 Windows 11（x64）。"
LangString NeedWebView ${LANG_ENGLISH} "Install Microsoft Edge WebView2 Runtime, then run setup again."
LangString NeedWebView ${LANG_SIMPCHINESE} "请先安装 Microsoft Edge WebView2 Runtime，再运行安装程序。"
LangString NeedEmpty ${LANG_ENGLISH} "Choose an empty folder, or use the registered WinOne folder to update."
LangString NeedEmpty ${LANG_SIMPCHINESE} "请选择空文件夹，或在已登记的 WinOne 目录中覆盖安装。"
LangString Updating ${LANG_ENGLISH} "Verifying, backing up and updating WinOne. Your settings and accounts will be kept."
LangString Updating ${LANG_SIMPCHINESE} "正在校验、备份并更新 WinOne。你的配置和账号会保留。"
LangString Failed ${LANG_ENGLISH} "The operation could not finish. Keep the installation folder for recovery."
LangString Failed ${LANG_SIMPCHINESE} "操作未完成，请保留安装目录以便恢复。"
LangString UpdateFailedDetails ${LANG_ENGLISH} "The update did not finish.$\r$\n$WinOneUpdateError"
LangString UpdateFailedDetails ${LANG_SIMPCHINESE} "覆盖安装未完成。$\r$\n$WinOneUpdateError"
LangString MaintenanceCloseFailed ${LANG_ENGLISH} "File operations finished, but the maintenance helper has not exited. Close this window before retrying WinOne or uninstallation."
LangString MaintenanceCloseFailed ${LANG_SIMPCHINESE} "文件处理已完成，但维护程序尚未退出。请关闭此窗口后，再重试启动 WinOne 或卸载。"
LangString RolledBack ${LANG_ENGLISH} "Installation failed. Changes from this attempt were removed."
LangString RolledBack ${LANG_SIMPCHINESE} "安装未完成，本次更改已撤销。"
LangString ShortcutExists ${LANG_ENGLISH} "A WinOne shortcut already exists. Keep or rename it before installing."
LangString ShortcutExists ${LANG_SIMPCHINESE} "已有 WinOne 快捷方式。请先另存或重命名。"
LangString RepairUnavailable ${LANG_ENGLISH} "The recovery record does not match this folder or installer. No files were changed."
LangString RepairUnavailable ${LANG_SIMPCHINESE} "恢复记录与安装目录或安装包不匹配，未修改文件。"
LangString RepairQuestion ${LANG_ENGLISH} "The previous installation did not finish. Repair it now? Your settings will be kept."
LangString RepairQuestion ${LANG_SIMPCHINESE} "上次安装未完成。现在修复吗？你的设置会保留。"
LangString UninstallQuestion ${LANG_ENGLISH} "The previous uninstall did not finish. Continue uninstalling? Your settings will be kept."
LangString UninstallQuestion ${LANG_SIMPCHINESE} "上次卸载未完成。现在继续卸载吗？你的设置会保留。"
LangString LaunchNow ${LANG_ENGLISH} "Launch WinOne now"
LangString LaunchNow ${LANG_SIMPCHINESE} "立即启动 WinOne"
LangString LaunchFailed ${LANG_ENGLISH} "Installation is complete, but WinOne could not start. Close setup, then open WinOne from the Start menu."
LangString LaunchFailed ${LANG_SIMPCHINESE} "安装已完成，但未能启动 WinOne。请关闭安装程序，再从开始菜单打开 WinOne。"
LangString OnboardingFailed ${LANG_ENGLISH} "The first-use state directory could not be prepared. Existing permissions and records were kept."
LangString OnboardingFailed ${LANG_SIMPCHINESE} "无法准备首次提示目录。已有权限和记录保持不变。"

!insertmacro WinOneGuardFunctions ""
!insertmacro WinOneGuardFunctions "un."
!insertmacro WinOneRemovalFunctions "" WinOneCheckGuard
!insertmacro WinOneRemovalFunctions "un." un.WinOneCheckGuard
!insertmacro WinOnePendingFunctions ""
!insertmacro WinOnePendingFunctions "un."
!insertmacro WinOneRecoveryFunctions ""
!insertmacro WinOneRecoveryFunctions "un."
!include "repair-ui.nsh"
!include "upgrade.nsh"
!include "language.nsh"
!insertmacro WinOneLanguageFunctions ""
!insertmacro WinOneLanguageFunctions "un."

Function un.onInit
  Call un.WinOneInitializeLanguage
  StrCpy $WinOneGuardHandle 0
  StrCpy $WinOneGuardRelease 0
FunctionEnd

Function WinOneDirectorySelected
  ; Normal setup always installs this package. Recovery of an old operation is
  ; not a separate question and must never switch Install into Uninstall.
  ClearErrors
FunctionEnd

!include "finish.nsh"
!include "progress.nsh"
!include "onboarding.nsh"

Function .onGUIEnd
  Call WinOneCloseGuardHandle
FunctionEnd
Function un.onGUIEnd
  Call un.WinOneCloseGuardHandle
FunctionEnd

!ifdef WINONE_TEST_BUILD
; A silent fixture failure otherwise loses its reason when t.TempDir is cleaned.
; This callback runs only after failure and does not alter the install path.
Function .onInstFailed
  FileOpen $0 "${WINONE_PROFILE_DIR}.installer-failure.txt" w
  FileWrite $0 "guard=$WinOneGuardError;installStarted=$WinOneInstallStarted;registrationStarted=$WinOneRegistrationStarted;shortcutStarted=$WinOneShortcutStarted;repair=$WinOneRepairMode;registered=$WinOneRegisteredLocation;target=$INSTDIR$\r$\n"
  FileClose $0
FunctionEnd
!endif

Function .onInit
  Call WinOneInitializeLanguage
  ; An update reuses the Go transaction and never starts this guard. Initialize
  ; both handles so the shared close path never waits on an empty variable.
  StrCpy $WinOneGuardHandle 0
  StrCpy $WinOneGuardRelease 0
  StrCpy $WinOneUpdateMode 0
  StrCpy $WinOneRepairMode 0
  ${GetParameters} $0
  ${GetOptions} $0 "/REPAIR" $1
  ${IfNot} ${Errors}
    StrCpy $WinOneRepairMode 1
  ${EndIf}
  StrCpy $WinOneUninstallRecovery 0
  ${GetOptions} $0 "/FINISHUNINSTALL" $1
  ${IfNot} ${Errors}
    StrCpy $WinOneUninstallRecovery 1
  ${EndIf}
  ${If} $WinOneRepairMode == 1
  ${AndIf} $WinOneUninstallRecovery == 1
    SetErrorLevel 3
    Abort
  ${EndIf}
  SetRegView 64
  ReadRegStr $WinOneRegisteredLocation HKCU "${WINONE_REGISTRY_KEY}" "InstallLocation"
  ${If} $WinOneRegisteredLocation != ""
  ${AndIf} $WinOneRepairMode != 1
  ${AndIf} $WinOneUninstallRecovery != 1
    StrCpy $INSTDIR $WinOneRegisteredLocation
  ${EndIf}
  ${IfNot} ${AtLeastWin11}
  ${OrIfNot} ${IsNativeAMD64}
    MessageBox MB_ICONSTOP "$(NeedWindows)" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
FunctionEnd

; Check after the destination page so graphical uninstall recovery also works
; without WebView2 when registration was already removed before interruption.
Function WinOneCheckWebView
  ; Microsoft documents the machine key in the 32-bit registry view.
  SetRegView 32
  ReadRegStr $0 HKLM "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${EndIf}
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    MessageBox MB_ICONSTOP "$(NeedWebView)" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
FunctionEnd

Section "WinOne"
  StrCpy $WinOneFailureCode 1
  !ifdef WINONE_TEST_BUILD
    ${GetParameters} $0
    ${GetOptions} $0 "/FAILPOINT=" $WinOneFailurePoint
    ${If} $WinOneFailurePoint == "registration-removed-before-install"
      ; Model an uninstall completing while this installer wizard was open.
      DeleteRegKey HKCU "${WINONE_REGISTRY_KEY}"
      RMDir "$INSTDIR"
    ${EndIf}
  !endif
  ${If} $WinOneUninstallRecovery == 1
    Goto completeUninstall
  ${EndIf}
  ; An uninstall can finish while the wizard is open. Decide against current
  ; registration, not the .onInit snapshot used to suggest the directory.
  SetRegView 64
  ReadRegStr $WinOneRegisteredLocation HKCU "${WINONE_REGISTRY_KEY}" "InstallLocation"
  Call WinOneCheckWebView
  ${If} $WinOneRepairMode == 1
    StrCpy $WinOneRecoveryOperation "install"
    Call WinOneCheckRecoveryRecord
    IfErrors repairUnavailable
    ${If} $WinOneRegisteredLocation != ""
    ${AndIf} $WinOneRegisteredLocation != $INSTDIR
      Goto repairUnavailable
    ${EndIf}
    Goto targetAccepted
  ${EndIf}
  ; The current package repairs registered, partial and unregistered installs.
  ; Old /REPAIR and /FINISHUNINSTALL remain explicit compatibility commands;
  ; ordinary setup never requires either, an old hash, or an old installer.
  ${If} $WinOneRegisteredLocation != ""
    Goto installCurrentPackage
  ${EndIf}
  IfFileExists "${WINONE_SHORTCUT}" installCurrentPackage
  Call WinOneDirectoryIsEmpty
  IfErrors installCurrentPackage
  Goto targetAccepted
installCurrentPackage:
  Call WinOneUpdateExisting
  IfErrors updateFailed
  Goto done
targetAccepted:
  Call WinOnePrepareOnboarding
  ClearErrors
  CreateDirectory "$INSTDIR"
  IfErrors failed
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=WinOne.Maintenance.exe "${WINONE_PAYLOAD_DIR}\WinOne.Host.exe"
  StrCpy $WinOneGuardCommand '"$PLUGINSDIR\WinOne.Maintenance.exe" --guard-installation "$INSTDIR" --data-dir "${WINONE_PROFILE_DIR}"'
  Call WinOneStartGuard
  IfErrors failed
  ; Recheck after acquiring the guard; preserve anything added during startup.
  ${If} $WinOneRepairMode == 1
    Call WinOneCheckRecoveryRecord
  ${Else}
    Call WinOneDirectoryIsEmpty
  ${EndIf}
  IfErrors failed
  StrCpy $WinOneInstallStarted 1
  ${If} $WinOneRepairMode != 1
    Call WinOneMarkPending
    IfErrors failed
    StrCpy $WinOneRecoveryOperation "install"
    Call WinOneWriteRecoveryRecord
    IfErrors failed
  ${EndIf}
  Call WinOneWritePayload
  IfErrors failed
  !ifdef WINONE_TEST_BUILD
    ${If} $WinOneFailurePoint == "interrupt-after-files"
      ; Simulate abrupt installer death; no failure handler is run.
      System::Call 'kernel32::TerminateProcess(p -1, i 31)'
    ${EndIf}
    ${If} $WinOneFailurePoint == "after-files"
      StrCpy $WinOneFailureCode 21
      Goto failed
    ${EndIf}
  !endif
  Call WinOneCheckGuard
  IfErrors failed
  ClearErrors
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  IfErrors failed
  ; Registration is last: failed file installation must not appear complete.
  SetRegView 64
  StrCpy $WinOneRegistrationStarted 1
  WriteRegStr HKCU "${WINONE_REGISTRY_KEY}" "DisplayName" "WinOne"
  WriteRegStr HKCU "${WINONE_REGISTRY_KEY}" "DisplayVersion" "${WINONE_VERSION}"
  WriteRegStr HKCU "${WINONE_REGISTRY_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${WINONE_REGISTRY_KEY}" "InstallLocation" "$INSTDIR"
  IfErrors failed
  !ifdef WINONE_TEST_BUILD
    ${If} $WinOneFailurePoint == "after-registration"
      StrCpy $WinOneFailureCode 22
      Goto failed
    ${EndIf}
  !endif
  StrCpy $WinOneShortcutStarted 1
  CreateShortcut "${WINONE_SHORTCUT}" "$INSTDIR\WinOne.exe"
  IfErrors failed
  Call WinOneCheckGuard
  IfErrors failed
  Call WinOneClearPending
  IfErrors failed
  Goto done
completeUninstall:
  StrCpy $WinOneRecoveryOperation "uninstall"
  Call WinOneCheckRecoveryRecord
  IfErrors repairUnavailable
  ${If} $WinOneRegisteredLocation != ""
  ${AndIf} $WinOneRegisteredLocation != $INSTDIR
    Goto repairUnavailable
  ${EndIf}
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=WinOne.Maintenance.exe "${WINONE_PAYLOAD_DIR}\WinOne.Host.exe"
  StrCpy $WinOneGuardCommand '"$PLUGINSDIR\WinOne.Maintenance.exe" --guard-installation "$INSTDIR" --data-dir "${WINONE_PROFILE_DIR}"'
  Call WinOneStartGuard
  IfErrors uninstallRecoveryFailed
  ; The matching uninstall record is written only after resource restoration.
  ; Recheck under the guard before relying on it with missing worker binaries.
  Call WinOneCheckRecoveryRecord
  IfErrors uninstallRecoveryFailed
  !insertmacro WinOneCompleteUninstall WinOneCheckGuard WinOneClearPending uninstallRecoveryFailed
  Goto done
uninstallRecoveryFailed:
  SetErrorLevel 2
  MessageBox MB_ICONSTOP "$(Failed)" /SD IDOK
  Abort
failed:
  ${If} $WinOneRepairMode == 1
    Goto recoveryFailed
  ${EndIf}
  ${If} $WinOneInstallStarted != 1
    Goto recoveryFailed
  ${EndIf}
  ; Cleanup is limited to this attempt in a previously empty directory. Never
  ; remove anything after losing the guard, or claim success on partial cleanup.
  Call WinOneCheckGuard
  IfErrors recoveryFailed
  ${If} $WinOneShortcutStarted == 1
    ClearErrors
    Delete "${WINONE_SHORTCUT}"
    IfErrors recoveryFailed
  ${EndIf}
  Call WinOneCheckGuard
  IfErrors recoveryFailed
  ${If} $WinOneRegistrationStarted == 1
    SetRegView 64
    ClearErrors
    DeleteRegKey HKCU "${WINONE_REGISTRY_KEY}"
    IfErrors recoveryFailed
  ${EndIf}
  Call WinOneCheckGuard
  IfErrors recoveryFailed
  SetOutPath "$PLUGINSDIR"
  IfErrors recoveryFailed
  ClearErrors
  Delete "$INSTDIR\Uninstall.exe"
  IfErrors recoveryFailed
  !insertmacro WinOneCallRemoval Payload recoveryFailed
  !insertmacro WinOneCallRemoval Directories recoveryFailed
  Call WinOneClearPending
  IfErrors recoveryFailed
  ; Removing the marker may make the installation root empty.
  RMDir "$INSTDIR"
  SetErrorLevel $WinOneFailureCode
  MessageBox MB_ICONSTOP "$(RolledBack)" /SD IDOK
  Abort
recoveryFailed:
  ${If} $WinOneInstallStarted == 1
    SetErrorLevel 2
  ${Else}
    SetErrorLevel $WinOneFailureCode
  ${EndIf}
  MessageBox MB_ICONSTOP "$(Failed)" /SD IDOK
  Abort
repairUnavailable:
  SetErrorLevel 3
  MessageBox MB_ICONSTOP "$(RepairUnavailable)" /SD IDOK
  Abort
updateFailed:
  ; The transaction owns rollback and recovery. Never run first-install cleanup
  ; against an existing installation after an update failure.
  Abort
done:
  ; Reap maintenance before exposing completion to callers or the Finish page.
  Call WinOneCloseGuardHandle
  IfErrors cleanupFailed
  Call WinOneEndUpdateProgress
  Goto complete
cleanupFailed:
  Call WinOneFailUpdateProgress
  SetErrorLevel 2
  MessageBox MB_ICONSTOP "$(MaintenanceCloseFailed)" /SD IDOK
  Abort
complete:
SectionEnd

Section "Uninstall"
  !ifdef WINONE_TEST_BUILD
    ${GetParameters} $0
    ${GetOptions} $0 "/FAILPOINT=" $WinOneFailurePoint
  !endif
  InitPluginsDir
  ClearErrors
  CopyFiles /SILENT "$INSTDIR\WinOne.Host.exe" "$PLUGINSDIR\WinOne.Maintenance.exe"
  IfErrors failed
  StrCpy $WinOneGuardCommand '"$PLUGINSDIR\WinOne.Maintenance.exe" --guard-installation "$INSTDIR" --data-dir "${WINONE_PROFILE_DIR}" --prepare-uninstall'
  Call un.WinOneStartGuard
  IfErrors failed
  Call un.WinOneMarkPending
  IfErrors failed
  StrCpy $WinOneRecoveryOperation "uninstall"
  Call un.WinOneWriteRecoveryRecord
  IfErrors failed
  !insertmacro WinOneCompleteUninstall un.WinOneCheckGuard un.WinOneClearPending failed
  Goto done
failed:
  SetErrorLevel 1
  MessageBox MB_ICONSTOP "$(Failed)" /SD IDOK
  Abort
done:
  Call un.WinOneCloseGuardHandle
  IfErrors cleanupFailed
  Goto complete
cleanupFailed:
  SetErrorLevel 2
  MessageBox MB_ICONSTOP "$(MaintenanceCloseFailed)" /SD IDOK
  Abort
complete:
SectionEnd
