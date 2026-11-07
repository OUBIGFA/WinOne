param(
 [Parameter(Mandatory)][string]$BunExe,
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$GoExe='go',
 [string]$CargoExe='cargo',
 [switch]$SkipInstall,
 [switch]$Test
)
$ErrorActionPreference='Stop'
function Invoke-ChatBuildTool([string]$Executable,[string[]]$Arguments,[string]$Failure){
 $previous=$ErrorActionPreference
 try{$ErrorActionPreference='Continue';& $Executable @Arguments 2>&1 | ForEach-Object {$_.ToString()};$code=$LASTEXITCODE}finally{$ErrorActionPreference=$previous}
 if($code-ne 0){throw $Failure}
}
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'chat-output.probe')
if($LASTEXITCODE -ne 0){throw 'Chat output must be Git-ignored'}
$module=Join-Path $root 'modules/chat-pi'
if(-not $SkipInstall){
 $env:npm_config_cache=Join-Path $root 'dist/resources/cache/npm'
 $installPreference=$ErrorActionPreference
 $ErrorActionPreference='Continue'
 & npm.cmd ci --prefix $module --ignore-scripts --no-audit --no-fund 2>&1 | ForEach-Object {$_.ToString()}
 $installExit=$LASTEXITCODE
 $ErrorActionPreference=$installPreference
 if($installExit -ne 0){throw 'Locked Pi dependencies could not be installed'}
}
$expected=Get-Content -LiteralPath (Join-Path $module 'package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$installed=Get-Content -LiteralPath (Join-Path $module 'node_modules/@earendil-works/pi-coding-agent/package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
if($installed.version -ne $expected.dependencies.'@earendil-works/pi-coding-agent'){throw 'Pi runtime differs from the pinned dependency'}
$zipPackage=Get-Content -LiteralPath (Join-Path $module 'node_modules/jszip/package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
if($zipPackage.version-ne $expected.dependencies.jszip){throw 'JSZip runtime differs from the pinned dependency'}
$shareSources=Get-Content -LiteralPath (Join-Path $root 'third_party/cindy/session-share-source.json') -Raw -Encoding UTF8 | ConvertFrom-Json
foreach($source in $shareSources.files){if((Get-FileHash -LiteralPath (Join-Path $root $source.target) -Algorithm SHA256).Hash.ToLowerInvariant()-ne $source.sourceSHA256){throw 'Pinned Cindy share format or crypto source changed'}}
$shareNotices=Get-Content -LiteralPath (Join-Path $root 'third_party/chat-share-notices.json') -Raw -Encoding UTF8 | ConvertFrom-Json
foreach($component in $shareNotices.components){
 if(-not $component.notices -or @($component.notices).Count-eq 0){throw ('Chat dependency is missing original license evidence: '+$component.name+' '+$component.version)}
 $packageDirectory=Join-Path $module $component.sourceDirectory
 $package=Get-Content -LiteralPath (Join-Path $packageDirectory 'package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
 if($package.name-ne $component.name -or $package.version-ne $component.version){throw 'Chat share dependency notice version differs from the installed lock'}
 foreach($notice in $component.notices){if((Get-FileHash -LiteralPath (Join-Path $packageDirectory $notice.sourceFile) -Algorithm SHA256).Hash.ToLowerInvariant()-ne $notice.sha256 -or (Get-FileHash -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Algorithm SHA256).Hash.ToLowerInvariant()-ne $notice.sha256){throw 'Chat share dependency notice content changed'}}
}
$photonDirectory=Join-Path $module 'node_modules/@silvia-odwyer/photon-node'
& (Join-Path $PSScriptRoot 'prepare-chat-math.ps1')
$mathNotices=Get-Content -LiteralPath (Join-Path $root 'third_party/chat-math-notices.json') -Raw -Encoding UTF8 | ConvertFrom-Json
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'verify-chat-codemode-notices.mjs')) 'Code Mode source notices failed'
$codeNotices=Get-Content -LiteralPath (Join-Path $root 'third_party/chat-codemode-notices.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$photon=Get-Content -LiteralPath (Join-Path $photonDirectory 'package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$photonExpected=& node -p "JSON.parse(require('fs').readFileSync(process.argv[1],'utf8')).packages['node_modules/@silvia-odwyer/photon-node'].version" (Join-Path $module 'package-lock.json')
if($LASTEXITCODE -ne 0 -or $photon.version -ne $photonExpected){throw 'Photon runtime differs from the pinned dependency'}
New-Item -ItemType Directory -Path $output -Force | Out-Null
& (Join-Path $PSScriptRoot 'build-search.ps1') -CargoExe $CargoExe -OutputDirectory $output
Copy-Item -LiteralPath (Join-Path $root 'third_party/cindy/LICENSE') -Destination (Join-Path $output 'Cindy-LICENSE.txt')
Copy-Item -LiteralPath (Join-Path $root 'third_party/cindy/NOTICE') -Destination (Join-Path $output 'Cindy-NOTICE.txt')
Copy-Item -LiteralPath (Join-Path $module 'node_modules/jszip/LICENSE.markdown') -Destination (Join-Path $output 'JSZip-LICENSE.markdown')
New-Item -ItemType Directory -Path (Join-Path $output 'licenses') -Force | Out-Null
foreach($component in $shareNotices.components){foreach($notice in $component.notices){Copy-Item -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Destination (Join-Path $output $notice.file)}}
Copy-Item -LiteralPath (Join-Path $root 'third_party/chat-share-notices.json') -Destination (Join-Path $output 'licenses/chat-share-notices.json')
foreach($component in $mathNotices.components){foreach($notice in $component.notices){Copy-Item -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Destination (Join-Path $output $notice.file)}}
Copy-Item -LiteralPath (Join-Path $root 'third_party/chat-math-notices.json'),(Join-Path $root 'third_party/chat-math-font-evidence.json') -Destination (Join-Path $output 'licenses')
foreach($resource in $mathNotices.resources){
 $destination=Join-Path $output $resource.file
 New-Item -ItemType Directory -Force -Path (Split-Path -Parent $destination)|Out-Null
 Copy-Item -LiteralPath (Join-Path $root $resource.source) -Destination $destination
}
foreach($component in $codeNotices.components){foreach($notice in $component.notices){Copy-Item -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Destination (Join-Path $output $notice.file)}}
Copy-Item -LiteralPath (Join-Path $root 'third_party/chat-codemode-notices.json') -Destination (Join-Path $output 'licenses/chat-codemode-notices.json')
Copy-Item -LiteralPath (Join-Path $photonDirectory 'photon_rs_bg.wasm') -Destination (Join-Path $output 'photon_rs_bg.wasm')
Copy-Item -LiteralPath (Join-Path $module 'node_modules/quickjs-wasi/quickjs.wasm') -Destination (Join-Path $output 'quickjs.wasm')
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'codemode-worker.mjs'),'--target=bun','--outfile',(Join-Path $output 'codemode-worker.js')) 'Code Mode VM worker bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'worker.mjs'),'--target=bun','--define','WINONE_CHAT_BUNDLE=true','--outfile',(Join-Path $output 'worker.js')) 'Pi worker bundle failed'
# Vendored Cindy automation sources must equal the pinned sync (or its recorded digests).
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'sync-cindy.mjs'),'--spec',(Join-Path $root 'third_party/cindy/scheduler.sync.json'),'--check') 'Vendored Cindy automation sources changed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'sync-cindy.mjs'),'--spec',(Join-Path $root 'third_party/cindy/onebot.sync.json'),'--check') 'Vendored Cindy OneBot sources changed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'sync-cindy.mjs'),'--spec',(Join-Path $root 'third_party/cindy/onebot-tasks.sync.json'),'--check') 'Vendored Cindy OneBot task sources changed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'sync-cindy.mjs'),'--spec',(Join-Path $root 'third_party/cindy/onebot-worktree.sync.json'),'--check') 'Vendored Cindy OneBot worktree sources changed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'sync-cindy.mjs'),'--spec',(Join-Path $root 'third_party/cindy/onebot-voice.sync.json'),'--check') 'Vendored Cindy OneBot voice sources changed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'sync-cindy.mjs'),'--spec',(Join-Path $root 'third_party/cindy/onebot-import.sync.json'),'--check') 'Vendored Cindy OneBot import sources changed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'sync-cindy.mjs'),'--spec',(Join-Path $root 'third_party/cindy/onebot-wechat.sync.json'),'--check') 'Vendored Cindy WeChat sources changed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'sync-cindy.mjs'),'--spec',(Join-Path $root 'third_party/cindy/ghost-control.sync.json'),'--check') 'Vendored Cindy plugin check sources changed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'generate-native-ghost-locales.mjs'),'--check') 'Native plugin permission translations changed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'verify-ghost-notices.mjs')) 'Ghost network source notices failed'
$ghostNotices=Get-Content -LiteralPath (Join-Path $root 'third_party/ghost-notices.json') -Raw -Encoding UTF8 | ConvertFrom-Json
foreach($notice in $ghostNotices.notices){Copy-Item -LiteralPath (Join-Path $root $notice.path) -Destination (Join-Path $output ('licenses/'+[IO.Path]::GetFileName($notice.path)))}
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'verify-ghost-notices.mjs'),$output) 'Ghost bundled source notices failed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'verify-onebot-notices.mjs')) 'OneBot source notices failed'
$oneBotNotices=Get-Content -LiteralPath (Join-Path $root 'third_party/onebot-notices.json') -Raw -Encoding UTF8 | ConvertFrom-Json
foreach($component in $oneBotNotices.components){foreach($notice in $component.notices){
 $destination=Join-Path $output $notice.file
 New-Item -ItemType Directory -Path (Split-Path -Parent $destination) -Force | Out-Null
 Copy-Item -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Destination $destination
}}
Copy-Item -LiteralPath (Join-Path $root 'third_party/onebot-notices.json') -Destination (Join-Path $output 'licenses/onebot-notices.json')
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'onebot-runtime-assets.mjs'),'--copy',$output) 'OneBot runtime assets changed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'cindy/wechat/main/im/wechat/silkWorker.ts'),'--target=bun','--external','silk-wasm','--outfile',(Join-Path $output 'wechat-silk-worker.js')) 'WeChat SILK decoder bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'onebot/worker.mjs'),'--target=bun','--define','WINONE_CHAT_BUNDLE=true','--external','sharp','--outfile',(Join-Path $output 'onebot.js')) 'OneBot worker bundle failed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'verify-onebot-notices.mjs'),$output) 'OneBot bundled notices failed'
$mathMetafile=Join-Path (Split-Path -Parent $output) 'chat-math-metafile.json'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'math/worker.mjs'),'--target=bun',('--metafile='+$mathMetafile),'--banner=/* WinOne MathJax-NewCM bundle: see licenses/npm-chat-mathjax-newcm-4.1.3-USE.txt for original licenses, attribution, source and bundling changes. */','--outfile',(Join-Path $output 'math.js')) 'Math worker bundle failed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'verify-chat-math-notices.mjs'),$output,$mathMetafile) 'Math bundled dependencies/notices failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'ghost/mail-ui-sdk.mjs'),'--target=browser','--format=iife','--outfile',(Join-Path $output 'ghost-mail-ui-sdk.js')) 'Original native mail editor helpers bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'ghost/worker.mjs'),'--target=bun','--outfile',(Join-Path $output 'ghost.js')) 'Cindy plugin host bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'ghost/node-worker.mjs'),'--target=bun','--outfile',(Join-Path $output 'nodeRuntimeWorkerProcess.js')) 'Plugin Node worker bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'cindy/ghost/apps/desktop/src/main/cindy-brain/libraryDbWorker.ts'),'--target=bun','--outfile',(Join-Path $output 'libraryDbWorker.js')) 'Plugin Library database worker bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'ghost/library-sqlite.mjs'),'--target=bun','--outfile',(Join-Path $output 'library-sqlite.js')) 'Plugin Library SQLite adapter bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'cindy/worktree/main/worktree/recoveryArchiveWorker.ts'),'--target=bun','--outfile',(Join-Path $output 'worktree-recovery-worker.js')) 'Original worktree recovery worker bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'ghost/snapshot-worker.mjs'),'--target=bun','--outfile',(Join-Path $output 'ghost-snapshot.js')) 'Cindy plugin snapshot worker bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'cindy/ghost/apps/desktop/src/preload/ghostPreload.ts'),'--target=browser','--format=iife','--outfile',(Join-Path $output 'ghost-sdk.js')) 'Cindy plugin SDK bundle failed'
Copy-Item -LiteralPath (Join-Path $module 'cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/mailComposeRuntime.js') -Destination (Join-Path $output 'ghost-mail-sdk.js')
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'scheduler-worker.mjs'),'--target=bun','--outfile',(Join-Path $output 'scheduler.js')) 'Cindy scheduler bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'scheduler-command-worker.mjs'),'--target=bun','--outfile',(Join-Path $output 'script-runner.js')) 'Cindy command runner bundle failed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'verify-chat-codemode-notices.mjs'),$output) 'Code Mode bundled notices failed'
$runtime=@{schemaVersion=1;piVersion=$installed.version;repository='https://github.com/earendil-works/pi';revision='f9a26365568127461c47a07a37dcb42c140a3ac5';workerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'worker.js')).Hash.ToLowerInvariant();schedulerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'scheduler.js')).Hash.ToLowerInvariant();scriptRunnerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'script-runner.js')).Hash.ToLowerInvariant();photonVersion=$photon.version;photonSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'photon_rs_bg.wasm')).Hash.ToLowerInvariant();dependencyLockSHA256=(Get-FileHash -LiteralPath (Join-Path $module 'package-lock.json')).Hash.ToLowerInvariant();cindyRevision=(Get-Content -LiteralPath (Join-Path $root 'third_party/cindy/scheduler.sync.json') -Raw -Encoding UTF8 | ConvertFrom-Json).revision}
$runtime.onebotSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'onebot.js')).Hash.ToLowerInvariant()
$runtime.codeModeWorkerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'codemode-worker.js')).Hash.ToLowerInvariant()
$runtime.quickjsSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'quickjs.wasm')).Hash.ToLowerInvariant()
$runtime.ghostHostSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'ghost.js')).Hash.ToLowerInvariant()
$runtime.ghostSnapshotSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'ghost-snapshot.js')).Hash.ToLowerInvariant()
$runtime.ghostNodeWorkerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'nodeRuntimeWorkerProcess.js')).Hash.ToLowerInvariant()
$runtime.worktreeRecoveryWorkerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'worktree-recovery-worker.js')).Hash.ToLowerInvariant()
$runtime.ghostLibraryWorkerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'libraryDbWorker.js')).Hash.ToLowerInvariant()
$runtime.ghostLibrarySqliteSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'library-sqlite.js')).Hash.ToLowerInvariant()
$runtime.mathSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'math.js')).Hash.ToLowerInvariant()
$runtime.ghostSDKSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'ghost-sdk.js')).Hash.ToLowerInvariant()
$runtime.ghostMailSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'ghost-mail-sdk.js')).Hash.ToLowerInvariant()
$runtime.ghostMailUiSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'ghost-mail-ui-sdk.js')).Hash.ToLowerInvariant()
[IO.File]::WriteAllText((Join-Path $output 'runtime.json'),($runtime|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'verify-native-chat-runtime.mjs'),$output) 'Native Chat protocol helper verification failed'
if($Test){
 $previous=@{};foreach($key in @('GOROOT','WINONE_CHAT_BUN_EXE','WINONE_CHAT_WORKER','WINONE_CHAT_TEST_DIRECTORY','WINONE_CHAT_RG_EXE','WINONE_GHOST_MAIL_UI_SDK','WINONE_GHOST_LIBRARY_WORKER','WINONE_GHOST_LIBRARY_SQLITE','WINONE_WORKTREE_RECOVERY_WORKER','WINONE_ONEBOT_TEST_OUTPUT','WINONE_ONEBOT_WORKER','TEMP','TMP','HOME','NO_PROXY')){$previous[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
 # Keep Pi's session fixtures outside the bundle tree. The package verifier
 # treats every file below the bundle as a shipped runtime resource.
 $temporaryRoot=[IO.Path]::GetFullPath((Join-Path $root '_temp'))+[IO.Path]::DirectorySeparatorChar
 if(-not $output.StartsWith($temporaryRoot,[StringComparison]::OrdinalIgnoreCase)){throw 'Chat test output must stay in the workspace temporary tree'}
 $taskRoot=Join-Path $temporaryRoot ($output.Substring($temporaryRoot.Length).Split([IO.Path]::DirectorySeparatorChar)[0])
 $testBase=if([string]::Equals($taskRoot,$output,[StringComparison]::OrdinalIgnoreCase)){$temporaryRoot}else{$taskRoot}
 $testDirectory=Join-Path $testBase ('chat-tests-'+[guid]::NewGuid().ToString('N'))
 & (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $testDirectory
 New-Item -ItemType Directory -Path $testDirectory -Force | Out-Null
 try{
  # Honor the supplied compiler, even when use-local-resources selected the
  # other supported Go toolchain for a preceding Host/UI build.
  $testGo=(Get-Command $GoExe).Source
  $env:GOROOT=Split-Path -Parent (Split-Path -Parent $testGo)
  # Default Skill discovery is intentionally enabled in production. Give all
  # fixture workers an isolated shared-skills home instead of importing the
  # developer's real ~/.agents catalog into persisted test conversations.
  $env:HOME=Join-Path $testDirectory 'home';New-Item -ItemType Directory -Path $env:HOME -Force | Out-Null
  # Bun tests start workers without StartRunner. Apply its loopback proxy
  # bypass so fixture servers on 127.0.0.1 are reached directly, as in production.
  $env:NO_PROXY=(@($env:NO_PROXY,'localhost,127.0.0.1,::1')|Where-Object {$_}) -join ','
  $env:WINONE_CHAT_BUN_EXE=$BunExe;$env:WINONE_CHAT_WORKER=Join-Path $output 'worker.js';$env:WINONE_CHAT_TEST_DIRECTORY=$testDirectory;$env:WINONE_CHAT_RG_EXE=Join-Path $output 'rg.exe';$env:TEMP=$testDirectory;$env:TMP=$testDirectory
  $env:WINONE_WORKTREE_RECOVERY_WORKER=Join-Path $output 'worktree-recovery-worker.js'
  $env:WINONE_GHOST_MAIL_UI_SDK=Join-Path $output 'ghost-mail-ui-sdk.js'
  $env:WINONE_GHOST_LIBRARY_WORKER=Join-Path $output 'libraryDbWorker.js';$env:WINONE_GHOST_LIBRARY_SQLITE=Join-Path $output 'library-sqlite.js'
  Invoke-ChatBuildTool $BunExe @('test',(Join-Path $module 'authorization.test.mjs'),(Join-Path $module 'runtime-schema.test.mjs')) 'Native input provenance and runtime schema checks failed'
  Invoke-ChatBuildTool $BunExe @('test',(Join-Path $module 'session.test.mjs'),(Join-Path $module 'durable-session.test.mjs'),(Join-Path $module 'durable-storage.test.mjs'),(Join-Path $module 'permissions.test.mjs'),(Join-Path $module 'operations.test.mjs'),(Join-Path $module 'resources.test.mjs'),(Join-Path $module 'attachments.test.mjs'),(Join-Path $module 'worker-cancellation.test.mjs'),(Join-Path $module 'worker-followup.test.mjs'),(Join-Path $module 'worker-queue.test.mjs'),(Join-Path $module 'worker-transport.test.mjs'),(Join-Path $module 'worker-controls.test.mjs'),(Join-Path $module 'worker-question.test.mjs'),(Join-Path $module 'worker-codemode.test.mjs'),(Join-Path $module 'worker-history.test.mjs'),(Join-Path $module 'file-changes.test.mjs'),(Join-Path $module 'scheduler-store.test.mjs'),(Join-Path $module 'automations.test.mjs'),(Join-Path $module 'automations-cindy.test.mjs'),(Join-Path $module 'archive.test.mjs'),'--timeout','20000') 'Pi session and permission checks failed'
  Invoke-ChatBuildTool $BunExe @('test',(Join-Path $module 'math/convert.test.mjs')) 'Math worker checks failed'
  Invoke-ChatBuildTool $BunExe @('test',(Join-Path $module 'ghost/agent-tools.test.mjs'),(Join-Path $module 'ghost/platform.test.mjs'),(Join-Path $module 'ghost/page-shim.test.mjs'),(Join-Path $module 'ghost/outbound.test.mjs'),(Join-Path $module 'ghost/tasks.test.mjs'),(Join-Path $module 'ghost/companions.test.mjs'),(Join-Path $module 'ghost/media.test.mjs'),(Join-Path $module 'ghost/events.test.mjs'),(Join-Path $module 'ghost/delegation.test.mjs'),(Join-Path $module 'ghost/teams.test.mjs'),(Join-Path $module 'ghost/orca-agent-tools.test.mjs'),(Join-Path $module 'ghost/worktrees.test.mjs'),(Join-Path $module 'ghost/worktrees-system.test.mjs'),(Join-Path $module 'ghost/write-access.test.mjs'),(Join-Path $module 'ghost/sdk-worker.test.mjs'),(Join-Path $module 'ghost/sdk-services.test.mjs'),(Join-Path $module 'ghost/clipboard.test.mjs'),(Join-Path $module 'ghost/mail-local.test.mjs'),(Join-Path $module 'ghost/mail-backend.test.mjs'),(Join-Path $module 'ghost/browser-local.test.mjs'),(Join-Path $module 'ghost/browser-extension-backend.test.mjs'),(Join-Path $module 'ghost/mcpapps.test.mjs'),(Join-Path $module 'ghost/mcpapps-admission.test.mjs'),(Join-Path $module 'ghost/handoff.test.mjs'),(Join-Path $module 'ghost/handoff-upload.test.mjs'),(Join-Path $module 'ghost/connections.test.mjs'),(Join-Path $module 'ghost/app-messages.test.mjs'),'--timeout','30000') 'Plugin platform checks failed'
  Invoke-ChatBuildTool $BunExe @('test',(Join-Path $module 'ghost/host.test.mjs'),'--timeout','30000') 'Cindy plugin storage and control-point checks failed'
  $env:WINONE_ONEBOT_TEST_OUTPUT=$testDirectory;$env:WINONE_ONEBOT_WORKER=Join-Path $output 'onebot.js'
  Invoke-ChatBuildTool $BunExe @('test',(Join-Path $module 'onebot'),'--timeout','20000') 'OneBot local runtime checks failed'
  Invoke-ChatBuildTool $BunExe @('test',(Join-Path $module 'worker-pause.test.mjs'),'--timeout','30000') 'Durable pause and recovery checks failed'
  Invoke-ChatBuildTool $GoExe @('-C',$root,'test','./internal/chat','-count=1') 'Bundled Pi process checks failed'
 }finally{foreach($key in $previous.Keys){[Environment]::SetEnvironmentVariable($key,$previous[$key],'Process')}}
}
