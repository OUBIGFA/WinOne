[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$BundleDirectory,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [string]$GoExe = 'go',
    [ValidateRange(2,300)][int]$DurationSeconds = 30,
    [ValidateRange(2,30)][int]$WarmupSeconds = 8,
    [ValidateSet('WebView2','MyGo')][string]$Frontend = 'WebView2'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Import-Module (Join-Path $PSScriptRoot 'resource-metrics.psm1') -Force
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$outputRoot = [IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $outputRoot
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
[IO.File]::WriteAllText((Join-Path $profile 'settings.json'),'{"schemaVersion":1,"theme":"light","language":"zh-CN","captureShortcut":"","pinShortcut":"","ocrShortcut":"","translateShortcut":"","selectionShortcut":"","quickAssistantShortcut":"","openShortcut":"","startup":{"runAtLogon":false,"startMinimized":true}}',[Text.UTF8Encoding]::new($false))
if($Frontend -eq 'MyGo'){
    # Native settings-only cost: no worker prewarm or configured programs.
    # Enabled-module cost is measured separately by benchmark-idle.ps1.
    $records=@{}
    $catalog=Get-Content -LiteralPath (Join-Path $workspace 'internal/catalog/builtin.json') -Raw -Encoding UTF8|ConvertFrom-Json
    foreach($id in $catalog.id){$records[$id]=@{enabled=$false;pendingStop=$false}}
    [IO.File]::WriteAllText((Join-Path $profile 'module-state.json'),(@{schemaVersion=4;revision=0;modules=$records}|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
}
$pipeName = '\\.\pipe\WinOne-desktop-resource-'+[guid]::NewGuid().ToString('N')
$arguments = @('--pipe',$pipeName,'--data-dir',('"'+$profile+'"'))
$hostProcess = $null
$desktopProcess = $null
$uiKnown = @{}
$completed = $false
$desktop = [WinOnePrivateTestDesktop]::new()
$windowHandle = [IntPtr]::Zero
try {
    $quoted='--standard-session --pipe "'+$pipeName+'" --data-dir "'+$profile+'"'
    $hostProcess = $desktop.Start($hostExe,$quoted,$workspace)
    $null=$hostProcess.Handle
    # A visible native window on the owned desktop, never a hidden window or
    # a foreground switch. Every sample verifies its HWND/PID and dimensions.
    $desktopProcess = $desktop.Start($desktopExe,($quoted+' --open-settings'),$workspace)
    $null=$desktopProcess.Handle
    $deadline = [datetime]::UtcNow.AddSeconds(20)
    do {
        if ($desktopProcess.HasExited -or $hostProcess.HasExited) { throw 'A test root exited during initialization' }
        $windowHandle=$desktop.WindowHandle([uint32]$desktopProcess.Id)
        if ($windowHandle -ne [IntPtr]::Zero) { break }
        Start-Sleep -Milliseconds 100
    } while ([datetime]::UtcNow -lt $deadline)
    if ($windowHandle -eq [IntPtr]::Zero) { throw 'Settings window did not become visible' }
    Start-Sleep -Seconds $WarmupSeconds
    Push-Location $workspace
    try {
        $expected=($Frontend -ne 'MyGo').ToString().ToLowerInvariant()
        $health = & $GoExe run ./tests/resource-metrics/probe --pipe $pipeName "--expect-enabled=$expected"
        if ($LASTEXITCODE -ne 0) { throw 'Module health check failed; this is not a valid idle baseline' }
        [IO.File]::WriteAllText((Join-Path $outputRoot 'module-health.json'),($health -join [Environment]::NewLine),[Text.UTF8Encoding]::new($false))
    } finally { Pop-Location }
    $inventory = @(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
    $root = @($inventory | Where-Object ProcessId -eq $desktopProcess.Id)
    if ($root.Count -ne 1 -or $null -eq $root[0].CreationDate) { throw 'Desktop identity unavailable' }
    $uiKnown[[string]$desktopProcess.Id] = [string]$root[0].CreationDate.ToUniversalTime().Ticks
    $family = Get-TrackedProcessFamily -Inventory $inventory -Known $uiKnown
    if ($Frontend -eq 'WebView2' -and @($family.Members | Where-Object Name -eq 'msedgewebview2.exe').Count -eq 0) { throw 'No WebView2 process was observed' }
    & (Join-Path $PSScriptRoot 'measure-resources.ps1') -RootProcessIds @($hostProcess.Id,$desktopProcess.Id) -OutputPath (Join-Path $outputRoot 'desktop-open.json') -Scenario ('settings-open-empty-profile-'+$Frontend) -DurationSeconds $DurationSeconds -BinaryPaths @($hostExe,$desktopExe) -WindowHandle ($windowHandle.ToInt64()) -WindowProcessId $desktopProcess.Id
    $openReport=Get-Content -LiteralPath (Join-Path $outputRoot 'desktop-open.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not (Test-OpenWindowResourceReport -Report $openReport -HostProcessId $hostProcess.Id -DesktopProcessId $desktopProcess.Id -Frontend $Frontend)) { throw 'Open-window sampling is incomplete or the expected visible process group is missing; profile retained' }
    $inventory = @(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
    $family = Get-TrackedProcessFamily -Inventory $inventory -Known $uiKnown
    # Send the normal close request only to the process this script created.
    $closeTimer = [Diagnostics.Stopwatch]::StartNew()
    $desktop.CloseWindow([uint32]$desktopProcess.Id)
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
    $desktop.Dispose()
    # Failure profiles are kept for diagnosis, never deleted beneath survivors.
    if ($completed) {
        Add-Type -AssemblyName Microsoft.VisualBasic
        [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($profile,[Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs,[Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin)
    }
}
