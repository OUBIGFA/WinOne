param([string]$GoExe = 'go', [ValidateSet('startup','always-on-top','administrator','capture-scroll','wintray','all')][string]$Suite = 'all', [string]$AlwaysOnTopExe)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
if (-not $AlwaysOnTopExe) { $AlwaysOnTopExe = Join-Path $root '_temp/build/current/always-on-top/WinOne.AlwaysOnTop.exe' }
$AlwaysOnTopExe = [IO.Path]::GetFullPath($AlwaysOnTopExe)
$runDir = Join-Path $root ('_temp/native-checks/' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $runDir -Force | Out-Null
$checks = @()
if ($Suite -in @('capture-scroll','all')) {
    $exe = Join-Path $root 'tests/capture-scroll/bin/Release/net10.0-windows/WinOne.Capture.ScrollTests.exe'
    if (-not (Test-Path -LiteralPath $exe)) { throw 'Build tests/capture-scroll/ScrollTests.csproj with ShareXSourceDirectory first' }
    $checks += @{ exe=$exe; name='capture-scroll'; args=@($runDir) }
}
function CompileTest([string]$directory, [string]$package, [string]$name) {
    $exe = Join-Path $runDir ($name + '.test.exe')
    Push-Location $directory
    try {
        & $GoExe test -c -o $exe $package
        if ($LASTEXITCODE -ne 0) { throw "Cannot compile $name checks" }
    } finally { Pop-Location }
    return $exe
}
if ($Suite -in @('startup','all')) {
    $exe = CompileTest (Join-Path $root 'modules/wintray') './internal/startup' 'startup'
    $checks += @{ exe=$exe; name='startup'; args=@('-test.run=Test(OriginalStartupLiveRoundTrip|ConfiguredStartupLiveRoundTrip|FrequencyLiveRoundTrip|LogonTaskRegistersWithSchtasks|ScheduledTaskLookupConfirmsMissingName)$','-test.v','-test.timeout=90s') }
}
if ($Suite -eq 'wintray') {
    $exe = CompileTest (Join-Path $root 'modules/wintray') './internal/tray' 'wintray-menu'
    $checks += @{ exe=$exe; name='wintray-menu'; cwd=(Join-Path $root 'modules/wintray/internal/tray'); args=@('-test.run=^Test(BoxRightClickThroughNativeMenuLoop|HostedMenuShowsHidesAndReleasesItsOwnOffscreenWindow)$','-test.v','-test.timeout=30s') }
}
if ($Suite -in @('always-on-top','administrator','all')) {
    foreach ($artifact in @($AlwaysOnTopExe,(Join-Path (Split-Path -Parent $AlwaysOnTopExe) 'WinOne.AlwaysOnTop.Native.dll'),(Join-Path $root 'tests/window-fixture/bin/Release/net10.0-windows/WinOne.WindowFixture.exe'))) {
        if (-not (Test-Path -LiteralPath $artifact)) { throw "Build required artifact first: $artifact" }
    }
    if ($Suite -eq 'administrator') {
        $exe = CompileTest $root './internal/modules' 'administrator'
        $checks += @{ exe=$exe; name='administrator'; args=@('-test.run=^TestLiveAdministratorPrivilegeRoundTrip$','-test.v','-test.timeout=150s') }
    } else {
    $exe = CompileTest $root './tests/integration' 'always-on-top'
    $checks += @{ exe=$exe; name='always-on-top'; args=@('-test.run=^TestAlwaysOnTopOwnedWindowRestoration$','-test.v','-test.timeout=30s') }
    $exe = CompileTest $root './internal/modules' 'privilege'
    $checks += @{ exe=$exe; name='privilege'; args=@('-test.run=^TestPrivilegeCancellationRestoresRealOwnedWindow$','-test.v','-test.timeout=30s') }
    }
}
$config = @{ root=$root; aotExe=$AlwaysOnTopExe; liveElevation=($Suite -eq 'administrator'); trayUI=($Suite -eq 'wintray'); checks=@($checks) } | ConvertTo-Json -Depth 5
[IO.File]::WriteAllText((Join-Path $runDir 'checks.json'), $config, [Text.UTF8Encoding]::new($false))
# A current-user interactive task avoids inherited tool-process registry/window differences.
$runner = @'
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$env:TEMP = Join-Path $PSScriptRoot 'data'
$env:TMP = $env:TEMP
New-Item -ItemType Directory -Path $env:TEMP -Force | Out-Null
$failed = $false
try {
    $config = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'checks.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    $env:WINTRAY_STARTUP_LIVE_TEST = '1'
    $env:WINTRAY_UI_TEST = if ($config.trayUI) { '1' } else { '' }
    if ($config.liveElevation) { $env:WINONE_LIVE_ELEVATION_TEST = '1' } else { $env:WINONE_LIVE_ELEVATION_TEST = '' }
    $env:WINONE_AOT_EXE = $config.aotExe
    $env:WINONE_WINDOW_FIXTURE = Join-Path $config.root 'tests/window-fixture/bin/Release/net10.0-windows/WinOne.WindowFixture.exe'
    foreach ($check in $config.checks) {
        $arguments = @($check.args)
        $working = if ($check.cwd) { $check.cwd } else { $config.root }
        Push-Location $working
        try {
            & $check.exe @arguments 2>&1 | Out-File -LiteralPath (Join-Path $PSScriptRoot ($check.name + '.log')) -Encoding utf8
            if ($LASTEXITCODE -ne 0) { $failed = $true }
        } finally { Pop-Location }
    }
} catch {
    $failed = $true
    $_ | Out-File -LiteralPath (Join-Path $PSScriptRoot 'runner-error.log') -Encoding utf8
} finally {
    [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'exit.txt'), [string][int]$failed, [Text.UTF8Encoding]::new($false))
}
'@
[IO.File]::WriteAllText((Join-Path $runDir 'run.ps1'), $runner, [Text.UTF8Encoding]::new($false))
$sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$arguments = [Security.SecurityElement]::Escape('-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "' + (Join-Path $runDir 'run.ps1') + '"')
$xml = @"
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><Triggers/><Principals><Principal id="User"><UserId>$sid</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals><Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><ExecutionTimeLimit>PT3M</ExecutionTimeLimit></Settings><Actions Context="User"><Exec><Command>powershell.exe</Command><Arguments>$arguments</Arguments></Exec></Actions></Task>
"@
[IO.File]::WriteAllText((Join-Path $runDir 'task.xml'), $xml, [Text.UTF8Encoding]::new($false))
$taskName = 'WinOne-NativeCheck-' + [Guid]::NewGuid().ToString('N')
& schtasks.exe /Create /TN $taskName /XML (Join-Path $runDir 'task.xml') | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Could not register temporary verification task' }
try {
    & schtasks.exe /Run /TN $taskName | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not start verification task' }
    $deadline = [DateTime]::UtcNow.AddSeconds(150)
    while (-not (Test-Path -LiteralPath (Join-Path $runDir 'exit.txt'))) {
        if ([DateTime]::UtcNow -gt $deadline) { throw "Verification timed out; logs: $runDir" }
        Start-Sleep -Milliseconds 500
    }
    if ((Get-Content -LiteralPath (Join-Path $runDir 'exit.txt') -Raw).Trim() -ne '0') { throw "Native verification failed; logs: $runDir" }
    Write-Output "Native verification passed; logs: $runDir"
} finally {
    $task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if ($task -and $task.State -eq 'Running') { Stop-ScheduledTask -TaskName $taskName }
    & schtasks.exe /Delete /TN $taskName /F | Out-Null
    if ($LASTEXITCODE -ne 0) { Write-Warning "Temporary verification task needs cleanup: $taskName" }
}
