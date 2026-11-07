param(
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe',
    [switch]$UseGPU
)
# Explicit system-interaction check. Only the test process's private desktop,
# generated profiles and verified child windows are touched; the user's
# desktop, Host, startup registration and installed apps are not used.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'close-check.exe')
if($LASTEXITCODE -ne 0){throw 'Close check output must be Git-ignored'}
$go=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$go=(Get-Command $go -ErrorAction Stop).Source
New-Item -ItemType Directory -Path $output -Force|Out-Null
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
$saved=@{GOROOT=$env:GOROOT;CGO_ENABLED=$env:CGO_ENABLED;GOTOOLCHAIN=$env:GOTOOLCHAIN;GOCACHE=$env:GOCACHE;WINONE_MYGO_CLOSE_CHECK_DIR=$env:WINONE_MYGO_CLOSE_CHECK_DIR;WINONE_MYGO_CLOSE_CHECK_GPU=$env:WINONE_MYGO_CLOSE_CHECK_GPU;TEMP=$env:TEMP;TMP=$env:TMP}
$desktop=$null
try{
    $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
    $env:CGO_ENABLED='0';$env:GOTOOLCHAIN='local'
    if(-not $env:GOCACHE){$env:GOCACHE=Join-Path $root 'dist/resources/cache/go'}
    $scratch=Join-Path $output 'scratch'
    New-Item -ItemType Directory -Path $scratch -Force|Out-Null
    $env:TEMP=$scratch;$env:TMP=$scratch
    $env:WINONE_MYGO_CLOSE_CHECK_DIR=$output
    $env:WINONE_MYGO_CLOSE_CHECK_GPU=if($UseGPU){'1'}else{$null}
    $test=Join-Path $output 'close-check.exe'
    Push-Location (Join-Path $root 'apps/desktop-mygo')
    try{
        & $go test -c -tags 'mygo_noinspector' -o $test ./app
        if($LASTEXITCODE -ne 0){throw 'Close lifecycle tests did not compile'}
    }finally{Pop-Location}
    $log=Join-Path $output 'tests.log'
    $runner=Join-Path $output 'run-tests.ps1'
    # Quote each complete Go flag: Windows PowerShell otherwise parses the
    # dotted -test.run= form as the unrelated -test argument.
    $pattern=if($UseGPU){'^TestWindow(CloseLifecycle|StartupInputAndGPUHandoff)$'}else{'^TestWindowCloseLifecycle$'}
    $run="& '"+$test.Replace("'","''")+"' '-test.run="+$pattern+"' '-test.v' '-test.timeout=60s' *> '"+$log.Replace("'","''")+"'`nexit `$LASTEXITCODE`n"
    [IO.File]::WriteAllText($runner,$run,[Text.UTF8Encoding]::new($false))
    $desktop=New-Object WinOnePrivateTestDesktop
    $process=$desktop.Start((Get-Command powershell.exe).Source,('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$runner+'"'),$root)
    if(-not $process.WaitForExit(90000)){throw "Close lifecycle check timed out: PID $($process.Id); see $log"}
    $process.Refresh()
    Get-Content -LiteralPath $log -Encoding Unicode
    if($process.ExitCode -ne 0){throw "Close lifecycle check failed; see $log"}
    Get-Content -LiteralPath (Join-Path $output 'close-timings.json') -Encoding UTF8
}finally{
    if($desktop){$desktop.Dispose()}
    foreach($name in $saved.Keys){[Environment]::SetEnvironmentVariable($name,$saved[$name],'Process')}
}
