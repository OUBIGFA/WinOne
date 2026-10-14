param(
    [Parameter(Mandatory=$true)][string]$CargoExe,
    [Parameter(Mandatory=$true)][string]$HostExe,
    [string]$GoExe,
    [string]$TargetDirectory = 'dist/resources/cache/rust-desktop',
    [string]$WorkDirectory = '_temp/rust-desktop-roundtrip'
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$work = [IO.Path]::GetFullPath((Join-Path $root $WorkDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $work
& git -C $root check-ignore -q --no-index -- (Join-Path $work 'roundtrip.probe')
if ($LASTEXITCODE -ne 0) { throw 'Test directory must be Git-ignored' }
$case = Join-Path $work ([guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $case -Force | Out-Null
# Build the current Host into this owned case when requested, leaving trial
# binaries untouched. A supplied Host must implement the tested configuration API.
if ($GoExe) {
    $HostExe = Join-Path $case 'WinOne.Host.exe'
    Push-Location $root
    try {
        & $GoExe test ./apps/host -run 'Test(AOTPreferences|CapturePreferences|WinTray|DiagnosticsService|DesktopControl)' -count=1
        if ($LASTEXITCODE -ne 0) { throw 'Host configuration tests failed' }
        & $GoExe build -trimpath -ldflags '-H windowsgui' -o $HostExe ./apps/host
        if ($LASTEXITCODE -ne 0) { throw 'Host build failed' }
    } finally { Pop-Location }
}
$pipe = '\\.\pipe\WinOne-rust-test-' + [guid]::NewGuid().ToString('N')
$savedPipe = $env:WINONE_RUST_TEST_PIPE
$process = $null
try {
    # A known log in this owned case verifies the real Host read boundary; it
    # must not depend on, expose, or create logs in the user's active profile.
    $logDirectory = Join-Path $case 'data/modules/capture'
    New-Item -ItemType Directory -Path $logDirectory -Force | Out-Null
    # Keep this runner readable by Windows PowerShell 5.1 without a BOM.
    $logText = 'Rust/Host ' + [char]0x65E5 + [char]0x5FD7 + ' fixture'
    [IO.File]::WriteAllText((Join-Path $logDirectory 'worker.log'), $logText, [Text.UTF8Encoding]::new($false))
    # A separate headless Host has no product window, tray or user configuration.
    $arguments = @('--headless', '--standard-session', '--pipe', $pipe, '--data-dir', ('"' + (Join-Path $case 'data') + '"'))
    $process = Start-Process -FilePath $HostExe -ArgumentList $arguments -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $case 'host.stdout.log') -RedirectStandardError (Join-Path $case 'host.stderr.log')
    $env:WINONE_RUST_TEST_PIPE = $pipe
    & (Join-Path $PSScriptRoot 'build-desktop-rust.ps1') -CargoExe $CargoExe -TargetDirectory $TargetDirectory -TestOnly -HostIntegration
    if ($LASTEXITCODE -ne 0) { throw 'Rust/Host integration failed' }
    if ($GoExe) {
        # Resolve the exact test artifact from Cargo metadata, not a stale glob.
        $manifest = Join-Path $root 'apps/desktop-rust/Cargo.toml'
        $savedRustc = $env:RUSTC
        $savedTestExe = $env:WINONE_RUST_CONFIGURATION_TEST_EXE
        $savedPermissionHelper = $env:WINONE_RUST_PERMISSION_HELPER
        $savedTemp = $env:TEMP; $savedTmp = $env:TMP
        Push-Location $root
        try {
            $cargoPath = (Get-Command $CargoExe -ErrorAction Stop).Source
            $env:RUSTC = Join-Path (Split-Path -Parent $cargoPath) 'rustc.exe'
            $artifactDirectory = if ([IO.Path]::IsPathRooted($TargetDirectory)) { $TargetDirectory } else { Join-Path $root $TargetDirectory }
            $artifacts = & $CargoExe test --locked --no-run --manifest-path $manifest --target-dir $artifactDirectory --bin winone-desktop --message-format=json
            if ($LASTEXITCODE -ne 0) { throw 'Rust configuration test artifact failed' }
            $artifact = $artifacts | ForEach-Object { $_ | ConvertFrom-Json } | Where-Object { $_.reason -eq 'compiler-artifact' -and $_.profile.test -and $_.executable } | Select-Object -Last 1
            if (-not $artifact) { throw 'Rust configuration test artifact missing' }
            $env:WINONE_RUST_CONFIGURATION_TEST_EXE = $artifact.executable
            $env:WINONE_RUST_PERMISSION_HELPER = $HostExe
            $permissionProbe = Start-Process -FilePath $artifact.executable -ArgumentList '--ignored --exact permissions::tests::real_host_helper_rejects_isolated_profiles_without_authorizing --nocapture' -WindowStyle Hidden -Wait -PassThru -RedirectStandardOutput (Join-Path $case 'permission.stdout.log') -RedirectStandardError (Join-Path $case 'permission.stderr.log')
            Get-Content -LiteralPath (Join-Path $case 'permission.stdout.log'),(Join-Path $case 'permission.stderr.log') -Encoding UTF8
            if ($permissionProbe.ExitCode -ne 0) { throw 'Rust permission helper boundary failed' }
            $env:TEMP = $case; $env:TMP = $case
            & $GoExe test ./apps/host -run '^TestRustDesktop(ConfigurationRoundtrip|MaintenanceLocks)$' -count=1 -v
            if ($LASTEXITCODE -ne 0) { throw 'Rust configuration handlers integration failed' }
        } finally {
            $env:RUSTC = $savedRustc; $env:WINONE_RUST_CONFIGURATION_TEST_EXE = $savedTestExe
            $env:WINONE_RUST_PERMISSION_HELPER = $savedPermissionHelper
            $env:TEMP = $savedTemp; $env:TMP = $savedTmp
            Pop-Location
        }
    }
} finally {
    $env:WINONE_RUST_TEST_PIPE = $savedPipe
    # Only this runner-owned, headless process is stopped; it has no module tasks.
    if ($process -and -not $process.HasExited) { Stop-Process -Id $process.Id; $process.WaitForExit() }
}
