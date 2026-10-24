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
 $packageDirectory=Join-Path $module $component.sourceDirectory
 $package=Get-Content -LiteralPath (Join-Path $packageDirectory 'package.json') -Raw -Encoding UTF8 | ConvertFrom-Json
 if($package.name-ne $component.name -or $package.version-ne $component.version){throw 'Chat share dependency notice version differs from the installed lock'}
 foreach($notice in $component.notices){if((Get-FileHash -LiteralPath (Join-Path $packageDirectory $notice.sourceFile) -Algorithm SHA256).Hash.ToLowerInvariant()-ne $notice.sha256 -or (Get-FileHash -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Algorithm SHA256).Hash.ToLowerInvariant()-ne $notice.sha256){throw 'Chat share dependency notice content changed'}}
}
$photonDirectory=Join-Path $module 'node_modules/@silvia-odwyer/photon-node'
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
foreach($component in $codeNotices.components){foreach($notice in $component.notices){Copy-Item -LiteralPath (Join-Path $root ('third_party/'+$notice.file)) -Destination (Join-Path $output $notice.file)}}
Copy-Item -LiteralPath (Join-Path $root 'third_party/chat-codemode-notices.json') -Destination (Join-Path $output 'licenses/chat-codemode-notices.json')
Copy-Item -LiteralPath (Join-Path $photonDirectory 'photon_rs_bg.wasm') -Destination (Join-Path $output 'photon_rs_bg.wasm')
Copy-Item -LiteralPath (Join-Path $module 'node_modules/quickjs-wasi/quickjs.wasm') -Destination (Join-Path $output 'quickjs.wasm')
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'codemode-worker.mjs'),'--target=bun','--outfile',(Join-Path $output 'codemode-worker.js')) 'Code Mode VM worker bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'worker.mjs'),'--target=bun','--define','WINONE_CHAT_BUNDLE=true','--outfile',(Join-Path $output 'worker.js')) 'Pi worker bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'scheduler-worker.mjs'),'--target=bun','--outfile',(Join-Path $output 'scheduler.js')) 'Cindy scheduler bundle failed'
Invoke-ChatBuildTool $BunExe @('build',(Join-Path $module 'scheduler-command-worker.mjs'),'--target=bun','--outfile',(Join-Path $output 'script-runner.js')) 'Cindy command runner bundle failed'
Invoke-ChatBuildTool $BunExe @((Join-Path $PSScriptRoot 'verify-chat-codemode-notices.mjs'),$output) 'Code Mode bundled notices failed'
$runtime=@{schemaVersion=1;piVersion=$installed.version;repository='https://github.com/earendil-works/pi';revision='4a080252f06caf63a525d7873c5d94d3c23887e0';workerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'worker.js')).Hash.ToLowerInvariant();schedulerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'scheduler.js')).Hash.ToLowerInvariant();scriptRunnerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'script-runner.js')).Hash.ToLowerInvariant();photonVersion=$photon.version;photonSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'photon_rs_bg.wasm')).Hash.ToLowerInvariant();dependencyLockSHA256=(Get-FileHash -LiteralPath (Join-Path $module 'package-lock.json')).Hash.ToLowerInvariant();cindyRevision='a152497ac24d155dd46142b73dda4fa10ad08ad0'}
$runtime.codeModeWorkerSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'codemode-worker.js')).Hash.ToLowerInvariant()
$runtime.quickjsSHA256=(Get-FileHash -LiteralPath (Join-Path $output 'quickjs.wasm')).Hash.ToLowerInvariant()
[IO.File]::WriteAllText((Join-Path $output 'runtime.json'),($runtime|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
if($Test){
 $previous=@{};foreach($key in @('WINONE_CHAT_BUN_EXE','WINONE_CHAT_WORKER','WINONE_CHAT_TEST_DIRECTORY','WINONE_CHAT_RG_EXE','TEMP','TMP','HOME')){$previous[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
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
  # Default Skill discovery is intentionally enabled in production. Give all
  # fixture workers an isolated shared-skills home instead of importing the
  # developer's real ~/.agents catalog into persisted test conversations.
  $env:HOME=Join-Path $testDirectory 'home';New-Item -ItemType Directory -Path $env:HOME -Force | Out-Null
  $env:WINONE_CHAT_BUN_EXE=$BunExe;$env:WINONE_CHAT_WORKER=Join-Path $output 'worker.js';$env:WINONE_CHAT_TEST_DIRECTORY=$testDirectory;$env:WINONE_CHAT_RG_EXE=Join-Path $output 'rg.exe';$env:TEMP=$testDirectory;$env:TMP=$testDirectory
  Invoke-ChatBuildTool $BunExe @('test',(Join-Path $module 'session.test.mjs'),(Join-Path $module 'durable-session.test.mjs'),(Join-Path $module 'durable-storage.test.mjs'),(Join-Path $module 'permissions.test.mjs'),(Join-Path $module 'operations.test.mjs'),(Join-Path $module 'resources.test.mjs'),(Join-Path $module 'attachments.test.mjs'),(Join-Path $module 'worker-cancellation.test.mjs'),(Join-Path $module 'worker-followup.test.mjs'),(Join-Path $module 'worker-queue.test.mjs'),(Join-Path $module 'worker-transport.test.mjs'),(Join-Path $module 'worker-controls.test.mjs'),(Join-Path $module 'worker-question.test.mjs'),(Join-Path $module 'worker-codemode.test.mjs'),(Join-Path $module 'worker-history.test.mjs'),(Join-Path $module 'file-changes.test.mjs'),(Join-Path $module 'scheduler-store.test.mjs'),(Join-Path $module 'automations.test.mjs'),(Join-Path $module 'archive.test.mjs'),'--timeout','20000') 'Pi session and permission checks failed'
  Invoke-ChatBuildTool $BunExe @('test',(Join-Path $module 'worker-pause.test.mjs'),'--timeout','30000') 'Durable pause and recovery checks failed'
  Invoke-ChatBuildTool $GoExe @('-C',$root,'test','./internal/chat','-count=1') 'Bundled Pi process checks failed'
 }finally{foreach($key in $previous.Keys){[Environment]::SetEnvironmentVariable($key,$previous[$key],'Process')}}
}
