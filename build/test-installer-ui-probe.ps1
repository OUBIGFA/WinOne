param(
    [Parameter(Mandatory)][string]$BundleDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$GoExe='go'
)
# Run the installer's real --check-ui handshake on an owned private desktop.
# This never switches the input desktop, installs WinOne, starts user workers,
# requests elevation, or connects to a network service.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$bundle=(Resolve-Path -LiteralPath $BundleDirectory).Path
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath $output){throw 'Choose a fresh probe output directory'}
foreach($name in @('WinOne.exe','WinOne.Host.exe')){if(-not(Test-Path -LiteralPath (Join-Path $bundle $name))){throw ('Missing probe executable: '+$name)}}
New-Item -ItemType Directory -Path $output|Out-Null
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
$previous=@{};foreach($key in @('TEMP','TMP','WINONE_DESKTOP_PROBE_EXE','WINONE_HOST_EXE')){$previous[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
$desktop=$null
try{
    $env:TEMP=$output;$env:TMP=$output
    $env:WINONE_DESKTOP_PROBE_EXE=Join-Path $bundle 'WinOne.exe'
    $env:WINONE_HOST_EXE=Join-Path $bundle 'WinOne.Host.exe'
    $test=Join-Path $output 'installer-ui-probe.test.exe'
    Push-Location $root
    try{& $GoExe test -c -o $test ./internal/startupprobe;if($LASTEXITCODE){throw 'Startup probe test compilation failed'}}finally{Pop-Location}
    $log=Join-Path $output 'probe.log'
    $runner=Join-Path $output 'run.ps1'
    $command="[Console]::OutputEncoding=[Text.UTF8Encoding]::new(`$false)`n& '"+$test.Replace("'","''")+"' '-test.run=^TestNativeDesktopProbeUnderMaintenance$' '-test.v' '-test.timeout=60s' *> '"+$log.Replace("'","''")+"'`nexit `$LASTEXITCODE`n"
    [IO.File]::WriteAllText($runner,$command,[Text.UTF8Encoding]::new($false))
    $inputBefore=[WinOnePrivateTestDesktop]::InputName()
    $desktop=New-Object WinOnePrivateTestDesktop
    $process=$desktop.Start((Get-Command powershell.exe).Source,('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$runner+'"'),$root)
    if(-not $process.WaitForExit(60000)){throw ('Startup probe timed out; inspect '+$log)}
    $process.Refresh()
    Get-Content -LiteralPath $log -Encoding Unicode
    if([WinOnePrivateTestDesktop]::InputName()-ne$inputBefore){throw 'Input desktop changed during probe'}
    if($process.ExitCode-ne 0){throw ('Installer UI probe failed; see '+$log)}
    Write-Output 'Installer UI probe passed with maintenance marker preserved; input desktop unchanged.'
}finally{
    if($desktop){$desktop.Dispose()}
    foreach($key in $previous.Keys){[Environment]::SetEnvironmentVariable($key,$previous[$key],'Process')}
}
