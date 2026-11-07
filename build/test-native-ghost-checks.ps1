param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe',
 [string]$BunExe='dist/resources/tools/bun-1.4.2/bun.exe',
 [string]$Filter='',
 [switch]$RunSystemChecks
)
# The default is real Cindy storage/decision tests only. Hidden browser-process
# execution is explicit, with generated profiles and no real WinOne installation.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Path $output -Force|Out-Null
$go=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$bun=if([IO.Path]::IsPathRooted($BunExe)){$BunExe}else{Join-Path $root $BunExe}
$go=(Get-Command $go).Source;$bun=(Get-Command $bun).Source
$names=@('WINONE_MATH_WORKER','GOROOT','GOCACHE','WINONE_CHAT_TEST_DIRECTORY','WINONE_GHOST_SANDBOX','WINONE_GHOST_SDK','WINONE_GHOST_SNAPSHOT','WINONE_GHOST_MAIL_UI_SDK','WINONE_GHOST_LIBRARY_WORKER','WINONE_GHOST_LIBRARY_SQLITE','WINONE_WORKTREE_RECOVERY_WORKER','HOME','TEMP','TMP','NO_PROXY')
$saved=@{};foreach($name in $names){$saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
try {
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
 $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
 $env:WINONE_CHAT_TEST_DIRECTORY=Join-Path $output 'profiles'
 $env:HOME=Join-Path $output 'home';$env:TEMP=Join-Path $output 'scratch';$env:TMP=$env:TEMP
 New-Item -ItemType Directory -Path $env:HOME,$env:TEMP,$env:WINONE_CHAT_TEST_DIRECTORY -Force|Out-Null
 $env:NO_PROXY=(@($env:NO_PROXY,'localhost,127.0.0.1,::1')|Where-Object {$_}) -join ','
 & node (Join-Path $PSScriptRoot 'sync-cindy.mjs') --spec (Join-Path $root 'third_party/cindy/ghost-control.sync.json') --check
 if($LASTEXITCODE-ne 0){throw 'Ghost source verification failed'}
 & node (Join-Path $PSScriptRoot 'verify-ghost-notices.mjs')
 if($LASTEXITCODE-ne 0){throw 'Ghost network source notices failed'}
 & node (Join-Path $PSScriptRoot 'verify-onebot-notices.mjs')
 if($LASTEXITCODE-ne 0){throw 'Ghost dependency notices failed'}
 $env:WINONE_GHOST_SNAPSHOT=Join-Path $output 'ghost-snapshot.js'
 & $bun build (Join-Path $root 'modules/chat-pi/ghost/snapshot-worker.mjs') --target=bun --outfile $env:WINONE_GHOST_SNAPSHOT
 if($LASTEXITCODE-ne 0){throw 'Ghost snapshot worker build failed'}
 $env:WINONE_GHOST_SDK=Join-Path $output 'ghost-sdk.js'
 & $bun build (Join-Path $root 'modules/chat-pi/cindy/ghost/apps/desktop/src/preload/ghostPreload.ts') --target=browser --format=iife --outfile $env:WINONE_GHOST_SDK
 if($LASTEXITCODE-ne 0){throw 'Ghost guest SDK build failed'}
 $env:WINONE_GHOST_MAIL_UI_SDK=Join-Path $output 'ghost-mail-ui-sdk.js'
 & $bun build (Join-Path $root 'modules/chat-pi/ghost/mail-ui-sdk.mjs') --target=browser --format=iife --outfile $env:WINONE_GHOST_MAIL_UI_SDK
 if($LASTEXITCODE-ne 0){throw 'Original mail UI helper build failed'}
 $env:WINONE_GHOST_LIBRARY_WORKER=Join-Path $output 'libraryDbWorker.js'
 & $bun build (Join-Path $root 'modules/chat-pi/cindy/ghost/apps/desktop/src/main/cindy-brain/libraryDbWorker.ts') --target=bun --outfile $env:WINONE_GHOST_LIBRARY_WORKER
 if($LASTEXITCODE-ne 0){throw 'Ghost Library database worker build failed'}
 $env:WINONE_GHOST_LIBRARY_SQLITE=Join-Path $output 'library-sqlite.js'
 & $bun build (Join-Path $root 'modules/chat-pi/ghost/library-sqlite.mjs') --target=bun --outfile $env:WINONE_GHOST_LIBRARY_SQLITE
 if($LASTEXITCODE-ne 0){throw 'Ghost Library SQLite adapter build failed'}
 $env:WINONE_WORKTREE_RECOVERY_WORKER=Join-Path $output 'worktree-recovery-worker.js'
 & $bun build (Join-Path $root 'modules/chat-pi/cindy/worktree/main/worktree/recoveryArchiveWorker.ts') --target=bun --outfile $env:WINONE_WORKTREE_RECOVERY_WORKER
 if($LASTEXITCODE-ne 0){throw 'Original worktree recovery worker build failed'}
 if($RunSystemChecks){
  & (Join-Path $PSScriptRoot 'prepare-mygo-filedrop.ps1') -GoExe $go
  $env:WINONE_GHOST_SANDBOX=Join-Path $output 'WinOne.Plugin.exe'
  Push-Location (Join-Path $root 'apps/desktop-mygo')
  try { & $go build -tags mygo_noinspector -o $env:WINONE_GHOST_SANDBOX ./cmd/ghostworker; if($LASTEXITCODE-ne 0){throw 'Native Ghost helper build failed'} } finally { Pop-Location }
  & (Join-Path $PSScriptRoot 'verify-desktop-mygo-notices.ps1') -Executable $env:WINONE_GHOST_SANDBOX -GoExe $go
 } else { $env:WINONE_GHOST_SANDBOX=$null }
 $selection=if($Filter){@('-t',$Filter)}else{@()}
 # Windows PowerShell 5 turns redirected native stderr (including Bun's normal
 # progress output) into ErrorRecords. Only the native exit code decides success.
 $ErrorActionPreference='Continue'
 & $bun test (Join-Path $root 'modules/chat-pi/ghost/host.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/platform.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/page-shim.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/outbound.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/tasks.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/companions.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/media.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/events.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/sdk-worker.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/sdk-services.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/clipboard.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/mail-local.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/mail-backend.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/browser-local.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/browser-extension-backend.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/mcpapps.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/mcpapps-admission.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/mcpapps-system.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/handoff.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/handoff-upload.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/connections.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/app-messages.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/delegation.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/teams.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/agent-tools.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/orca-agent-tools.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/write-access.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/worktrees.test.mjs') (Join-Path $root 'modules/chat-pi/ghost/worktrees-system.test.mjs') (Join-Path $root 'modules/chat-pi/authorization.test.mjs') (Join-Path $root 'modules/chat-pi/runtime-schema.test.mjs') --timeout 60000 @selection
 $testExitCode=$LASTEXITCODE
 $ErrorActionPreference='Stop'
 if($testExitCode-ne 0){throw "Ghost control-point checks failed (exit $testExitCode)"}
} finally {foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$saved[$name],'Process')}}
