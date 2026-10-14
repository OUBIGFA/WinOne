[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$BundleDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [ValidateRange(2,100)][int]$Runs=21,
    [switch]$EnforceWarmBudget
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
Import-Module (Join-Path $PSScriptRoot 'resource-metrics.psm1') -Force
$workspace=Split-Path -Parent $PSScriptRoot
$bundle=(Resolve-Path -LiteralPath $BundleDirectory).Path
$output=[IO.Path]::GetFullPath((Join-Path $workspace $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$desktopExe=Join-Path $bundle 'WinOne.exe'
$hostExe=Join-Path $bundle 'WinOne.Host.exe'
foreach($file in @($desktopExe,$hostExe)){if(-not(Test-Path -LiteralPath $file -PathType Leaf)){throw "Missing binary: $file"}}
if($EnforceWarmBudget -and $Runs -lt 21){throw 'Budget enforcement requires at least 20 same-profile reopens'}
& git -C $workspace check-ignore -q --no-index -- (Join-Path $output 'result.json')
if($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose a new Git-ignored output directory'}
New-Item -ItemType Directory -Path $output | Out-Null
$profile=Join-Path $output 'profile'
New-Item -ItemType Directory -Path $profile | Out-Null
# No user profile, configured programs, startup entries or registered shortcuts.
# This probe measures process creation, not the separately tested retained
# window reopen. Disable retention explicitly and leave every shortcut empty.
[IO.File]::WriteAllText((Join-Path $profile 'settings.json'),'{"schemaVersion":1,"theme":"light","language":"zh-CN","captureShortcut":"","pinShortcut":"","ocrShortcut":"","translateShortcut":"","selectionShortcut":"","quickAssistantShortcut":"","openShortcut":"","startup":{"runAtLogon":false,"startMinimized":true,"keepUIRunning":false}}',[Text.UTF8Encoding]::new($false))
$moduleRecords=@{}
$catalog=Get-Content -LiteralPath (Join-Path $workspace 'internal/catalog/builtin.json') -Raw -Encoding UTF8|ConvertFrom-Json
foreach($id in $catalog.id){$moduleRecords[$id]=@{enabled=$false;pendingStop=$false}}
[IO.File]::WriteAllText((Join-Path $profile 'module-state.json'),(@{schemaVersion=4;revision=0;modules=$moduleRecords}|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
$pipe='\\.\pipe\WinOne-startup-benchmark-'+[guid]::NewGuid().ToString('N')
$savedTrace=$env:WINONE_DESKTOP_STARTUP_TRACE
$savedBrowser=$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS
$reports=[Collections.Generic.List[object]]::new()
$process=$null
$failure=$null
$result=[ordered]@{
    Scope='Visible UI; isolated profile; no debugger or resident WebView. First launch uses a fresh profile, not a verified cold OS cache. Readiness means committed Home data and controls, not physical input-to-photon latency.'
    StartedUtc=[DateTime]::UtcNow.ToString('o')
    OSVersion=[Environment]::OSVersion.VersionString
    Processor=@(Get-CimInstance Win32_Processor | ForEach-Object {$_.Name})
    TotalMemoryBytes=(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory
    DesktopSHA256=(Get-FileHash -LiteralPath $desktopExe -Algorithm SHA256).Hash
    HostSHA256=(Get-FileHash -LiteralPath $hostExe -Algorithm SHA256).Hash
    RequestedRuns=$Runs;Runs=@();Completed=$false;Failure=$null
    WarmP95Ms=$null;WarmBudgetMs=300;WarmBudgetPassed=$false
}
try{
    $env:WINONE_DESKTOP_STARTUP_TRACE='1'
    $env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=$null
    for($run=1;$run -le $Runs;$run++){
        $start=[Diagnostics.ProcessStartInfo]::new($desktopExe,'--standard-session --pipe "'+$pipe+'" --data-dir "'+$profile+'"')
        $start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardError=$true
        $timer=[Diagnostics.Stopwatch]::StartNew()
        $process=[Diagnostics.Process]::Start($start)
        $null=$process.Handle
        $trace=[Collections.Generic.List[string]]::new()
        $traceFile=Join-Path $output "trace-$run.log"
        $ready=$false;$deadline=[DateTime]::UtcNow.AddSeconds(20)
        while(-not $ready){
            $lineTask=$process.StandardError.ReadLineAsync()
            while(-not $lineTask.Wait(50)){
                if($process.HasExited){throw 'UI exited before readiness'}
                if([DateTime]::UtcNow -gt $deadline){throw 'UI readiness trace timed out'}
            }
            $line=$lineTask.Result
            if($null -eq $line){throw 'UI trace ended before readiness'}
            $trace.Add($line)
            if($line -match '^startup ([0-9.]+)ms ui-ready$'){
                $ready=$true
                $internalReady=[double]::Parse($Matches[1],[Globalization.CultureInfo]::InvariantCulture)
                $externalReady=$timer.Elapsed.TotalMilliseconds
            }
        }
        # Disk writes stay outside the startup measurement; finally also saves
        # the last partial trace if readiness or cleanup fails.
        [IO.File]::WriteAllLines($traceFile,$trace.ToArray(),[Text.UTF8Encoding]::new($false))
        $process.Refresh()
        if($process.MainWindowHandle -eq [IntPtr]::Zero){throw 'No visible settings window at readiness'}
        # Inventory work stays outside the timed startup. Record creation times
        # so a recycled PID cannot be counted as a surviving WebView process.
        $inventory=@(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
        $root=@($inventory | Where-Object ProcessId -eq $process.Id)
        if($root.Count -ne 1 -or $null -eq $root[0].CreationDate){throw 'Desktop process identity unavailable'}
        $known=@{}
        $known[[string]$process.Id]=[string]$root[0].CreationDate.ToUniversalTime().Ticks
        # Parent PIDs can refer to an exited process and later be reused by
        # this launch. The shared tracker also checks parent/child birth order.
        $family=Get-TrackedProcessFamily -Inventory $inventory -Known $known
        if($family.Errors.Count){throw ($family.Errors -join '; ')}
        $webviews=@($family.Members | Where-Object Name -eq 'msedgewebview2.exe')
        if($webviews.Count -eq 0){throw 'No WebView2 descendants were observed'}
        $identities=@($webviews | Select-Object ProcessId,ParentProcessId,@{Name='CreatedUtc';Expression={$_.CreationDate.ToUniversalTime().ToString('o')}})
        $sample=[pscustomobject]@{Run=$run;NativeReadyMs=$internalReady;ProcessStartToReadyMs=$externalReady;CloseMs=$null;DesktopProcessId=$process.Id;DesktopCreatedUtc=$root[0].CreationDate.ToUniversalTime().ToString('o');ObservedWebViews=$webviews.Count;WebViewIdentities=$identities;RemainingWebViewIdentities=@();ExitCode=$null;ObservedWebViewsExited=$false;Trace=$trace.ToArray()}
        $reports.Add($sample)
        $close=[Diagnostics.Stopwatch]::StartNew()
        if(-not $process.CloseMainWindow()){throw 'Owned settings window refused normal close'}
        if(-not $process.WaitForExit(10000)){throw 'Owned UI did not close normally'}
        # The sibling Host can inherit stderr; do not wait for stream EOF.
        $sample.ExitCode=$process.ExitCode
        if($sample.ExitCode -ne 0){throw "Owned UI exited with $($sample.ExitCode)"}
        $deadline=[DateTime]::UtcNow.AddSeconds(10)
        do{
            $current=@(Get-CimInstance Win32_Process -Property ProcessId,CreationDate)
            $remaining=@($webviews | Where-Object {$observed=$_;$current | Where-Object {$_.ProcessId -eq $observed.ProcessId -and $_.CreationDate -eq $observed.CreationDate}})
            $remainingIds=@($remaining | ForEach-Object {$_.ProcessId})
            $sample.RemainingWebViewIdentities=@($identities | Where-Object ProcessId -in $remainingIds)
            if($remaining.Count -eq 0){break}
            if([DateTime]::UtcNow -gt $deadline){throw 'WebView2 survived normal UI close'}
            Start-Sleep -Milliseconds 50
        }while($true)
        $sample.CloseMs=$close.Elapsed.TotalMilliseconds
        $sample.ObservedWebViewsExited=$true
        if($sample.CloseMs -gt 15000){throw 'UI process cleanup exceeded the 15-second budget'}
        $process.Dispose();$process=$null
        $result.Runs=$reports.ToArray()
        [IO.File]::WriteAllText((Join-Path $output 'result.json'),($result|ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false))
    }
    # Nearest-rank P95; the fresh-profile launch is reported separately.
    $warm=@($reports | Select-Object -Skip 1 | ForEach-Object {$_.ProcessStartToReadyMs} | Sort-Object)
    $result.WarmP95Ms=$warm[[int][Math]::Ceiling(0.95*$warm.Count)-1]
    $result.WarmBudgetPassed=$warm.Count -ge 20 -and $result.WarmP95Ms -le $result.WarmBudgetMs
    $result.Completed=$true
    if($EnforceWarmBudget -and -not $result.WarmBudgetPassed){throw "Warm startup P95 exceeded 300ms: $($result.WarmP95Ms)ms"}
}catch{
    $failure=$_
    $result.Failure=$_.Exception.Message
}finally{
    $env:WINONE_DESKTOP_STARTUP_TRACE=$savedTrace
    $env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=$savedBrowser
    try{
        if($process){
            if(-not $process.HasExited){[void]$process.CloseMainWindow();if(-not $process.WaitForExit(3000)){$process.Kill();$process.WaitForExit()}}
            $process.Dispose()
        }
        # Only the Host belonging to this invocation's fresh profile is a
        # fixture. The installed user's Host and administrator state are untouched.
        Get-CimInstance Win32_Process -Filter "Name = 'WinOne.Host.exe'" | Where-Object {$_.ExecutablePath -eq $hostExe -and $_.CommandLine.Contains($profile)} | ForEach-Object {
            $owned=Get-Process -Id $_.ProcessId -ErrorAction SilentlyContinue
            if($owned){try{if(-not $owned.HasExited){$owned.Kill();if(-not $owned.WaitForExit(5000)){throw 'Fixture Host did not exit'}}}finally{$owned.Dispose()}}
        }
    }catch{
        $result.Completed=$false
        $result.Failure=(@($result.Failure,$_.Exception.Message)|Where-Object {$_}) -join '; '
        if($null -eq $failure){$failure=$_}
    }finally{
        if(Get-Variable trace -ErrorAction SilentlyContinue){[IO.File]::WriteAllLines($traceFile,$trace.ToArray(),[Text.UTF8Encoding]::new($false))}
        $result.Runs=$reports.ToArray()
        $result.FinishedUtc=[DateTime]::UtcNow.ToString('o')
        [IO.File]::WriteAllText((Join-Path $output 'result.json'),($result|ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false))
    }
}
$reports | Select-Object Run,NativeReadyMs,ProcessStartToReadyMs,CloseMs,ObservedWebViews | Format-Table -AutoSize
if($null -ne $failure){throw $failure}
