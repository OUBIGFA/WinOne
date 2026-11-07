param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [Parameter(Mandatory)][string]$BundleDirectory,
 [Parameter(Mandatory)][string]$ChatDirectory,
 [Parameter(Mandatory)][string]$VideoFixture,
 [ValidateSet('profile','workspace','automations','browser')][string[]]$Components=@('profile','workspace','automations','browser'),
 [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe',
 [string]$PythonExe='python.exe'
)
# Explicit real-window checks, never on the user's desktop or profile. The
# upstream automation endpoint is linked only into this test executable.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'result.json')
if($LASTEXITCODE-ne 0){throw 'Native window test output must be ignored'}
if(Test-Path -LiteralPath (Join-Path $output 'result.json')){throw 'Choose a new output to retain prior evidence'}
$app=Join-Path $output 'app'
& (Join-Path $PSScriptRoot 'build-native-e2e.ps1') -OutputDirectory $app.Substring($root.Length+1) -BundleDirectory $BundleDirectory -GoExe $GoExe
$go=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$previous=@{GOROOT=$env:GOROOT;GOCACHE=$env:GOCACHE;TEMP=$env:TEMP;TMP=$env:TMP}
$desktop=$null
try {
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go);$env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
 & (Join-Path $PSScriptRoot 'build-host.ps1') -GoExe $go -OutputDirectory $app -TestOnboardingDirectory (Join-Path $output 'onboarding')
 Copy-Item -Path (Join-Path $ChatDirectory '*') -Destination (Join-Path $app 'shared/chat') -Recurse -Force
 & (Join-Path $PSScriptRoot 'prepare-native-media.ps1') -OutputDirectory $app
 Push-Location (Join-Path $root 'apps/desktop-mygo')
 # The test entry delegates its normal mode to production app.Main. Fixture
 # calls have the same canonical exe path; the Host's exact identity check
 # remains enabled, unlike an independently named RPC helper.
 # The Go driver stays in the desktop module: root Host/module builds must
 # not acquire MyGo/automation dependencies merely by listing root ./....
 try { & $go build -o (Join-Path $app 'WinOne.exe') ./tests/native-ui-matrix; if($LASTEXITCODE-ne 0){throw 'Native matrix application build failed'} } finally {Pop-Location}
 New-Item -ItemType Directory -Force (Join-Path $output 'scratch')|Out-Null
 $env:TEMP=Join-Path $output 'scratch';$env:TMP=$env:TEMP
 . (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
 $desktop=New-Object WinOnePrivateTestDesktop
 $runner=Join-Path $output 'run.ps1';$log=Join-Path $output 'tests.log'
 $python=(Get-Command $PythonExe).Source
 $script=Join-Path $root 'tests/native-ui-matrix/run.py'
 $run="& '"+$python.Replace("'","''")+"' '"+$script.Replace("'","''")+"' --output '"+$output.Replace("'","''")+"' *> '"+$log.Replace("'","''")+"'`nexit `$LASTEXITCODE`n"
 [IO.File]::WriteAllText($runner,$run,[Text.UTF8Encoding]::new($false))
 $process=$desktop.Start((Get-Command powershell.exe).Source,('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$runner+'"'),$root)
 if(-not $process.WaitForExit(300000)){throw 'Native window matrix exceeded five minutes'}
 $process.Refresh()
 Get-Content -LiteralPath $log -Encoding Unicode
 if($process.ExitCode-ne 0){throw "Native window matrix failed: $log"}
 # After the real window exits, run production native components against the
 # still-running real Host. The canonical executable path retains the Host's
 # exact peer identity check, with no service mock or private bypass.
 Copy-Item -LiteralPath (Join-Path $app 'WinOne.exe') -Destination (Join-Path $output 'WinOne.Window.exe')
 $targets=@(
  @{name='profile';package='./pages/onebot';test='TestRealHostNativeProfileMemorySkillAndVoiceCombinations'},
  @{name='workspace';package='./chat/workspace';test='TestRealHostNativeDocumentConflictReviewPDFAndMedia'},
  @{name='automations';package='./chat/automations';test='TestRealHostNativeTemplatesAndExactProjectImport'},
  @{name='browser';package='./webui';test='TestRealNativeBrowserCommentScreenshotAndHostDraft'}
 )
 $targets=@($targets | Where-Object {$Components -contains $_.name})
 $failures=@()
 $componentResults=@{}
 foreach($target in $targets){
  $binary=Join-Path $output ($target.name+'-check.exe')
  Push-Location (Join-Path $root 'apps/desktop-mygo')
  try { & $go test -c -o $binary $target.package; if($LASTEXITCODE-ne 0){throw 'Real native combination checks did not compile'} } finally {Pop-Location}
  Copy-Item -LiteralPath $binary -Destination (Join-Path $app 'WinOne.exe') -Force
  $nativeLog=Join-Path $output ($target.name+'-tests.log')
  $nativeRunner=Join-Path $output ('run-'+$target.name+'.ps1')
  $browserCheck=if($target.name-eq 'browser'){'1'}else{''}
  $nativeRun="`$env:WINONE_NATIVE_BROWSER_CHECK='"+$browserCheck+"'`n`$env:WINONE_NATIVE_UI_REAL_PIPE=Get-Content -LiteralPath '"+(Join-Path $output 'pipe.txt').Replace("'","''")+"' -Raw`n`$env:WINONE_NATIVE_UI_REAL_PROFILE='"+(Join-Path $output 'profile').Replace("'","''")+"'`n`$env:WINONE_NATIVE_UI_REPOSITORY='"+$root.Replace("'","''")+"'`n`$env:WINONE_NATIVE_UI_VIDEO='"+(Resolve-Path -LiteralPath $VideoFixture).Path.Replace("'","''")+"'`n`$env:WINONE_NATIVE_UI_SHOTS='"+(Join-Path $output ($target.name+'-shots')).Replace("'","''")+"'`n`$env:HOME='"+(Join-Path $output 'home').Replace("'","''")+"';`$env:USERPROFILE=`$env:HOME`n& '"+(Join-Path $app 'WinOne.exe').Replace("'","''")+"' '-test.run=^"+$target.test+"$' '-test.v' '-test.timeout=120s' *> '"+$nativeLog.Replace("'","''")+"'`nexit `$LASTEXITCODE`n"
  [IO.File]::WriteAllText($nativeRunner,$nativeRun,[Text.UTF8Encoding]::new($false))
  $nativeProcess=$desktop.Start((Get-Command powershell.exe).Source,('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$nativeRunner+'"'),$root)
  if(-not $nativeProcess.WaitForExit(150000)){throw 'Real native combination checks timed out'}
  $nativeProcess.Refresh();Get-Content -LiteralPath $nativeLog -Encoding Unicode
  $componentResults[$target.name]=@{exitCode=$nativeProcess.ExitCode;log=$nativeLog}
  if($nativeProcess.ExitCode-ne 0){$failures+=$nativeLog}
 }
 $matrix=@{windowExitCode=$process.ExitCode;components=$componentResults;unselected=@(@('profile','workspace','automations','browser')|Where-Object {$_ -notin $Components});exitCode=0}
 if($failures.Count-gt 0){$matrix.exitCode=1}
 [IO.File]::WriteAllText((Join-Path $output 'matrix.json'),($matrix|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
 if($failures.Count-gt 0){throw ('Real native combination checks failed: '+($failures-join ', '))}
 Get-Content -LiteralPath (Join-Path $output 'result.json') -Encoding utf8
} finally {
 # Normal tray exit is limited to the exact test-built Host on this desktop.
 if($desktop){
  $owned=@(Get-CimInstance Win32_Process|Where-Object {$_.ExecutablePath -and $_.ExecutablePath.StartsWith($app+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)})
  foreach($p in $owned|Where-Object Name -eq 'WinOne.Host.exe'){try{$desktop.RequestHostExit([uint32]$p.ProcessId)}catch{Write-Warning $_}}
  Start-Sleep -Milliseconds 500
  foreach($p in $owned){$live=Get-Process -Id $p.ProcessId -ErrorAction SilentlyContinue;if($live -and $live.Path -eq $p.ExecutablePath){Stop-Process -Id $p.ProcessId -Force}}
  $desktop.Dispose()
 }
 foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
}
