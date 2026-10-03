!ifndef WINONE_DIRECTORY_INCLUDED
!define WINONE_DIRECTORY_INCLUDED
!include LogicLib.nsh

; Check native error codes instead of treating every enumeration failure as an
; empty target. The caller must still hold its installation guard before writes.
Function WinOneDirectoryIsEmpty
  StrCpy $0 $INSTDIR
  System::Call 'kernel32::GetFileAttributesW(w r0) i.r1 ?e'
  Pop $2
  ${If} $1 == -1
    ${If} $2 == 2
    ${OrIf} $2 == 3
      ClearErrors
      Return
    ${EndIf}
    SetErrors
    Return
  ${EndIf}
  IntOp $1 $1 & 16
  ${If} $1 == 0
    SetErrors
    Return
  ${EndIf}
  StrCpy $0 "$INSTDIR\*"
  System::Alloc 592
  Pop $1
  ${If} $1 == 0
    SetErrors
    Return
  ${EndIf}
  System::Call 'kernel32::FindFirstFileW(w r0, p r1) p.r2 ?e'
  Pop $3
  ${If} $2 == -1
    System::Free $1
    ${If} $3 == 2
      ClearErrors
    ${Else}
      SetErrors
    ${EndIf}
    Return
  ${EndIf}
  StrCpy $9 0
next:
  ; WIN32_FIND_DATAW: cFileName starts at byte 44 and contains 260 WCHARs.
  System::Call '*$1(&v44, &w260 .r3)'
  ${If} $3 != "."
  ${AndIf} $3 != ".."
    Goto done
  ${EndIf}
  System::Call 'kernel32::FindNextFileW(p r2, p r1) i.r4 ?e'
  Pop $5
  ${If} $4 != 0
    Goto next
  ${EndIf}
  ${If} $5 == 18
    StrCpy $9 1
  ${EndIf}
done:
  System::Call 'kernel32::FindClose(p r2)'
  System::Free $1
  ${If} $9 == 1
    ClearErrors
  ${Else}
    SetErrors
  ${EndIf}
FunctionEnd
!endif
