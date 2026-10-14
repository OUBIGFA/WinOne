; Follow the current user's Windows display language before any page/message.
; Both Chinese variants use the available Simplified Chinese translation;
; unsupported languages fall back to English. Regional number/date formats and
; a previous installer's language choice must not override the display language.
!macro WinOneLanguageFunctions Prefix
Function ${Prefix}WinOneInitializeLanguage
  Push $0
  System::Call 'kernel32::GetUserDefaultUILanguage() i.r0'
  ${If} $0 == 0
    System::Call 'kernel32::GetSystemDefaultUILanguage() i.r0'
  ${EndIf}
  Push $0
  Call ${Prefix}WinOneApplyUILanguage
  Pop $0
FunctionEnd

Function ${Prefix}WinOneApplyUILanguage
  Exch $0
  IntOp $0 $0 & 0x3ff
  StrCpy $LANGUAGE ${LANG_ENGLISH}
  ${If} $0 == 4
    StrCpy $LANGUAGE ${LANG_SIMPCHINESE}
  ${EndIf}
  Pop $0
FunctionEnd
!macroend
