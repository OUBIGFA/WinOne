; NSIS cannot measure the Go transaction. Keep its real file extraction
; progress, then use native indeterminate activity until the helper and all
; maintenance handles have completed. No elapsed-time percentage is invented.
Var WinOneUpdateProgressActive
Var WinOneUpdateProgressStyle

Function WinOneBeginUpdateProgress
  Push $0
  ${If} $mui.InstFilesPage.ProgressBar != 0
  ${AndIf} $mui.InstFilesPage.ProgressBar != ""
    System::Call 'user32::GetWindowLongW(p $mui.InstFilesPage.ProgressBar, i -16) i.r0'
    StrCpy $WinOneUpdateProgressStyle $0
    IntOp $0 $0 | 8 ; PBS_MARQUEE
    System::Call 'user32::SetWindowLongW(p $mui.InstFilesPage.ProgressBar, i -16, i r0)'
    SendMessage $mui.InstFilesPage.ProgressBar 0x40A 1 0 ; PBM_SETMARQUEE
    StrCpy $WinOneUpdateProgressActive 1
  ${EndIf}
  Pop $0
FunctionEnd

Function WinOneEndUpdateProgress
  ${If} $WinOneUpdateProgressActive == 1
    SendMessage $mui.InstFilesPage.ProgressBar 0x40A 0 0
    System::Call 'user32::SetWindowLongW(p $mui.InstFilesPage.ProgressBar, i -16, i $WinOneUpdateProgressStyle)'
    StrCpy $WinOneUpdateProgressActive 0
  ${EndIf}
FunctionEnd

Function WinOneFailUpdateProgress
  Call WinOneEndUpdateProgress
  ${If} $mui.InstFilesPage.ProgressBar != 0
  ${AndIf} $mui.InstFilesPage.ProgressBar != ""
    SendMessage $mui.InstFilesPage.ProgressBar 0x402 0 0 ; PBM_SETPOS
    SendMessage $mui.InstFilesPage.ProgressBar 0x410 2 0 ; PBM_SETSTATE / PBST_ERROR
  ${EndIf}
FunctionEnd
