!ifndef WINONE_PENDING_INCLUDED
!define WINONE_PENDING_INCLUDED
!include LogicLib.nsh

!macro WinOnePendingFunctions Prefix
; Presence alone blocks runtime startup. Flush creation before changing payload
; files; an empty or partially written marker is still an incomplete operation.
Function ${Prefix}WinOneMarkPending
  ClearErrors
  StrCpy $0 "$INSTDIR\.winone-pending"
  System::Call 'kernel32::CreateFileW(w r0, i 0x40000000, i 1, p 0, i 2, i 0x80000080, p 0) p.r1'
  ${If} $1 == -1
    SetErrors
    Return
  ${EndIf}
  System::Call 'kernel32::FlushFileBuffers(p r1) i.r2'
  System::Call 'kernel32::CloseHandle(p r1) i.r3'
  ${If} $2 == 0
  ${OrIf} $3 == 0
    SetErrors
  ${EndIf}
FunctionEnd

Function ${Prefix}WinOneClearPending
  ClearErrors
  Delete "$INSTDIR\.winone-recovery.ini"
  IfErrors done
  Delete "$INSTDIR\.winone-pending"
done:
FunctionEnd
!macroend
!endif
