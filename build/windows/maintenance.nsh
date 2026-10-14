!ifndef WINONE_MAINTENANCE_INCLUDED
!define WINONE_MAINTENANCE_INCLUDED
!include LogicLib.nsh
!if ${NSIS_PTR_SIZE} != 4
  !error "WinOne maintenance structures require the x86 Unicode NSIS target"
!endif

; System structures below target the bundled x86 Unicode NSIS compiler.
; The guarded application remains x64. Never close the process handle before
; the last file operation; check it before each mutation and on completion.
Var WinOneGuardCommand
Var WinOneGuardHandle
Var WinOneGuardPID
Var WinOneGuardError
Var WinOneGuardRelease
Var WinOneGuardReleased

!macro WinOneGuardFunctions Prefix
Function ${Prefix}WinOneCheckGuard
  ClearErrors
  ${If} $WinOneGuardHandle == 0
    StrCpy $WinOneGuardError "guard_missing"
    SetErrors
    Return
  ${EndIf}
  System::Call 'kernel32::WaitForSingleObject(p $WinOneGuardHandle, i 0) i.r0'
  ${If} $0 != 258
    StrCpy $WinOneGuardError "guard_exited"
    SetErrors
  ${EndIf}
FunctionEnd

Function ${Prefix}WinOneStartGuard
  ClearErrors
  StrCpy $WinOneGuardHandle 0
  StrCpy $WinOneGuardPID 0
  StrCpy $WinOneGuardRelease 0
  StrCpy $WinOneGuardReleased 0
  StrCpy $WinOneGuardError "guard_start_failed"
  System::Call 'kernel32::GetCurrentProcessId() i.r0'
  System::Call 'kernel32::GetTickCount() i.r1'
  !ifdef WINONE_TEST_BUILD
    ; Isolated fixtures must not contend with the user's real Host pipe. The
    ; production guard retains its normal single-session admission boundary.
    StrCpy $WinOneGuardCommand '$WinOneGuardCommand --pipe "\\.\pipe\WinOne-installer-test-$0-$1"'
  !endif
  StrCpy $2 'Local\WinOne-Installer-$0-$1'
  StrCpy $WinOneGuardCommand '$WinOneGuardCommand --installer-pid $0 --installer-ready "$2"'
  System::Call 'kernel32::CreateEventW(p 0, i 1, i 0, w r2) p.r9 ?e'
  Pop $3
  ${If} $9 == 0
    StrCpy $WinOneGuardError "guard_event_failed:$3"
    SetErrors
    Return
  ${EndIf}
  ${If} $3 == 183
    StrCpy $WinOneGuardError "guard_event_exists"
    System::Call 'kernel32::CloseHandle(p r9)'
    SetErrors
    Return
  ${EndIf}
  StrCpy $8 "$2-release"
  System::Call 'kernel32::CreateEventW(p 0, i 1, i 0, w r8) p.r7 ?e'
  Pop $3
  ${If} $7 == 0
  ${OrIf} $3 == 183
    ${If} $7 != 0
      System::Call 'kernel32::CloseHandle(p r7)'
    ${EndIf}
    System::Call 'kernel32::CloseHandle(p r9)'
    StrCpy $WinOneGuardError "guard_release_event_failed:$3"
    SetErrors
    Return
  ${EndIf}
  StrCpy $WinOneGuardRelease $7
  ; Derive this name from readiness: older package helpers ignore the extra
  ; event and keep their owner-exit behavior, without receiving an unknown flag.
  ; Fully initialize STARTUPINFOW and PROCESS_INFORMATION, including padding.
  System::Call '*(i 68, p 0, p 0, p 0, i 0, i 0, i 0, i 0, i 0, i 0, i 0, i 0, h 0, h 0, p 0, p 0, p 0, p 0) p.r1'
  System::Call '*(p 0, p 0, i 0, i 0) p.r2'
  ; Pass the quoted command through a register, never interpolate its quotes
  ; into System's own call-description grammar.
  StrCpy $0 $WinOneGuardCommand
  System::Call 'kernel32::CreateProcessW(p 0, w r0, p 0, p 0, i 0, i 0x08000000, p 0, p 0, p r1, p r2) i.r3 ?e'
  Pop $0
  System::Free $1
  ${If} $3 == 0
    StrCpy $WinOneGuardError "guard_create_failed:$0"
    System::Free $2
    System::Call 'kernel32::CloseHandle(p r9)'
    SetErrors
    Return
  ${EndIf}
  System::Call '*$2(p .r4, p .r5, i .r6, i)'
  System::Free $2
  System::Call 'kernel32::CloseHandle(p r5)'
  StrCpy $WinOneGuardHandle $4
  StrCpy $WinOneGuardPID $6
  ; Process comes first: simultaneous exit/readiness must report failure.
  System::Call '*(p r4, p r9) p.r6'
  System::Call 'kernel32::WaitForMultipleObjects(i 2, p r6, i 0, i 70000) i.r7'
  System::Free $6
  System::Call 'kernel32::CloseHandle(p r9)'
  ${If} $7 != 1
    StrCpy $WinOneGuardError "guard_not_ready"
    SetErrors
    Return
  ${EndIf}
  Call ${Prefix}WinOneCheckGuard
FunctionEnd

Function ${Prefix}WinOneCloseGuardHandle
  ClearErrors
  ${If} $WinOneGuardHandle != 0
    ; GUI completion and final process exit are different events. A foreign
    ; DLL can block the installer's ExitProcess after its window has closed.
    ; Release and reap our guard now, while this owner is still alive.
    ${If} $WinOneGuardRelease != 0
      System::Call 'kernel32::SetEvent(p $WinOneGuardRelease) i.r0'
      ${If} $0 == 0
        StrCpy $WinOneGuardError "guard_release_failed"
        SetErrors
      ${EndIf}
    ${EndIf}
    System::Call 'kernel32::WaitForSingleObject(p $WinOneGuardHandle, i 5000) i.r0'
    ${If} $0 != 0
      StrCpy $WinOneGuardError "guard_close_timeout"
      SetErrors
    ${Else}
      StrCpy $WinOneGuardReleased 1
    ${EndIf}
    System::Call 'kernel32::CloseHandle(p $WinOneGuardHandle)'
    StrCpy $WinOneGuardHandle 0
  ${EndIf}
  ${If} $WinOneGuardRelease != 0
    System::Call 'kernel32::CloseHandle(p $WinOneGuardRelease)'
    StrCpy $WinOneGuardRelease 0
  ${EndIf}
FunctionEnd

; Only the successful Finish-page action calls this. Waiting for the helper's
; process exit acknowledges closure of both the product pipe and file leases.
Function ${Prefix}WinOneReleaseGuard
  Call ${Prefix}WinOneCheckGuard
  IfErrors failed
  System::Call 'kernel32::SetEvent(p $WinOneGuardRelease) i.r0'
  ${If} $0 == 0
    Goto failed
  ${EndIf}
  System::Call 'kernel32::WaitForSingleObject(p $WinOneGuardHandle, i 5000) i.r0'
  ${If} $0 != 0
    Goto failed
  ${EndIf}
  Call ${Prefix}WinOneCloseGuardHandle
  ClearErrors
  Return
failed:
  SetErrors
FunctionEnd
!macroend
!endif
