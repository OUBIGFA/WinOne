; Shared native checks keep large manifests from expanding the same branches
; for every file. $0 is the target path; $1/$2 remain scratch registers.
!macro WinOneValidationFunctions Prefix
Function ${Prefix}WinOneValidateParent
  ClearErrors
  System::Call 'kernel32::GetFileAttributesW(w r0) i.r1 ?e'
  Pop $2
  ${If} $1 == -1
    ${If} $2 == 2
    ${OrIf} $2 == 3
      ClearErrors
      Return
    ${EndIf}
  ${Else}
    IntOp $1 $1 & 0x410
    ${If} $1 == 16
      ClearErrors
      Return
    ${EndIf}
  ${EndIf}
  SetErrors
FunctionEnd

Function ${Prefix}WinOneValidateFile
  ClearErrors
  System::Call 'kernel32::GetFileAttributesW(w r0) i.r1 ?e'
  Pop $2
  ${If} $1 == -1
    ${If} $2 == 2
    ${OrIf} $2 == 3
      ClearErrors
      Return
    ${EndIf}
  ${Else}
    ; Do not overwrite a read-only file, directory or reparse point.
    IntOp $1 $1 & 0x411
    ${If} $1 == 0
      ClearErrors
      Return
    ${EndIf}
  ${EndIf}
  SetErrors
FunctionEnd

Function ${Prefix}WinOneValidateDelete
  ClearErrors
  System::Call 'kernel32::CreateFileW(w r0, i 0x10000, i 7, p 0, i 3, i 0x80, p 0) p.r1 ?e'
  Pop $2
  ${If} $1 == -1
    ${If} $2 == 2
    ${OrIf} $2 == 3
      ClearErrors
      Return
    ${EndIf}
  ${Else}
    System::Call 'kernel32::CloseHandle(p r1) i.r2'
    ${If} $2 != 0
      ClearErrors
      Return
    ${EndIf}
  ${EndIf}
  SetErrors
FunctionEnd
!macroend

!insertmacro WinOneValidationFunctions ""
!insertmacro WinOneValidationFunctions "un."

; NSIS supplies __UNINSTALL__ for uninstaller sections/functions. The same
; removal macro therefore works for installation rollback and uninstallation.
!macro WinOneValidateEntry Kind FailureLabel
  !ifdef __UNINSTALL__
    Call un.WinOneValidate${Kind}
  !else
    Call WinOneValidate${Kind}
  !endif
  IfErrors ${FailureLabel}
!macroend
