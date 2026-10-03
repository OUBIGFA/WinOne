; NSIS calls this after generating the exact uninstaller embedded in setup.
; The destination travels through the environment, never through shell syntax.
!uninstfinalize 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "${__FILEDIR__}\..\export-uninstaller.ps1" "%1"' = 0
