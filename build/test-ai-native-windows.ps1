param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [Parameter(Mandatory)][string]$AIExecutable,
 [string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe'
)
# Real MyGo windows and the real private Magpie worker, on a non-input desktop.
# No user profile, global hooks, browser registration or foreground interaction.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Force $output,(Join-Path $output 'profile'),(Join-Path $output 'scratch')|Out-Null
$go=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$ai=(Resolve-Path -LiteralPath $AIExecutable).Path
$binary=Join-Path $output 'WinOne.AINativeTests.exe'
$previous=@{GOROOT=$env:GOROOT;GOCACHE=$env:GOCACHE;TEMP=$env:TEMP;TMP=$env:TMP}
$desktop=$null;$process=$null
try {
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
 $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
 & (Join-Path $PSScriptRoot 'prepare-mygo-filedrop.ps1') -GoExe $go
 Push-Location (Join-Path $root 'apps/desktop-mygo')
 try { & $go test -c -o $binary ./pages/aiservices; if($LASTEXITCODE-ne 0){throw '原生 AI 窗口测试编译失败'} } finally {Pop-Location}
 $env:TEMP=Join-Path $output 'scratch';$env:TMP=$env:TEMP
 . (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
 $desktop=New-Object WinOnePrivateTestDesktop
 $runner=Join-Path $output 'run.ps1';$log=Join-Path $output 'tests.log'
 $run="`$env:WINONE_AI_NATIVE_WINDOW='1'`n`$env:WINONE_AI_NATIVE_PROFILE='"+(Join-Path $output 'profile').Replace("'","''")+"'`n`$env:WINONE_ROUTING_TEST_EXE='"+$ai.Replace("'","''")+"'`n& '"+$binary.Replace("'","''")+"' '-test.run=^TestRealNativeAI' '-test.v' '-test.timeout=120s' 2>&1 | Out-File '"+$log.Replace("'","''")+"' -Encoding utf8`nexit `$LASTEXITCODE`n"
 [IO.File]::WriteAllText($runner,$run,[Text.UTF8Encoding]::new($false))
 $process=$desktop.Start((Get-Command powershell.exe).Source,('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$runner+'"'),$root)
 if(-not $process.WaitForExit(150000)){throw '原生 AI 实窗验证超时，请查看保留的日志'}
 $process.Refresh()
 if($process.ExitCode-ne 0){throw "原生 AI 实窗验证失败：$log"}
 Write-Output "原生 AI 实窗验证通过：$log"
} finally {
 if($process -and -not $process.HasExited){$process.Kill()}
 if($desktop){$desktop.Dispose()}
 foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
}
