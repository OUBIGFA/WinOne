param(
    [Parameter(Mandatory)][string]$DesktopExe,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$GoExe,
    [string]$HostExe
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$output = [IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'result.json')
if ($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)) { throw 'Choose a new Git-ignored output directory' }
$executable = (Resolve-Path -LiteralPath $DesktopExe).Path
New-Item -ItemType Directory -Path (Join-Path $output 'profile/logs') -Force | Out-Null
$offlineDirectory = Join-Path $output 'offline'
New-Item -ItemType Directory -Path $offlineDirectory | Out-Null
$offlineExecutable = Join-Path $offlineDirectory 'WinOne.exe'
Copy-Item -LiteralPath $executable -Destination $offlineExecutable
$profile = Join-Path $output 'profile'
$log = 'offline ' + [char]0x65E5 + [char]0x5FD7 + ' fixture'
[IO.File]::WriteAllText((Join-Path $profile 'logs/host.log'), $log, [Text.UTF8Encoding]::new($false))
if (-not ('RustDesktopProbe' -as [type])) { Add-Type -Path (Join-Path $PSScriptRoot 'desktop-rust-probe.cs') }
$probe = $null
$savedLocal = $env:LOCALAPPDATA
$env:LOCALAPPDATA = Join-Path $output 'local-appdata'
try {
    # No Host is created. This verifies the shipped executable's offline path,
    # not a successful request to a substitute pipe server.
    $pipe = '\\.\pipe\WinOne-rust-test-offline-' + [guid]::NewGuid().ToString('N')
    $probe = [RustDesktopProbe]::new($offlineExecutable, $pipe, $profile, $false)
    $read = $probe.OpenGlobalLogs($log)
    $reopen = $probe.Reopen()
    $close = $probe.CloseNormally()
    $report = [ordered]@{ Scenario='rust-private-desktop-offline-lifecycle'; HostStarted=$false; LogMilliseconds=$read; ReopenMilliseconds=$reopen; CloseMilliseconds=$close; DesktopSHA256=(Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash }
    $json = $report | ConvertTo-Json
    [IO.File]::WriteAllText((Join-Path $output 'result.json'), $json, [Text.UTF8Encoding]::new($false))
    $json
} finally {
    if ($probe) { $probe.Dispose() }
    $env:LOCALAPPDATA = $savedLocal
}

if ($GoExe) {
    if (-not $HostExe) { throw 'HostExe is required for the Host exit integration' }
    $bundle = Join-Path $output 'bundle'
    New-Item -ItemType Directory -Path $bundle | Out-Null
    Copy-Item -LiteralPath $executable -Destination (Join-Path $bundle 'WinOne.exe')
    Copy-Item -LiteralPath (Resolve-Path -LiteralPath $HostExe).Path -Destination (Join-Path $bundle 'WinOne.Host.exe')
    $report.HostSHA256 = (Get-FileHash -LiteralPath (Join-Path $bundle 'WinOne.Host.exe') -Algorithm SHA256).Hash
    $testExe = Join-Path $output 'host-lifecycle-tests.exe'
    $savedBundle = $env:WINONE_EXIT_BUNDLE
    $savedTemp = $env:TEMP; $savedTmp = $env:TMP
    Push-Location $root
    try {
        $env:LOCALAPPDATA = Join-Path $output 'local-appdata'
        $bootstrapProfile = Join-Path $output 'bootstrap-profile'
        $bootstrapPipe = '\\.\pipe\WinOne-rust-test-bootstrap-' + [guid]::NewGuid().ToString('N')
        $request = @{ pipe=$bootstrapPipe; dataDirectory=$bootstrapProfile; standardSession=$false } | ConvertTo-Json -Compress
        # The Host is a windowsgui executable. Use explicit redirected handles
        # like the Rust caller; PowerShell's native pipeline is not its protocol.
        $start = [Diagnostics.ProcessStartInfo]::new((Join-Path $bundle 'WinOne.Host.exe'),'--desktop-bootstrap')
        $start.UseShellExecute=$false; $start.CreateNoWindow=$true
        $start.RedirectStandardInput=$true; $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
        $bootstrapProcess = [Diagnostics.Process]::Start($start)
        try {
            $bootstrapProcess.StandardInput.Write($request); $bootstrapProcess.StandardInput.Close()
            if (-not $bootstrapProcess.WaitForExit(10000)) { $bootstrapProcess.Kill(); throw 'Bootstrap test helper timed out' }
            $decision = $bootstrapProcess.StandardOutput.ReadToEnd() | ConvertFrom-Json
            if ($bootstrapProcess.ExitCode -ne 0 -or $decision.version -ne 1 -or $decision.redirected -or $decision.warning) { throw ('Desktop bootstrap protocol failed: ' + $bootstrapProcess.StandardError.ReadToEnd()) }
        } finally { $bootstrapProcess.Dispose() }
        $report.BootstrapProtocolPassed = $true
        $automaticProfile = Join-Path $output 'automatic-profile'
        $historyDirectory=Join-Path $automaticProfile 'modules/capture'
        New-Item -ItemType Directory -Path $historyDirectory -Force | Out-Null
        $savedImage=Join-Path $historyDirectory 'saved.png'
        Add-Type -AssemblyName System.Drawing
        $image=[Drawing.Bitmap]::new(24,16)
        try {$image.Save($savedImage,[Drawing.Imaging.ImageFormat]::Png)} finally {$image.Dispose()}
        $metadata=@{id=('a'*32);path=$savedImage;width=24;height=16;copied=$false;pinned=$false} | ConvertTo-Json -Compress
        [IO.File]::WriteAllText((Join-Path $historyDirectory 'last-capture.json'),$metadata,[Text.UTF8Encoding]::new($false))
        $automaticPipe = '\\.\pipe\WinOne-rust-test-automatic-' + [guid]::NewGuid().ToString('N')
        $automaticProbe = $null; $automaticHost = $null
        try {
            # No Host is prestarted: the real Rust entrypoint must create its
            # sibling and reach the normal settings catalog by itself.
            $automaticProbe = [RustDesktopProbe]::new((Join-Path $bundle 'WinOne.exe'), $automaticPipe, $automaticProfile)
            $automaticHost = [Diagnostics.Process]::GetProcessById($automaticProbe.ConnectedHostProcessId())
            if ($automaticHost.MainModule.FileName -ne (Join-Path $bundle 'WinOne.Host.exe')) { throw 'Automatic startup connected to an unexpected Host' }
            $report.AutomaticReadyMilliseconds = $automaticProbe.ReadyMilliseconds
            if (Test-Path -LiteralPath (Join-Path $bundle 'capture/WinOne.Capture.exe')) {throw 'History probe must not bundle the capture runtime'}
            $report.LocalCaptureHistoryMilliseconds=$automaticProbe.ReadSavedCaptureHistory($savedImage)
            [void]$automaticProbe.Reopen()
            if ($automaticProbe.ConnectedHostProcessId() -ne $automaticHost.Id) { throw 'Repeated launch replaced the Host' }
            [void]$automaticProbe.CloseNormally()
            $report.AutomaticHostStartPassed = $true
        } finally {
            if ($automaticProbe) { $automaticProbe.Dispose() }
            if ($automaticHost) {
                # Only the freshly created, empty fixture Host is stopped.
                # Its cleanup is not counted as a normal product-exit check.
                if (-not $automaticHost.HasExited) { $automaticHost.Kill(); $automaticHost.WaitForExit() }
                $automaticHost.Dispose()
            } else {
                Get-CimInstance Win32_Process -Filter "Name = 'WinOne.Host.exe'" | Where-Object {
                    $_.ExecutablePath -eq (Join-Path $bundle 'WinOne.Host.exe') -and $_.CommandLine -and $_.CommandLine.Contains($automaticProfile)
                } | ForEach-Object { Stop-Process -Id $_.ProcessId -ErrorAction Stop }
            }
        }
        & $GoExe test -c -o $testExe ./apps/host
        if ($LASTEXITCODE -ne 0) { throw 'Host lifecycle test build failed' }
        $env:WINONE_EXIT_BUNDLE = $bundle
        $env:TEMP = $output; $env:TMP = $output
        # Reuse the existing packaged-window test: it checks two profiles,
        # another installation, hidden windows, and normal Host-driven close.
        & (Join-Path $root 'tests/capture-host-probe/run-private.ps1') -Suite native-tests -ProbeExe $testExe -TestFilter '^Test(ApplicationExitClosesRealSettings|RustCaptureEntryDoesNotCreateSettingsWindow)$' -OutputDirectory (Join-Path $output 'host-exit')
        if ($LASTEXITCODE -ne 0) { throw 'Host-driven settings exit failed' }
        $report.HostDrivenExitPassed = $true
        $report.CaptureEntryPassed = $true
        [IO.File]::WriteAllText((Join-Path $output 'result.json'), ($report | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    } finally {
        $env:WINONE_EXIT_BUNDLE = $savedBundle
        $env:TEMP = $savedTemp; $env:TMP = $savedTmp
        $env:LOCALAPPDATA = $savedLocal
        Pop-Location
    }
}
