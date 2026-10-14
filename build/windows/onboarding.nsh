; Same initializer for fresh install, repair and upgrade. The marker belongs to
; the machine, so uninstall/rollback never removes this directory or its files.
!ifndef WINONE_ONBOARDING_HELPER
  !error "Provide the fixed onboarding initializer (test builds must use an isolated compiled path)."
!endif

Function WinOnePrepareOnboarding
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=WinOne.Onboarding.exe "${WINONE_ONBOARDING_HELPER}"
  nsExec::ExecToStack /TIMEOUT=30000 '"$PLUGINSDIR\WinOne.Onboarding.exe"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    !ifdef WINONE_TEST_BUILD
      FileOpen $2 "${WINONE_PROFILE_DIR}.onboarding-failure.txt" w
      FileWrite $2 "exit=$0; detail=$1"
      FileClose $2
    !endif
    DetailPrint "$1"
    MessageBox MB_ICONSTOP "$(OnboardingFailed)$\r$\n$1" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
FunctionEnd
