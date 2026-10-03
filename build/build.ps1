param([string]$GoExe = 'go', [string]$Downloader, [switch]$SkipInstall, [string]$OutputDirectory = 'dist/build', [switch]$RunStartupProbes, [string]$UpdateRepository = '')
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$buildVersion = (Get-Content -LiteralPath (Join-Path $root 'apps/desktop/frontend/package.json') -Raw -Encoding UTF8 | ConvertFrom-Json).version
# These values enter Go linker arguments, so reject URLs, whitespace and shell syntax.
if ($UpdateRepository -and ($UpdateRepository -notmatch '^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?/[A-Za-z0-9_.-]{1,100}$' -or ($UpdateRepository.Split('/')[1] -in @('.', '..')))) { throw 'UpdateRepository must be an owner/repository name' }
if ($buildVersion -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') { throw 'Invalid build version' }
Set-Location $root
$output = if ([IO.Path]::IsPathRooted($OutputDirectory)) { [IO.Path]::GetFullPath($OutputDirectory) } else { [IO.Path]::GetFullPath((Join-Path $root $OutputDirectory)) }
# Build only into a Git-ignored tree; keep alternate previews out of tracked source.
& git check-ignore -q --no-index -- (Join-Path $output 'build-output.probe')
if ($LASTEXITCODE -ne 0) { throw 'OutputDirectory must be inside a Git-ignored directory such as dist/preview' }
New-Item -ItemType Directory -Path $output -Force | Out-Null
$licenseDirectory = Join-Path $output 'licenses'
New-Item -ItemType Directory -Path $licenseDirectory -Force | Out-Null
Get-ChildItem -LiteralPath (Join-Path $root 'third_party/licenses') -File | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $licenseDirectory -Force }
$env:npm_config_cache = Join-Path $root '_temp/build/npm-cache'
function CheckExit { if ($LASTEXITCODE -ne 0) { throw "Command failed: $LASTEXITCODE" } }
if (-not $SkipInstall) { & npm.cmd ci --prefix apps/desktop/frontend --no-audit --no-fund; CheckExit }
& npm.cmd test --prefix apps/desktop/frontend; CheckExit
& npm.cmd run build --prefix apps/desktop/frontend; CheckExit
Push-Location third_party/systray
try { & $GoExe test ./...; CheckExit } finally { Pop-Location }
& $GoExe run ./build/icon -out internal/branding/icon.ico -native-png internal/branding/logo-native.png; CheckExit
& (Join-Path $PSScriptRoot 'build-always-on-top.ps1') -OutputDirectory (Join-Path $output 'always-on-top')
& $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico internal/branding/icon.ico -o modules/always-on-top/cmd/worker/resource_windows.syso; CheckExit
& $GoExe build -trimpath -ldflags '-H windowsgui' -o (Join-Path $output 'always-on-top/WinOne.AlwaysOnTop.exe') ./modules/always-on-top/cmd/worker; CheckExit
$sharex = & (Join-Path $PSScriptRoot 'prepare-sharex.ps1') -Downloader $Downloader
& dotnet build tests/capture-unit/CaptureTests.csproj -c Release "-p:ShareXSourceDirectory=$sharex" --nologo; CheckExit
& dotnet build tests/capture-scroll/ScrollTests.csproj -c Release "-p:ShareXSourceDirectory=$sharex" --nologo; CheckExit
& dotnet tests/capture-unit/bin/Release/net10.0-windows/WinOne.Capture.Tests.dll (Join-Path $root '_temp/capture-tests'); CheckExit
& dotnet publish modules/capture/WinOne.Capture.csproj -c Release -r win-x64 --self-contained true "-p:ShareXSourceDirectory=$sharex" -o (Join-Path $output 'capture') --nologo; CheckExit
& (Join-Path $PSScriptRoot 'build-capture-regions.ps1') -OutputDirectory (Join-Path $output 'capture')
$regionLifetimeProbe = Join-Path $root '_temp/build/capture-regions-lifetime-probe'
& (Join-Path $PSScriptRoot 'build-capture-regions.ps1') -TestLifetime -OutputDirectory $regionLifetimeProbe
& dotnet tests/capture-unit/bin/Release/net10.0-windows/WinOne.Capture.Tests.dll --region-helper-lifetime (Join-Path $regionLifetimeProbe 'WinOne.Capture.Regions.LifetimeProbe.exe'); CheckExit
if ($RunStartupProbes) {
    $previousRegionHelper = $env:WINONE_REGION_HELPER
    try {
        $env:WINONE_REGION_HELPER = Join-Path $output 'capture/WinOne.Capture.Regions.exe'
        & dotnet tests/capture-unit/bin/Release/net10.0-windows/WinOne.Capture.Tests.dll --native-window-regions; CheckExit
    } finally { $env:WINONE_REGION_HELPER = $previousRegionHelper }
}
& $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico internal/branding/icon.ico -o apps/desktop/resource_windows.syso; CheckExit
& $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico internal/branding/icon.ico -o apps/host/resource_windows.syso; CheckExit
& $GoExe test ./...; CheckExit
& $GoExe vet ./...; CheckExit
& $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico internal/branding/icon.ico -o modules/wintray/cmd/worker/resource_windows.syso; CheckExit
Push-Location modules/wintray
try {
    & $GoExe test ./worker ./internal/config ./internal/stringutil ./internal/tray ./internal/traybox ./internal/orchestrator; CheckExit
    & $GoExe build -trimpath -ldflags '-H windowsgui' -o (Join-Path $output 'WinOne.WinTray.exe') ./cmd/worker; CheckExit
    $previousWorker = $env:WINONE_WINTRAY_EXE
    try {
        $env:WINONE_WINTRAY_EXE = Join-Path $output 'WinOne.WinTray.exe'
        & $GoExe test ./worker -run TestWorkerPersistsAndRunsScript -count=1 -timeout 35s; CheckExit
    } finally { $env:WINONE_WINTRAY_EXE = $previousWorker }
} finally { Pop-Location }
# Host is a background application even when Task Scheduler launches it directly.
& $GoExe build -trimpath -ldflags '-H windowsgui' -o (Join-Path $output 'WinOne.Host.exe') ./apps/host; CheckExit
& $GoExe build -trimpath -tags desktop,production -ldflags "-H windowsgui -X main.updateRepository=$UpdateRepository -X main.updateVersion=$buildVersion" -o (Join-Path $output 'WinOne.exe') ./apps/desktop; CheckExit
$previousHost = $env:WINONE_HOST_EXE
$previousCapture = $env:WINONE_CAPTURE_EXE
$previousProbeBundle = $env:WINONE_PROBE_BUNDLE
$previousDesktopProbe = $env:WINONE_DESKTOP_PROBE_EXE
$previousNativeHotkey = $env:WINONE_NATIVE_HOTKEY_TEST
try {
    $env:WINONE_HOST_EXE = Join-Path $output 'WinOne.Host.exe'
    $env:WINONE_CAPTURE_EXE = Join-Path $output 'capture/WinOne.Capture.exe'
    if ($RunStartupProbes) { $env:WINONE_PROBE_BUNDLE = $output; $env:WINONE_DESKTOP_PROBE_EXE = Join-Path $output 'WinOne.exe'; $env:WINONE_NATIVE_HOTKEY_TEST='1' }
    & $GoExe test ./internal/startupprobe -count=1; CheckExit
    & $GoExe test ./apps/host -run '^TestNativeConfigurationProbe$|^TestNativeRecordingReleasesAndRestoresRegistration$' -count=1; CheckExit
    & $GoExe test ./tests/integration -count=1; CheckExit
} finally { $env:WINONE_HOST_EXE = $previousHost; $env:WINONE_CAPTURE_EXE = $previousCapture; $env:WINONE_PROBE_BUNDLE = $previousProbeBundle; $env:WINONE_DESKTOP_PROBE_EXE = $previousDesktopProbe; $env:WINONE_NATIVE_HOTKEY_TEST=$previousNativeHotkey }
$currentVersion = (Get-Content -LiteralPath (Join-Path $root 'apps/desktop/frontend/package.json') -Raw -Encoding UTF8 | ConvertFrom-Json).version
if ($currentVersion -ne $buildVersion) { throw 'Version changed during build; rebuild before packaging' }
& node (Join-Path $root 'tests/notices/frontend.test.mjs'); CheckExit
& (Join-Path $PSScriptRoot 'verify-notices.ps1') -GoExe $GoExe -BundleDirectory $output
& $GoExe run ./build/package --record-build --bundle $output --license (Join-Path $root 'LICENSE') --version $buildVersion; CheckExit
Write-Output "Build verified: $output/WinOne.exe"
