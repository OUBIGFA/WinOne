param([Parameter(Mandatory)][string]$OutputDirectory,[string]$GoExe='dist/resources/tools/go-1.27.3/bin/go.exe',[string]$LiveRoom='',[switch]$PublicRelay)
# Run the real WebView2 host on a separate desktop with repository-only data.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Force $output,(Join-Path $output 'profile'),(Join-Path $output 'scratch')|Out-Null
$go=if([IO.Path]::IsPathRooted($GoExe)){$GoExe}else{Join-Path $root $GoExe}
$binary=Join-Path $output 'WinOne.FlashChatTests.exe'
$previous=@{GOROOT=$env:GOROOT;GOCACHE=$env:GOCACHE;TEMP=$env:TEMP;TMP=$env:TMP}
$desktop=$null;$process=$null
try {
 $env:GOROOT=Split-Path -Parent (Split-Path -Parent $go)
 $env:GOCACHE=Join-Path $root 'dist/resources/cache/go'
 & (Join-Path $PSScriptRoot 'prepare-mygo-filedrop.ps1') -GoExe $go
 Push-Location (Join-Path $root 'apps/desktop-mygo')
 try { & $go test -c -o $binary ./pages/flashchat; if($LASTEXITCODE-ne 0){throw 'Flash Chat native test compilation failed'} } finally {Pop-Location}
 $env:TEMP=Join-Path $output 'scratch';$env:TMP=$env:TEMP
 . (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
 $desktop=New-Object WinOnePrivateTestDesktop
 $runner=Join-Path $output 'run.ps1';$log=Join-Path $output 'tests.log'
 $run="`$env:WINONE_FLASHCHAT_NATIVE_WINDOW='1'`n`$env:WINONE_FLASHCHAT_NATIVE_PROFILE='"+(Join-Path $output 'profile').Replace("'","''")+"'`n& '"+$binary.Replace("'","''")+"' '-test.run=^TestRealNativeFlashChat$' '-test.v' '-test.timeout=120s' 2>&1 | Out-File '"+$log.Replace("'","''")+"' -Encoding utf8`nexit `$LASTEXITCODE`n"
 if($LiveRoom){
  if($LiveRoom -notmatch '^[a-zA-Z0-9-]+$'){throw 'LiveRoom must be a random test room identifier'}
  $run="`$env:WINONE_FLASHCHAT_LIVE_ROOM='$LiveRoom'`n"+$run.Replace('TestRealNativeFlashChat','TestLiveNativeFlashChat').Replace('120s','1860s')
 }else{
  # Exercise the real WebView capture API with synthetic devices. Never open
  # the user's camera or microphone during an unattended native check.
  $run="`$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS='--use-fake-device-for-media-stream --use-fake-ui-for-media-stream --auto-select-desktop-capture-source=Entire screen'`n"+$run
  if($PublicRelay){$run="`$env:WINONE_FLASHCHAT_PUBLIC_RELAY='1'`n"+$run.Replace('TestRealNativeFlashChat','TestNativePeersThroughRelay')}
 }
 [IO.File]::WriteAllText($runner,$run,[Text.UTF8Encoding]::new($false))
 $process=$desktop.Start((Get-Command powershell.exe).Source,('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$runner+'"'),$root)
 $limit=if($LiveRoom){1890000}else{150000}
 if(-not $process.WaitForExit($limit)){throw "Flash Chat native verification timed out: $log"}
 $process.Refresh()
 if($process.ExitCode-ne 0){throw "Flash Chat native verification failed: $log"}
 Write-Output "Flash Chat native verification passed: $log"
} finally {
 if($process -and -not $process.HasExited){$process.Kill()}
 if($desktop){$desktop.Dispose()}
 foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
}
