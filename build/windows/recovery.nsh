!ifndef WINONE_RECOVERY_INCLUDED
!define WINONE_RECOVERY_INCLUDED
!include LogicLib.nsh
Var WinOneRecoveryOperation
Var WinOneRecoveryError

!macro WinOneRecoveryFunctions Prefix
; Record the exact payload, not only its human version: development packages
; may share a version while containing different binaries. Caller holds guard.
Function ${Prefix}WinOneWriteRecoveryRecord
  ClearErrors
  StrCpy $WinOneRecoveryError "record_write"
  FileOpen $0 "$INSTDIR\.winone-recovery.ini" w
  IfErrors failed
  FileWriteUTF16LE /BOM $0 "[Recovery]$\r$\nProduct=WinOne$\r$\nSchema=1$\r$\nVersion=${WINONE_VERSION}$\r$\nPackageHash=${WINONE_MANIFEST_SHA256}$\r$\nDirectory=$INSTDIR$\r$\nOperation=$WinOneRecoveryOperation$\r$\nComplete=1$\r$\n"
  IfErrors writeFailed
  FileClose $0
  IfErrors failed
  StrCpy $0 "$INSTDIR\.winone-recovery.ini"
  StrCpy $WinOneRecoveryError "record_open"
  System::Call 'kernel32::CreateFileW(w r0, i 0x40000000, i 7, p 0, i 3, i 0x80, p 0) p.r1'
  ${If} $1 == -1
    Goto failed
  ${EndIf}
  StrCpy $WinOneRecoveryError "record_flush_file"
  System::Call 'kernel32::FlushFileBuffers(p r1) i.r2'
  System::Call 'kernel32::CloseHandle(p r1) i.r3'
  ${If} $2 == 0
  ${OrIf} $3 == 0
    Goto failed
  ${EndIf}
  ClearErrors
  Return
writeFailed:
  FileClose $0
failed:
  SetErrors
FunctionEnd

Function ${Prefix}WinOneCheckRecoveryRecord
  ClearErrors
  IfFileExists "$INSTDIR\.winone-pending" 0 failed
  !insertmacro WinOneRecoveryCheck "Product" "WinOne"
  !insertmacro WinOneRecoveryCheck "Schema" "1"
  !insertmacro WinOneRecoveryCheck "Complete" "1"
  !insertmacro WinOneRecoveryCheck "Version" "${WINONE_VERSION}"
  !insertmacro WinOneRecoveryCheck "PackageHash" "${WINONE_MANIFEST_SHA256}"
  !insertmacro WinOneRecoveryCheck "Directory" "$INSTDIR"
  !insertmacro WinOneRecoveryCheck "Operation" "$WinOneRecoveryOperation"
  ClearErrors
  Return
failed:
  SetErrors
FunctionEnd
!macroend

!macro WinOneRecoveryCheck Key Expected
  ReadINIStr $0 "$INSTDIR\.winone-recovery.ini" "Recovery" "${Key}"
  IfErrors failed
  ${If} $0 != "${Expected}"
    Goto failed
  ${EndIf}
!macroend
!endif
