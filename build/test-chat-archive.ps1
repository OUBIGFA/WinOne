param(
 [Parameter(Mandatory)][string]$BunExe,
 [Parameter(Mandatory)][string]$GoExe,
 [Parameter(Mandatory)][string]$WorkDirectory,
 [switch]$SkipInstall
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$work=[IO.Path]::GetFullPath((Join-Path $root $WorkDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $work
& git -C $root check-ignore -q --no-index -- (Join-Path $work 'probe')
if($LASTEXITCODE-ne 0){throw 'Chat archive fixtures must be ignored'}
New-Item -ItemType Directory -Path $work -Force | Out-Null
$worker=Join-Path $work 'worker'
& (Join-Path $PSScriptRoot 'build-chat.ps1') -BunExe $BunExe -GoExe $GoExe -OutputDirectory $worker -SkipInstall:$SkipInstall
$previous=@{}
foreach($key in @('WINONE_CHAT_BUN_EXE','WINONE_CHAT_WORKER','WINONE_CHAT_TEST_DIRECTORY','TEMP','TMP')){$previous[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
$fixtures=Join-Path $work 'fixtures'
New-Item -ItemType Directory -Path $fixtures -Force | Out-Null
try{
 $env:WINONE_CHAT_BUN_EXE=$BunExe;$env:WINONE_CHAT_WORKER=Join-Path $worker 'worker.js';$env:WINONE_CHAT_TEST_DIRECTORY=$fixtures;$env:TEMP=$fixtures;$env:TMP=$fixtures
 $ErrorActionPreference='Continue'
 & $GoExe -C $root test ./internal/chat -run 'ConversationProjectMove|NativeConversationProjectMove|ChatArchive' -count=1
 $ErrorActionPreference='Stop'
 if($LASTEXITCODE-ne 0){throw 'Chat project move or archive process checks failed'}
 $ErrorActionPreference='Continue'
 & $BunExe test (Join-Path $root 'modules/chat-pi/archive.test.mjs') --timeout 30000
 $ErrorActionPreference='Stop'
 if($LASTEXITCODE-ne 0){throw 'Cindy archive format or full Pi snapshot checks failed'}
}finally{foreach($key in $previous.Keys){[Environment]::SetEnvironmentVariable($key,$previous[$key],'Process')}}
