; The local setup entry reuses the Go journal/backup/rollback coordinator.
; Only the extracted candidate uses the installer file macros directly.
Var WinOneUpdateTarget
Var WinOneStagingPayload
Var WinOneUpdateProgressArguments

Function WinOnePayloadGuard
  ${If} $WinOneStagingPayload != 1
    Call WinOneCheckGuard
    Return
  ${EndIf}
  ClearErrors
FunctionEnd

; Expand the large payload writer only once. Both modes use the same checked
; file list; staging writes only to the installer's private temporary directory.
Function WinOneWritePayload
  !insertmacro WinOneInstallFiles WinOnePayloadGuard writeFailed
  ClearErrors
  Return
writeFailed:
  SetErrors
FunctionEnd

Function WinOneUpdateExisting
  Call WinOnePrepareOnboarding
  InitPluginsDir
  StrCpy $WinOneUpdateTarget $INSTDIR
  SetOutPath "$PLUGINSDIR"
  File /oname=WinOne.Update.exe "${WINONE_UPDATE_HELPER}"
  StrCpy $INSTDIR "$PLUGINSDIR\payload"
  StrCpy $WinOneStagingPayload 1
  Call WinOneWritePayload
  IfErrors stageFailed
  StrCpy $WinOneStagingPayload 0
  ClearErrors
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  IfErrors stageFailed
  StrCpy $INSTDIR $WinOneUpdateTarget
  ClearErrors
  CreateShortcut "$PLUGINSDIR\payload\WinOne.lnk" "$INSTDIR\WinOne.exe"
  IfErrors stageFailed
  SetOutPath "$PLUGINSDIR"
  DetailPrint "$(Updating)"
  Call WinOneBeginUpdateProgress
  StrCpy $WinOneUpdateProgressArguments ""
  IfSilent progressReady
    System::Call 'kernel32::GetCurrentProcessId() i.r0'
    StrCpy $WinOneUpdateProgressArguments '--progress-window $mui.InstFilesPage.Text --progress-pid $0 --language $LANGUAGE'
progressReady:
  ; Do not show a previous attempt's report if this helper cannot start.
  Delete "${WINONE_PROFILE_DIR}\installer-error.txt"
  ClearErrors
  ; The helper writes actual phase names to this label. Do not replace them
  ; with the internal command line, or advance a percentage while it runs.
  SetDetailsPrint none
  ExecWait '"$PLUGINSDIR\WinOne.Update.exe" --installation "$INSTDIR" --profile "${WINONE_PROFILE_DIR}" --staged "$PLUGINSDIR\payload" --registry-key "${WINONE_REGISTRY_KEY}" --shortcut "${WINONE_SHORTCUT}" --report "${WINONE_PROFILE_DIR}\installer-error.txt" $WinOneUpdateProgressArguments' $0
  SetDetailsPrint both
  IfErrors failed
  ${If} $0 != 0
    Goto failed
  ${EndIf}
  ; The synchronous helper has released all leases before the Finish page.
  StrCpy $WinOneUpdateMode 1
  ClearErrors
  Return
stageFailed:
  StrCpy $WinOneStagingPayload 0
  StrCpy $INSTDIR $WinOneUpdateTarget
  StrCpy $WinOneUpdateError "$(Failed)"
  Goto showFailure
failed:
  StrCpy $WinOneUpdateError "$(Failed)"
  ClearErrors
  FileOpen $1 "${WINONE_PROFILE_DIR}\installer-error.txt" r
  IfErrors noReport
  StrCpy $WinOneUpdateError ""
readReport:
  ClearErrors
  FileReadUTF16LE $1 $2
  IfErrors reportRead
  StrCpy $WinOneUpdateError "$WinOneUpdateError$2"
  Goto readReport
reportRead:
  FileClose $1
noReport:
  ${If} $WinOneUpdateError == ""
    StrCpy $WinOneUpdateError "$(Failed)"
  ${EndIf}
showFailure:
  Call WinOneFailUpdateProgress
  SetErrorLevel 1
  MessageBox MB_ICONSTOP "$(UpdateFailedDetails)" /SD IDOK
  SetErrors
FunctionEnd
