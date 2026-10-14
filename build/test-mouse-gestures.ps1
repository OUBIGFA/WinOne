param(
 [Parameter(Mandatory)][string]$WorkerExecutable,
 [Parameter(Mandatory)][string]$PlaywrightModule,
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$ChromiumExecutable,
 [string]$NodeExe='node',
 [ValidateRange(1024,65535)][int]$Port=5198
)
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'report.json')
if($LASTEXITCODE-ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose a new ignored gesture test directory'}
$worker=(Resolve-Path -LiteralPath $WorkerExecutable).Path
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $worker
$playwright=(Resolve-Path -LiteralPath $PlaywrightModule).Path
$node=(Get-Command $NodeExe -ErrorAction Stop).Source
$frontend=Join-Path $root 'apps/desktop/frontend'
$vite=Join-Path $frontend 'node_modules/vite/bin/vite.js'
if(-not(Test-Path -LiteralPath $vite)){throw 'Prepare the locked frontend dependencies before running this check'}
New-Item -ItemType Directory -Path $output | Out-Null
$keys=@('WINONE_STROKE_TEST_EXE','WINONE_PLAYWRIGHT_MODULE','WINONE_LAYOUT_OUTPUT','WINONE_LAYOUT_URL','WINONE_CHROMIUM')
$previous=@{}
foreach($key in $keys){$previous[$key]=[Environment]::GetEnvironmentVariable($key,'Process')}
$server=$null
try{
 # This runner owns the Vite process, fixture profile, worker and headless
 # browser. It never attaches to an installed WinOne or a user's browser.
 $server=Start-Process -FilePath $node -ArgumentList @(('"'+$vite+'"'),'--host','127.0.0.1','--port',([string]$Port),'--strictPort') -WorkingDirectory $frontend -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $output 'vite.log') -RedirectStandardError (Join-Path $output 'vite-error.log')
 $ready=$false
 for($attempt=0;$attempt-lt 60;$attempt++){
  if($server.HasExited){throw 'The isolated Vite server exited before it was ready'}
  try{$response=Invoke-WebRequest -Uri ('http://127.0.0.1:'+ $Port +'/tests/mouse-gestures-preview.html') -UseBasicParsing -TimeoutSec 2;if($response.StatusCode-eq 200){$ready=$true;break}}catch{Start-Sleep -Milliseconds 100}
 }
 if(-not $ready){throw 'The isolated Vite server did not become ready'}
 $env:WINONE_STROKE_TEST_EXE=$worker
 $env:WINONE_PLAYWRIGHT_MODULE=$playwright
 $env:WINONE_LAYOUT_OUTPUT=Join-Path $output 'layout'
 $env:WINONE_LAYOUT_URL='http://127.0.0.1:'+ $Port +'/tests/mouse-gestures-preview.html'
 if($ChromiumExecutable){$env:WINONE_CHROMIUM=(Resolve-Path -LiteralPath $ChromiumExecutable).Path}
 # Windows PowerShell 5 treats native stderr as an ErrorRecord. Preserve the
 # complete driver failure instead of aborting on its first diagnostic line.
 $driverPreference=$ErrorActionPreference
 try{$ErrorActionPreference='Continue';$driverOutput=@(& $node (Join-Path $frontend 'tests/mouse-gestures-check.mjs') 2>&1);$driverExit=$LASTEXITCODE}finally{$ErrorActionPreference=$driverPreference}
 $driverLines=[string[]]@($driverOutput | ForEach-Object {$_.ToString()})
 [IO.File]::WriteAllLines((Join-Path $output 'driver.log'),$driverLines,[Text.UTF8Encoding]::new($false))
 $driverLines | Write-Output
 if($driverExit-ne 0){throw ('Mouse gesture scene or browser checks failed. See '+(Join-Path $output 'driver.log'))}
}finally{
 foreach($key in $keys){[Environment]::SetEnvironmentVariable($key,$previous[$key],'Process')}
 if($server){if(-not $server.HasExited){$server.Kill();$server.WaitForExit()};$server.Dispose()}
}
