param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [Parameter(Mandatory)][string]$BundleDirectory,
 [switch]$ReuseBuild,
 [string]$PythonExe='python.exe'
)
# Explicit opt-in to isolated native windows; never use the user's desktop.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'result.json')
if($LASTEXITCODE-ne 0){throw 'UI review output must be ignored'}
if(Test-Path -LiteralPath (Join-Path $output 'result.json')){throw 'Use a fresh output to retain evidence'}
$app=Join-Path $output 'app'
if(-not $ReuseBuild){
 & (Join-Path $PSScriptRoot 'build-native-e2e.ps1') -OutputDirectory $app.Substring($root.Length+1) -BundleDirectory $BundleDirectory
}
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
$desktop=New-Object WinOnePrivateTestDesktop
try {
 $runner=Join-Path $output 'run.ps1';$log=Join-Path $output 'tests.log'
 $python=(Get-Command $PythonExe).Source
 $script=Join-Path $root 'tests/ui-design/run.py'
 $run="`$env:PYTHONUTF8='1'`n& '"+$python.Replace("'","''")+"' '"+$script.Replace("'","''")+"' --output '"+$output.Replace("'","''")+"' *> '"+$log.Replace("'","''")+"'`nexit `$LASTEXITCODE`n"
 [IO.File]::WriteAllText($runner,$run,[Text.UTF8Encoding]::new($false))
 $process=$desktop.Start((Get-Command powershell.exe).Source,('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$runner+'"'),$root)
 if(-not $process.WaitForExit(300000)){throw 'UI design capture exceeded five minutes'}
 $process.Refresh()
 Get-Content -LiteralPath $log -Encoding Unicode
 if($process.ExitCode-ne 0){throw "UI design capture failed: $log"}
 Get-Content -LiteralPath (Join-Path $output 'result.json') -Encoding UTF8
} finally {
 # Only processes launched from this exact isolated bundle are owned here.
 $owned=@(Get-CimInstance Win32_Process|Where-Object {$_.ExecutablePath -and $_.ExecutablePath.StartsWith($app+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)})
 foreach($p in $owned|Where-Object Name -eq 'WinOne.Host.exe'){try{$desktop.RequestHostExit([uint32]$p.ProcessId)}catch{Write-Warning $_}}
 Start-Sleep -Milliseconds 500
 foreach($p in $owned){$live=Get-Process -Id $p.ProcessId -ErrorAction SilentlyContinue;if($live -and $live.Path -eq $p.ExecutablePath){Stop-Process -Id $p.ProcessId -Force}}
 $desktop.Dispose()
}
