[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$BundleDirectory,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [string]$GoExe = 'go',
    [ValidateRange(2,300)][int]$DurationSeconds = 30,
    [ValidateRange(2,30)][int]$WarmupSeconds = 8
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Import-Module (Join-Path $PSScriptRoot 'resource-metrics.psm1') -Force
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$outputRoot = [IO.Path]::GetFullPath($OutputDirectory)
$bundle = [IO.Path]::GetFullPath($BundleDirectory)
$hostExe = Join-Path $bundle 'WinOne.Host.exe'
$desktopExe = Join-Path $bundle 'WinOne.exe'
foreach ($file in @($hostExe,$desktopExe)) { if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Missing binary: $file" } }
if ($outputRoot.StartsWith($workspace+'\',[StringComparison]::OrdinalIgnoreCase)) {
    & git -C $workspace check-ignore --quiet -- (Join-Path $outputRoot 'probe.json')
    if ($LASTEXITCODE -ne 0) { throw 'Benchmark output must be Git-ignored' }
}
foreach ($name in @('desktop-open.json','desktop-closed.json','window-lifetime.json','module-health.json')) {
    if (Test-Path -LiteralPath (Join-Path $outputRoot $name)) { throw 'Choose a new output directory; reports are preserved' }
}
[IO.Directory]::CreateDirectory($outputRoot) | Out-Null
$profile = [IO.Path]::GetFullPath((Join-Path $outputRoot ('profile-'+[guid]::NewGuid().ToString('N'))))
if (-not $profile.StartsWith($outputRoot.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase) -or (Test-Path -LiteralPath $profile)) { throw 'Invalid temporary profile' }
[IO.Directory]::CreateDirectory($profile) | Out-Null
# Empty, independent settings: no user programs, logon tasks or global hotkeys.
[IO.File]::WriteAllText((Join-Path $profile 'settings.json'),'{"schemaVersion":1,"theme":"light","language":"zh-CN","captureShortcut":"","pinShortcut":""}',[Text.UTF8Encoding]::new($false))
$pipeName = '\\.\pipe\WinOne-desktop-resource-'+[guid]::NewGuid().ToString('N')
$arguments = @('--pipe',$pipeName,'--data-dir',('"'+$profile+'"'))
$hostProcess = $null
$desktopProcess = $null
$uiKnown = @{}
$completed = $false
try {
    $hostProcess = Start-Process -FilePath $hostExe -ArgumentList $arguments -WindowStyle Hidden -PassThru
    $null=$hostProcess.Handle
    # This scenario requires a visible interactive window; Hidden measures a
    # different WebView lifecycle and cannot establish open-window resource use.
    $desktopProcess = Start-Process -FilePath $desktopExe -ArgumentList ($arguments + @('--open-settings')) -WindowStyle Normal -PassThru
    $null=$desktopProcess.Handle
    $deadline = [datetime]::UtcNow.AddSeconds(20)
    do {
        if ($desktopProcess.HasExited -or $hostProcess.HasExited) { throw 'A test root exited during initialization' }
        $desktopProcess.Refresh()
        if ($desktopProcess.MainWindowHandle -ne [IntPtr]::Zero) { break }
        Start-Sleep -Milliseconds 100
    } while ([datetime]::UtcNow -lt $deadline)
    if ($desktopProcess.MainWindowHandle -eq [IntPtr]::Zero) { throw 'Settings window did not become visible' }
    Start-Sleep -Seconds $WarmupSeconds
    Push-Location $workspace
    try {
        $health = & $GoExe run ./tests/resource-metrics/probe --pipe $pipeName
        if ($LASTEXITCODE -ne 0) { throw 'Module health check failed; this is not a valid idle baseline' }
        [IO.File]::WriteAllText((Join-Path $outputRoot 'module-health.json'),($health -join [Environment]::NewLine),[Text.UTF8Encoding]::new($false))
    } finally { Pop-Location }
    $inventory = @(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
    $root = @($inventory | Where-Object ProcessId -eq $desktopProcess.Id)
    if ($root.Count -ne 1 -or $null -eq $root[0].CreationDate) { throw 'Desktop identity unavailable' }
    $uiKnown[[string]$desktopProcess.Id] = [string]$root[0].CreationDate.ToUniversalTime().Ticks
    $family = Get-TrackedProcessFamily -Inventory $inventory -Known $uiKnown
    if (@($family.Members | Where-Object Name -eq 'msedgewebview2.exe').Count -eq 0) { throw 'No WebView2 process was observed' }
    & (Join-Path $PSScriptRoot 'measure-resources.ps1') -RootProcessIds @($hostProcess.Id,$desktopProcess.Id) -OutputPath (Join-Path $outputRoot 'desktop-open.json') -Scenario 'settings-open-empty-profile' -DurationSeconds $DurationSeconds -BinaryPaths @($hostExe,$desktopExe) -WindowHandle ($desktopProcess.MainWindowHandle.ToInt64()) -WindowProcessId $desktopProcess.Id
    $openReport=Get-Content -LiteralPath (Join-Path $outputRoot 'desktop-open.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not (Test-OpenWindowResourceReport -Report $openReport -HostProcessId $hostProcess.Id -DesktopProcessId $desktopProcess.Id)) { throw 'Open-window sampling is incomplete or the expected visible process group is missing; profile retained' }
    $inventory = @(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
    $family = Get-TrackedProcessFamily -Inventory $inventory -Known $uiKnown
    # Send the normal close request only to the process this script created.
    $closeTimer = [Diagnostics.Stopwatch]::StartNew()
    if (-not $desktopProcess.CloseMainWindow()) { throw 'Settings window rejected the close request' }
    if (-not $desktopProcess.WaitForExit(10000)) { throw 'Desktop did not exit after closing its window' }
    $deadline = [datetime]::UtcNow.AddSeconds(10)
    do {
        $inventory = @(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
        $family = Get-TrackedProcessFamily -Inventory $inventory -Known $uiKnown
        if ($family.Members.Count -eq 0) { break }
        Start-Sleep -Milliseconds 100
    } while ([datetime]::UtcNow -lt $deadline)
    $closeTimer.Stop()
    $lifetime = [pscustomobject]@{DesktopExited=$desktopProcess.HasExited;ObservedUIProcessesExited=($family.Members.Count -eq 0);ObservedProcessCount=$uiKnown.Count;CloseToExitMilliseconds=$closeTimer.Elapsed.TotalMilliseconds;WithinCloseBudget=($closeTimer.Elapsed.TotalMilliseconds -le 15000);Remaining=@($family.Members | Select-Object ProcessId,Name)}
    [IO.File]::WriteAllText((Join-Path $outputRoot 'window-lifetime.json'),($lifetime|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
    if ($family.Members.Count -gt 0) { throw 'Observed UI descendants survived window close; profile retained' }
    if (-not $lifetime.WithinCloseBudget) { throw 'UI process cleanup exceeded the 15-second budget; profile retained' }
    & (Join-Path $PSScriptRoot 'measure-resources.ps1') -RootProcessIds @($hostProcess.Id) -OutputPath (Join-Path $outputRoot 'desktop-closed.json') -Scenario 'settings-closed-empty-profile' -DurationSeconds $DurationSeconds -BinaryPaths @($hostExe,$desktopExe)
    $closedReport = Get-Content -LiteralPath (Join-Path $outputRoot 'desktop-closed.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    $validClosed = Test-HostOnlyResourceReport -Report $closedReport -HostProcessId $hostProcess.Id
    $lifetime | Add-Member -NotePropertyName ClosedScenarioValid -NotePropertyValue $validClosed
    [IO.File]::WriteAllText((Join-Path $outputRoot 'window-lifetime.json'),($lifetime|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
    if (-not $validClosed) { throw 'Closed-window sampling included another process or incomplete data; this is not a valid idle baseline' }
    $completed = $true
} finally {
    foreach ($target in @($desktopProcess,$hostProcess)) {
        if ($null -eq $target) { continue }
        if (-not $target.HasExited) {
            $target.Kill()
            if (-not $target.WaitForExit(5000)) { throw 'Test process did not exit; profile retained' }
        }
        $target.Dispose()
    }
    # Failure profiles are kept for diagnosis, never deleted beneath survivors.
    if ($completed) {
        Add-Type -AssemblyName Microsoft.VisualBasic
        [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($profile,[Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs,[Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin)
    }
}
