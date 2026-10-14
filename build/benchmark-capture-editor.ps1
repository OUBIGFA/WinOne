[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$BundleDirectory,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [ValidateRange(2,300)][int]$DurationSeconds = 30,
    [ValidateRange(2,30)][int]$WarmupSeconds = 8
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
Import-Module (Join-Path $PSScriptRoot 'resource-metrics.psm1') -Force
$workspace=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$outputRoot=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $outputRoot
$bundle=[IO.Path]::GetFullPath($BundleDirectory)
$hostExe=Join-Path $bundle 'WinOne.Host.exe'
$desktopExe=Join-Path $bundle 'WinOne.exe'
$fixture=Join-Path $workspace 'tests/capture-unit/bin/Release/net10.0-windows/WinOne.Capture.Tests.dll'
foreach($file in @($hostExe,$desktopExe,$fixture)){if(-not(Test-Path -LiteralPath $file -PathType Leaf)){throw "Missing file: $file"}}
if(Test-Path -LiteralPath $outputRoot){throw 'Choose a new output directory; previous evidence is preserved'}
if($outputRoot.StartsWith($workspace+'\',[StringComparison]::OrdinalIgnoreCase)){
    & git -C $workspace check-ignore --quiet -- (Join-Path $outputRoot 'probe.json')
    if($LASTEXITCODE -ne 0){throw 'Resource output must be Git-ignored'}
}
[IO.Directory]::CreateDirectory($outputRoot)|Out-Null
$profile=Join-Path $outputRoot 'profile'
[IO.Directory]::CreateDirectory($profile)|Out-Null
[IO.File]::WriteAllText((Join-Path $profile 'settings.json'),'{"schemaVersion":1,"theme":"light","language":"zh-CN","captureShortcut":"","pinShortcut":""}',[Text.UTF8Encoding]::new($false))
# Create synthetic pixels through the production draft path, never capture the screen.
$draftJson=& dotnet $fixture (Join-Path $profile 'modules/capture') --native-editor-fixture
if($LASTEXITCODE -ne 0){throw 'Could not create synthetic draft'}
$draft=($draftJson -join [Environment]::NewLine)|ConvertFrom-Json
[IO.File]::WriteAllText((Join-Path $outputRoot 'fixture.json'),($draft|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
$scenario=[pscustomobject]@{SyntheticPixels=$true;StartupMeasured=$false;WarmupSeconds=$WarmupSeconds;DurationSeconds=$DurationSeconds;NormalCloseMeasured=$false}
[IO.File]::WriteAllText((Join-Path $outputRoot 'scenario.json'),($scenario|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
$pipeName='\\.\pipe\WinOne-capture-resource-'+[guid]::NewGuid().ToString('N')
$arguments=@('--pipe',$pipeName,'--data-dir',('"'+$profile+'"'))
$hostProcess=$null;$desktopProcess=$null;$known=@{};$completed=$false
try {
    $hostProcess=Start-Process $hostExe -ArgumentList $arguments -WindowStyle Hidden -PassThru
    $null=$hostProcess.Handle
    $desktopProcess=Start-Process $desktopExe -ArgumentList ($arguments+@('--capture-draft',$draft.id)) -WindowStyle Hidden -PassThru
    $null=$desktopProcess.Handle
    $deadline=[datetime]::UtcNow.AddSeconds(30)
    $ready=$false
    do {
        if($desktopProcess.HasExited -or $hostProcess.HasExited){throw 'Test process exited before the editor was ready'}
        $desktopProcess.Refresh()
        if($desktopProcess.MainWindowHandle -ne [IntPtr]::Zero){
            $context=Get-WindowResourceContext -WindowHandle $desktopProcess.MainWindowHandle.ToInt64() -ProcessId $desktopProcess.Id
            # Wails may keep one native bottom pixel outside the WebView viewport.
            $ready=$context.Visible -and -not $context.Minimized -and -not $context.HasCaption -and $context.ClientWidth -eq $draft.width -and $context.ClientHeight -in @($draft.height,($draft.height+1))
        }
        if(-not $ready){Start-Sleep -Milliseconds 100}
    }while(-not $ready -and [datetime]::UtcNow -lt $deadline)
    if(-not $ready){throw 'Full-size capture surface did not become visible; do not count a fallback window as success'}
    # Empty modules may retire immediately after startup. This measures the
    # visible editor after warmup; cold-start peaks require a separate scenario.
    Start-Sleep -Seconds $WarmupSeconds
    $inventory=@(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
    foreach($rootId in @($hostProcess.Id,$desktopProcess.Id)){
        $entry=@($inventory|Where-Object ProcessId -eq $rootId)
        if($entry.Count -ne 1 -or $null -eq $entry[0].CreationDate){throw 'Test process identity unavailable'}
        $known[[string]$rootId]=[string]$entry[0].CreationDate.ToUniversalTime().Ticks
    }
    $family=Get-TrackedProcessFamily -Inventory $inventory -Known $known
    if(@($family.Members|Where-Object Name -eq 'msedgewebview2.exe').Count -eq 0){throw 'No WebView2 process observed'}
    & (Join-Path $PSScriptRoot 'measure-resources.ps1') -RootProcessIds @($hostProcess.Id,$desktopProcess.Id) -OutputPath (Join-Path $outputRoot 'editor-open.json') -Scenario 'capture-editor-synthetic-full-frame' -DurationSeconds $DurationSeconds -BinaryPaths @($hostExe,$desktopExe,(Join-Path $bundle 'capture/WinOne.Capture.dll')) -WindowHandle $desktopProcess.MainWindowHandle.ToInt64() -WindowProcessId $desktopProcess.Id
    $report=Get-Content -LiteralPath (Join-Path $outputRoot 'editor-open.json') -Raw -Encoding UTF8|ConvertFrom-Json
    foreach($sample in $report.Samples){
        if(-not $sample.Complete -or -not $sample.Window.Visible -or $sample.Window.Minimized -or $sample.Window.HasCaption -or $sample.Window.ClientWidth -ne $draft.width -or $sample.Window.ClientHeight -notin @($draft.height,($draft.height+1))){throw 'Editor state changed or a sample is incomplete; report is not a valid surface baseline'}
    }
    $inventory=@(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
    $family=Get-TrackedProcessFamily -Inventory $inventory -Known $known
    $completed=$true
} finally {
    # This is test teardown, not a measurement of normal user-initiated close.
    $inventory=@(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
    foreach($process in @($hostProcess,$desktopProcess)){
        if($null -eq $process -or $process.HasExited){continue}
        $entry=@($inventory|Where-Object ProcessId -eq $process.Id)
        if($entry.Count -eq 1 -and $null -ne $entry[0].CreationDate){$known[[string]$process.Id]=[string]$entry[0].CreationDate.ToUniversalTime().Ticks}
    }
    $family=Get-TrackedProcessFamily -Inventory $inventory -Known $known
    foreach($process in @($desktopProcess,$hostProcess)){
        if($null -eq $process){continue}
        if(-not $process.HasExited){$process.Kill();$null=$process.WaitForExit(10000)}
        $process.Dispose()
    }
    $deadline=[datetime]::UtcNow.AddSeconds(10)
    do {
        $inventory=@(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
        $family=Get-TrackedProcessFamily -Inventory $inventory -Known $known
        if($family.Members.Count -eq 0){break}
        Start-Sleep -Milliseconds 200
    }while([datetime]::UtcNow -lt $deadline)
    $cleanup=[pscustomobject]@{Mode='test-owned-root-termination';NormalCloseMeasured=$false;Remaining=@($family.Members|Select-Object ProcessId,Name)}
    [IO.File]::WriteAllText((Join-Path $outputRoot 'test-cleanup.json'),($cleanup|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
    if($family.Members.Count -gt 0){throw 'Test descendants remain; profile retained for diagnosis'}
    if($completed){
        $resolvedProfile=[IO.Path]::GetFullPath($profile)
        if(-not $resolvedProfile.StartsWith($outputRoot.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase)){throw 'Profile cleanup escaped the output directory'}
        Add-Type -AssemblyName Microsoft.VisualBasic
        [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($resolvedProfile,[Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs,[Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin)
    }
}
