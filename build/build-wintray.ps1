param(
    [string]$GoExe='go',
    [string]$OutputDirectory='_temp/build/current',
    [switch]$RunConsoleChecks
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$GoExe=(Get-Command $GoExe -ErrorAction Stop).Source
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'build-output.probe')
if($LASTEXITCODE -ne 0){throw 'WinTray output must be Git-ignored'}
New-Item -ItemType Directory -Path $output -Force | Out-Null
# The new tray collector needs both target-process architectures. ABI probes
# load only the private test DLL; they do not hook the user's Explorer.
& (Join-Path $PSScriptRoot 'build-wintray-native.ps1') -OutputDir (Join-Path $output 'native') -Test
Push-Location $root
try {
    & $GoExe run ./build/icon -out internal/branding/icon.ico
    if($LASTEXITCODE -ne 0){throw 'WinTray icon generation failed'}
    & $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico internal/branding/icon.ico -o modules/wintray/cmd/worker/resource_windows.syso
    if($LASTEXITCODE -ne 0){throw 'WinTray Windows resources failed'}
} finally {Pop-Location}
$oldConsole=$env:WINTRAY_CONSOLE_LIVE_TEST;$oldWorker=$env:WINONE_WINTRAY_EXE
Push-Location (Join-Path $root 'modules/wintray')
try {
    $env:WINTRAY_CONSOLE_LIVE_TEST=if($RunConsoleChecks){'1'}else{''}
    $testOptions=@();if($RunConsoleChecks){$testOptions=@('-count=1')}
    & $GoExe test ./worker ./internal/config ./internal/stringutil ./internal/tray ./internal/traybox ./internal/orchestrator @testOptions
    if($LASTEXITCODE -ne 0){throw 'WinTray regression checks failed'}
    $worker=Join-Path $output 'WinOne.WinTray.exe'
    & $GoExe build -trimpath -ldflags '-H windowsgui' -o $worker ./cmd/worker
    if($LASTEXITCODE -ne 0){throw 'WinTray build failed'}
    $recovery=Join-Path $output 'WinOne.WinTray.Recovery.exe'
    & $GoExe build -trimpath -ldflags '-H windowsgui' -o $recovery ./cmd/tray-recovery
    if($LASTEXITCODE -ne 0){throw 'WinTray recovery companion build failed'}
    $env:WINONE_WINTRAY_EXE=$worker
    & $GoExe test ./worker -run '^TestWorker(PersistsAndRunsScript|CoexistsWithIndependentWinTray)$' -count=1 -timeout 35s
    if($LASTEXITCODE -ne 0){throw 'Built WinTray worker integration failed'}
    Get-FileHash -LiteralPath $worker,$recovery -Algorithm SHA256
} finally {Pop-Location;$env:WINTRAY_CONSOLE_LIVE_TEST=$oldConsole;$env:WINONE_WINTRAY_EXE=$oldWorker}
