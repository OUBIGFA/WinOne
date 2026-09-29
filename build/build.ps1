param([string]$GoExe = 'go', [switch]$SkipInstall)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$env:npm_config_cache = Join-Path $root '_temp/build/npm-cache'
function CheckExit { if ($LASTEXITCODE -ne 0) { throw "Command failed: $LASTEXITCODE" } }
if (-not $SkipInstall) { & npm.cmd ci --prefix apps/desktop/frontend --no-audit --no-fund; CheckExit }
& npm.cmd test --prefix apps/desktop/frontend; CheckExit
& npm.cmd run build --prefix apps/desktop/frontend; CheckExit
& $GoExe run ./build/icon -out internal/branding/icon.ico; CheckExit
& $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico internal/branding/icon.ico -o apps/desktop/resource_windows.syso; CheckExit
& $GoExe test ./...; CheckExit
& $GoExe vet ./...; CheckExit
& $GoExe run github.com/akavel/rsrc@v0.10.2 -manifest build/windows/app.manifest -ico internal/branding/icon.ico -o modules/wintray/cmd/worker/resource_windows.syso; CheckExit
Push-Location modules/wintray
try {
    & $GoExe test ./worker ./internal/config ./internal/stringutil; CheckExit
    & $GoExe build -trimpath -ldflags '-H windowsgui' -o ../../dist/WinOne.WinTray.exe ./cmd/worker; CheckExit
    $previousWorker = $env:WINONE_WINTRAY_EXE
    try {
        $env:WINONE_WINTRAY_EXE = Join-Path $root 'dist/WinOne.WinTray.exe'
        & $GoExe test ./worker -run TestWorkerPersistsAndRunsScript -count=1 -timeout 35s; CheckExit
    } finally { $env:WINONE_WINTRAY_EXE = $previousWorker }
} finally { Pop-Location }
New-Item -ItemType Directory -Path (Join-Path $root 'dist') -Force | Out-Null
& $GoExe build -trimpath -o dist/WinOne.Host.exe ./apps/host; CheckExit
& $GoExe build -trimpath -tags desktop,production -ldflags '-H windowsgui' -o dist/WinOne.exe ./apps/desktop; CheckExit
$previousHost = $env:WINONE_HOST_EXE
try {
    $env:WINONE_HOST_EXE = Join-Path $root 'dist/WinOne.Host.exe'
    & $GoExe test ./tests/integration -count=1; CheckExit
} finally { $env:WINONE_HOST_EXE = $previousHost }
Write-Output 'Build verified: dist/WinOne.exe'
