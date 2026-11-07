param([string]$GoExe='go',[string]$DictionaryPath='',[switch]$NativeSettings)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$prior=$env:WINONE_TEST_LEARNING_DICTIONARY
Push-Location $root
try {
 if($NativeSettings){
  Push-Location (Join-Path $root 'apps/desktop-mygo')
  try {& $GoExe test ./pages/selection ./i18n -count=1;if($LASTEXITCODE -ne 0){throw 'Learning settings tests failed'}}finally{Pop-Location}
  return
 }
 if($DictionaryPath){$env:WINONE_TEST_LEARNING_DICTIONARY=[IO.Path]::GetFullPath($DictionaryPath)}
 & $GoExe test ./internal/learning/... ./internal/resultwindow ./internal/selection ./internal/quickassistant ./internal/profiles ./internal/distribution ./apps/host -count=1
 if($LASTEXITCODE -ne 0){throw 'Learning behavior tests failed'}
} finally {$env:WINONE_TEST_LEARNING_DICTIONARY=$prior;Pop-Location}
