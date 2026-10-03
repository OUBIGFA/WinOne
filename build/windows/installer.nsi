Unicode true
!ifdef WINONE_EXPORT_UNINSTALLER
  !include "export-uninstaller.nsh"
!endif
!ifdef WINONE_TEST_BUILD
  ; Select compression before includes can embed native plug-ins.
  SetCompressor zlib
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
!ifdef WINONE_TEST_BUILD
  Var WinOneFailurePoint
!endif
!define MUI_ICON "${WINONE_ICON}"
!define MUI_UNICON "${WINONE_ICON}"
!define MUI_ABORTWARNING
!define MUI_LANGDLL_WINDOWTITLE "WinOne — 语言 / Language"
!define MUI_LANGDLL_INFO "选择语言 / Select a language"
!insertmacro MUI_PAGE_LICENSE "${WINONE_PAYLOAD_DIR}\LICENSE.txt"
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE WinOneDirectorySelected
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
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
LangString NeedEmpty ${LANG_ENGLISH} "Choose an empty folder. Updating an existing installation is not available in this build."
LangString NeedEmpty ${LANG_SIMPCHINESE} "请选择空文件夹。此测试版尚不支持覆盖升级。"
LangString Existing ${LANG_ENGLISH} "WinOne is already installed. This test build does not support upgrades."
LangString Existing ${LANG_SIMPCHINESE} "已安装 WinOne。此测试版尚不支持升级。"
LangString Failed ${LANG_ENGLISH} "The operation could not finish. Keep the installation folder for recovery."
LangString Failed ${LANG_SIMPCHINESE} "操作未完成，请保留安装目录以便恢复。"
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

!insertmacro WinOneGuardFunctions ""
!insertmacro WinOneGuardFunctions "un."
!insertmacro WinOnePendingFunctions ""
!insertmacro WinOnePendingFunctions "un."
!insertmacro WinOneRecoveryFunctions ""
!insertmacro WinOneRecoveryFunctions "un."
!include "repair-ui.nsh"

Function WinOneDirectorySelected
  Call WinOneOfferRecovery
FunctionEnd

Function .onGUIEnd
  Call WinOneCloseGuardHandle
FunctionEnd
Function un.onGUIEnd
  Call un.WinOneCloseGuardHandle
FunctionEnd

Function .onInit
  !insertmacro MUI_LANGDLL_DISPLAY
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
    Call WinOneOfferRecovery
    ${If} $WinOneRepairMode != 1
    ${AndIf} $WinOneUninstallRecovery != 1
      MessageBox MB_ICONSTOP "$(Existing)" /SD IDOK
      SetErrorLevel 1
      Abort
    ${EndIf}
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
  !endif
  ${If} $WinOneUninstallRecovery == 1
    Goto completeUninstall
  ${EndIf}
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
  IfFileExists "${WINONE_SHORTCUT}" 0 +4
    MessageBox MB_ICONSTOP "$(ShortcutExists)" /SD IDOK
    SetErrorLevel 1
    Abort
  ; A first-install transaction must never overwrite an existing tree.
  Call WinOneDirectoryIsEmpty
  ${If} ${Errors}
    MessageBox MB_ICONSTOP "$(NeedEmpty)" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
targetAccepted:
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
  !insertmacro WinOneInstallFiles WinOneCheckGuard failed
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
  !insertmacro WinOneRemoveFiles WinOneCheckGuard recoveryFailed
  !insertmacro WinOneRemoveEmptyDirectories WinOneCheckGuard recoveryFailed
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
done:
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
SectionEnd
