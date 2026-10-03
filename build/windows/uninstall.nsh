!ifndef WINONE_UNINSTALL_INCLUDED
!define WINONE_UNINSTALL_INCLUDED
; Shared by the installed uninstaller and the original package's recovery path.
; Resource restoration must already have completed before this macro is used.
!macro WinOneCompleteUninstall GuardFunction ClearPendingFunction FailureLabel
  !insertmacro WinOneRemoveFiles ${GuardFunction} ${FailureLabel}
  !ifdef WINONE_TEST_BUILD
    ${If} $WinOneFailurePoint == "interrupt-after-removal"
      System::Call 'kernel32::TerminateProcess(p -1, i 32)'
    ${EndIf}
  !endif
  Call ${GuardFunction}
  IfErrors ${FailureLabel}
  ClearErrors
  Delete "${WINONE_SHORTCUT}"
  IfErrors ${FailureLabel}
  SetRegView 64
  ; A prior attempt may already have removed registration. Distinguish a
  ; missing key from access/query failures instead of treating both as absent.
  StrCpy $0 "${WINONE_REGISTRY_KEY}"
  System::Call 'advapi32::RegOpenKeyExW(p 0x80000001, w r0, i 0, i 0x20119, *p .r1) i.r2'
  ${If} $2 == 0
    System::Call 'advapi32::RegCloseKey(p r1)'
    DeleteRegKey HKCU "${WINONE_REGISTRY_KEY}"
    IfErrors ${FailureLabel}
  ${ElseIf} $2 != 2
    Goto ${FailureLabel}
  ${EndIf}
  !ifdef WINONE_TEST_BUILD
    ${If} $WinOneFailurePoint == "interrupt-after-registration-removal"
      System::Call 'kernel32::TerminateProcess(p -1, i 33)'
    ${EndIf}
  !endif
  Delete "$INSTDIR\Uninstall.exe"
  IfErrors ${FailureLabel}
  !insertmacro WinOneRemoveEmptyDirectories ${GuardFunction} ${FailureLabel}
  Call ${ClearPendingFunction}
  IfErrors ${FailureLabel}
  RMDir "$INSTDIR"
!macroend
!endif
