; Shared successful Finish-page actions, also exercised by owned UI tests.
Function WinOneFinishShow
  ; Finishing an interrupted uninstall must never offer to run a removed app.
  ${If} $WinOneUninstallRecovery == 1
    SendMessage $mui.FinishPage.Run ${BM_SETCHECK} ${BST_UNCHECKED} 0
    ShowWindow $mui.FinishPage.Run ${SW_HIDE}
  ${EndIf}
FunctionEnd

Function WinOneFinishRun
  ${If} $WinOneUninstallRecovery == 1
    Return
  ${EndIf}
  ${If} $WinOneUpdateMode != 1
    ${If} $WinOneGuardReleased != 1
      Call WinOneReleaseGuard
      IfErrors failed
    ${EndIf}
  ${EndIf}
  ClearErrors
  ExecShell "open" "$INSTDIR\WinOne.exe"
  IfErrors failed
  Return
failed:
  MessageBox MB_ICONEXCLAMATION "$(LaunchFailed)" /SD IDOK
FunctionEnd

