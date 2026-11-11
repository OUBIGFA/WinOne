param(
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe',
    [string]$HostGoExe='go'
)
# Explicit startup regression entry: real Host/pipe, private desktop and
# disabled modules. Never connect to the installed Host or the live profile.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'host-startup.test.exe')
if($LASTEXITCODE-ne 0){throw 'Startup test output must be Git-ignored'}
$go=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$go=(Get-Command $go -ErrorAction Stop).Source
New-Item -ItemType Directory -Path $output -Force|Out-Null
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
$saved=@{}
foreach($name in @('GOROOT','GOCACHE','TEMP','TMP','LOCALAPPDATA','APPDATA','WINONE_MYGO_HOST_STARTUP_CHECK_DIR')){
    $saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process')
}
$desktop=$null
try{
    $env:GOROOT=''
    $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
    & (Join-Path $PSScriptRoot 'build-host.ps1') -GoExe $HostGoExe -OutputDirectory $output
    $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
    foreach($name in @('scratch','local','roaming','profiles')){New-Item -ItemType Directory -Path (Join-Path $output $name) -Force|Out-Null}
    $env:TEMP=Join-Path $output 'scratch';$env:TMP=$env:TEMP
    $env:LOCALAPPDATA=Join-Path $output 'local';$env:APPDATA=Join-Path $output 'roaming'
    $env:WINONE_MYGO_HOST_STARTUP_CHECK_DIR=Join-Path $output 'profiles'
    $test=Join-Path $output 'host-startup.test.exe'
    Push-Location (Join-Path $root 'apps/desktop-mygo')
    try{
        & $go test -c -tags 'mygo_noinspector' -o $test ./app
        if($LASTEXITCODE-ne 0){throw 'Host startup regression did not compile'}
    }finally{Pop-Location}
    $log=Join-Path $output 'tests.log'
    $runner=Join-Path $output 'run-tests.ps1'
    $run="[Console]::OutputEncoding=[Text.UTF8Encoding]::new(`$false)`n& '"+$test.Replace("'","''")+"' '-test.run=^TestNativeHostStartup$' '-test.v' '-test.timeout=45s' *> '"+$log.Replace("'","''")+"'`nexit `$LASTEXITCODE`n"
    [IO.File]::WriteAllText($runner,$run,[Text.UTF8Encoding]::new($false))
    $inputBefore=[WinOnePrivateTestDesktop]::InputName()
    $desktop=New-Object WinOnePrivateTestDesktop
    $process=$desktop.Start((Get-Command powershell.exe).Source,('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$runner+'"'),$root)
    if(-not $process.WaitForExit(55000)){throw ('Host startup regression timed out: '+$log)}
    $process.Refresh()
    Get-Content -LiteralPath $log -Encoding Unicode
    if([WinOnePrivateTestDesktop]::InputName()-ne $inputBefore){throw 'Input desktop changed during startup regression'}
    if($process.ExitCode-ne 0){throw ('Host startup regression failed: '+$log)}
    Write-Output 'Real Host cold start, released reservation and reopen verified; input desktop unchanged.'
}finally{
    if($desktop){
        # Only this runner's binary may survive an interrupted test. Compare
        # its absolute executable path before ending it and releasing desktop.
        $ownedHost=Join-Path $output 'WinOne.Host.exe'
        Get-CimInstance Win32_Process -Filter "Name='WinOne.Host.exe'"|Where-Object {$_.ExecutablePath -and [string]::Equals($_.ExecutablePath,$ownedHost,[StringComparison]::OrdinalIgnoreCase)}|ForEach-Object {
            $remaining=Get-Process -Id $_.ProcessId -ErrorAction SilentlyContinue
            if($remaining -and [string]::Equals($remaining.Path,$ownedHost,[StringComparison]::OrdinalIgnoreCase)){
                Stop-Process -InputObject $remaining -Force
                if(-not $remaining.WaitForExit(5000)){throw 'Test-owned Host did not exit during cleanup'}
            }
        }
        $desktop.Dispose()
    }
    foreach($name in $saved.Keys){[Environment]::SetEnvironmentVariable($name,$saved[$name],'Process')}
}
