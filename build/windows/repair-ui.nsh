!ifndef WINONE_REPAIR_UI_INCLUDED
!define WINONE_REPAIR_UI_INCLUDED
!include LogicLib.nsh

; Interactive consent is offered only after validating the recovery record.
; Silent callers must explicitly opt in with /REPAIR or /FINISHUNINSTALL.
Function WinOneOfferRecovery
  ${If} $WinOneRepairMode == 1
  ${OrIf} $WinOneUninstallRecovery == 1
    Return
  ${EndIf}
  IfSilent done
  StrCpy $WinOneRecoveryOperation "install"
  Call WinOneCheckRecoveryRecord
  IfErrors checkUninstall
  MessageBox MB_ICONQUESTION|MB_YESNO|MB_DEFBUTTON2 "$(RepairQuestion)" IDYES accepted
  SetErrorLevel 1
  Quit
accepted:
  StrCpy $WinOneRepairMode 1
  Goto done
checkUninstall:
  StrCpy $WinOneRecoveryOperation "uninstall"
  Call WinOneCheckRecoveryRecord
  IfErrors done
  MessageBox MB_ICONQUESTION|MB_YESNO|MB_DEFBUTTON2 "$(UninstallQuestion)" IDYES finishUninstall
  SetErrorLevel 1
  Quit
finishUninstall:
  StrCpy $WinOneUninstallRecovery 1
done:
FunctionEnd
!endif
