[CmdletBinding()]
param([Parameter(Mandatory)][string]$BundleDirectory,[Parameter(Mandatory)][string]$OutputDirectory,[string]$GoExe='go',[ValidateRange(30,172800)][int]$DurationSeconds=86400,[ValidateRange(5,60)][int]$SampleIntervalSeconds=60,[ValidateRange(15,3600)][int]$CycleIntervalSeconds=300)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory));$bundle=(Resolve-Path -LiteralPath $BundleDirectory).Path
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
git -C $root check-ignore -q --no-index -- (Join-Path $output 'probe')
if($LASTEXITCODE-ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose a new ignored stability directory'}
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
Import-Module (Join-Path $PSScriptRoot 'resource-metrics.psm1') -Force
New-Item -ItemType Directory -Path $output | Out-Null
$profile=Join-Path $output 'profile';New-Item -ItemType Directory -Path $profile|Out-Null
$catalog=Get-Content -LiteralPath (Join-Path $root 'internal/catalog/builtin.json') -Raw -Encoding UTF8|ConvertFrom-Json
$intent=@{};foreach($id in $catalog.id){$intent[$id]=@{enabled=$true;pendingStop=$false}}
$utf8=[Text.UTF8Encoding]::new($false)
[IO.File]::WriteAllText((Join-Path $profile 'module-state.json'),(@{schemaVersion=4;revision=0;modules=$intent}|ConvertTo-Json -Depth 5),$utf8)
[IO.File]::WriteAllText((Join-Path $profile 'settings.json'),'{"schemaVersion":1,"language":"zh-CN","theme":"light","captureShortcut":"","pinShortcut":"","ocrShortcut":"","translateShortcut":"","selectionShortcut":"","quickAssistantShortcut":"","openShortcut":"","startup":{"runAtLogon":false,"startMinimized":true}}',$utf8)
$probe=Join-Path $output 'stability-probe.exe'
& $GoExe -C $root build -o $probe ./tests/resource-metrics/stability-probe
if($LASTEXITCODE-ne 0){throw 'Stability probe build failed'}
 $desktop=[WinOnePrivateTestDesktop]::new();$hostProcess=$null;$failure=$null
$timer=[Diagnostics.Stopwatch]::new();$known=@{};$previous=@{}
$status=[ordered]@{schemaVersion=1;state='starting';scope='One persistent Host on a private desktop, six module API/activation/disable/enable cycles and full observed process-family resources; no physical capture, real account, user windows or clipboard.';requestedSeconds=$DurationSeconds;startedUTC=[DateTime]::UtcNow.ToString('o');elapsedSeconds=0;samples=0;cycles=0;hostPID=0;hostCreatedUTC='';runnerPID=$PID;lastSampleUTC='';hostSHA256=(Get-FileHash -LiteralPath (Join-Path $bundle 'WinOne.Host.exe')).Hash.ToLowerInvariant();failure=$null;normalExit=$false}
function SaveStatus{
 $temporary=Join-Path $output 'status.partial.json';$path=Join-Path $output 'status.json'
 [IO.File]::WriteAllText($temporary,($status|ConvertTo-Json -Depth 5),$utf8)
 if(Test-Path -LiteralPath $path){[IO.File]::Replace($temporary,$path,(Join-Path $output 'previous-status.json'),$true)}else{[IO.File]::Move($temporary,$path)}
}
try{
 $pipe='\\.\pipe\WinOne-stability-'+[guid]::NewGuid().ToString('N')
 $hostProcess=$desktop.Start((Join-Path $bundle 'WinOne.Host.exe'),('--standard-session --pipe "'+$pipe+'" --data-dir "'+$profile+'"'),$root)
 $status.hostPID=$hostProcess.Id;$status.hostCreatedUTC=$hostProcess.StartTime.ToUniversalTime().ToString('o')
 # WMI exposes creation time with less precision than Process.StartTime.
 # Seed the tracker from the same authoritative inventory used in samples,
 # while the retained Process handle pins this particular Host's identity.
 $birth=@(Get-CimInstance Win32_Process -Filter ("ProcessId="+$hostProcess.Id))
 if($birth.Count-ne 1 -or -not $birth[0].CreationDate -or [Math]::Abs(($birth[0].CreationDate.ToUniversalTime()-$hostProcess.StartTime.ToUniversalTime()).TotalMilliseconds)-gt 1){throw 'Host inventory identity unavailable'}
 $known[[string]$hostProcess.Id]=[string]$birth[0].CreationDate.ToUniversalTime().Ticks
 $status.hostCreatedUTC=$birth[0].CreationDate.ToUniversalTime().ToString('o')
 Start-Sleep -Seconds 3
 $timer.Start();$nextCycle=0.0;$nextSample=0.0
 $status.state='running';SaveStatus
 while($timer.Elapsed.TotalSeconds-lt$DurationSeconds){
  if($hostProcess.HasExited){throw 'Persistent Host exited during stability test'}
  if($timer.Elapsed.TotalSeconds-ge$nextCycle){
   $health=& $probe --pipe $pipe --cycle
   if($LASTEXITCODE-ne 0){throw 'Module stability cycle failed'}
   $parsed=$health|ConvertFrom-Json;if(-not $parsed.healthy -or -not $parsed.cycled){throw 'Module cycle did not return a verified receipt'}
   [IO.File]::AppendAllText((Join-Path $output 'cycles.jsonl'),($parsed|ConvertTo-Json -Depth 7 -Compress)+[Environment]::NewLine,$utf8)
   $status.cycles++;$nextCycle=$timer.Elapsed.TotalSeconds+$CycleIntervalSeconds
  }
  if($timer.Elapsed.TotalSeconds-ge$nextSample){
   $inventory=@(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
   $family=Get-TrackedProcessFamily -Inventory $inventory -Known $known
   $counters=@{};foreach($counter in Get-CimInstance Win32_PerfRawData_PerfProc_Process){$counters[[string]$counter.IDProcess]=$counter}
   $sample=Get-ProcessResourceRows -Family $family -Counters $counters -Previous $previous -LogicalProcessors ([Environment]::ProcessorCount)
   $record=[ordered]@{elapsedSeconds=$timer.Elapsed.TotalSeconds;capturedUTC=[DateTime]::UtcNow.ToString('o');sample=$sample}
   [IO.File]::AppendAllText((Join-Path $output 'resources.jsonl'),($record|ConvertTo-Json -Depth 7 -Compress)+[Environment]::NewLine,$utf8)
   if(-not $sample.Complete){
    # An idle worker may finish after WMI enumerates it. Preserve this failed
    # observation, then retry only after proving the unavailable process exited
    # (or its PID was reused); a live unreadable process remains a hard failure.
    $failed=($sample.Errors-join'; ');$latest=@(Get-CimInstance Win32_Process -Property ProcessId,CreationDate)
    $retryable=$true
    foreach($problem in $sample.Errors){if($problem-notmatch '^Process (\d+):'){$retryable=$false;break};$id=$Matches[1];if(@($latest|Where-Object {$_.ProcessId-eq[uint32]$id -and [string]$_.CreationDate.ToUniversalTime().Ticks-eq$known[$id]}).Count){$retryable=$false;break}}
    if(-not $retryable){throw ('Incomplete stability resource sample: '+$failed)}
    $nextSample=$timer.Elapsed.TotalSeconds+1
    Write-Output ('Observed worker exited during counters; retained partial sample and retrying: '+$failed)
    continue
   }
   if(@($sample.Processes|Where-Object {$_.ProcessId-eq$hostProcess.Id}).Count-ne 1){throw 'Persistent Host identity changed'}
   $status.samples++;$status.lastSampleUTC=$record.capturedUTC;$status.elapsedSeconds=$timer.Elapsed.TotalSeconds
   SaveStatus;$nextSample=$timer.Elapsed.TotalSeconds+$SampleIntervalSeconds
   Write-Output ('Stability alive: {0:N1}s, {1} samples, {2} cycles, Host PID {3}'-f $status.elapsedSeconds,$status.samples,$status.cycles,$status.hostPID)
  }
  $remaining=[Math]::Min($DurationSeconds-$timer.Elapsed.TotalSeconds,[Math]::Min($nextSample-$timer.Elapsed.TotalSeconds,$nextCycle-$timer.Elapsed.TotalSeconds))
  if($remaining-gt 0){Start-Sleep -Milliseconds ([int]([Math]::Min(5,$remaining)*1000))}
 }
 # Finish active module cleanup, then close its owned Host window normally.
 # The private desktop lacks Explorer's notification area. The actual tray
 # menu path is verified separately on the current desktop with owned windows.
 $closing=& $probe --pipe $pipe --disable
 if($LASTEXITCODE-ne 0){throw 'Final module disable failed'}
 [IO.File]::WriteAllText((Join-Path $output 'closing.json'),($closing-join[Environment]::NewLine),$utf8)
 $exitHelper=$desktop.Start($probe,('--pipe "'+$pipe+'" --exit-pid '+$hostProcess.Id),$root)
 try{if(-not $exitHelper.WaitForExit(10000) -or $exitHelper.ExitCode-ne 0){throw 'Private tray exit helper failed'}}finally{$exitHelper.Dispose()}
 if(-not $hostProcess.WaitForExit(15000) -or $hostProcess.ExitCode-ne 0){throw 'Persistent Host did not exit normally'}
 $status.normalExit=$true
 # A retained PID alone is not proof: compare every observed creation time.
 $current=@(Get-CimInstance Win32_Process -Property ProcessId,CreationDate)
 foreach($id in $known.Keys){if(@($current|Where-Object {$_.ProcessId-eq[uint32]$id -and [string]$_.CreationDate.ToUniversalTime().Ticks-eq$known[$id]}).Count){throw 'Observed product process survived stability shutdown'}}
 $status.elapsedSeconds=$timer.Elapsed.TotalSeconds;$status.state='passed';SaveStatus
}catch{$failure=$_;$status.elapsedSeconds=$timer.Elapsed.TotalSeconds;$status.state='failed';$status.failure=$_.Exception.Message;SaveStatus
}finally{if($hostProcess){if(-not $hostProcess.HasExited){$hostProcess.Kill();$hostProcess.WaitForExit()};$hostProcess.Dispose()};$desktop.Dispose()}
if($failure){throw $failure}
Write-Output ('Stability duration and cleanup passed: '+$DurationSeconds+' seconds. Resource trends still require review.')

