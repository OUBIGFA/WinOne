param(
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$GoExe = 'dist/resources/tools/go-1.27.3/bin/go.exe',
    [string]$UpdateRepository = '',
    [string]$Version = '',
    [switch]$SkipTests
)
# Builds the MyGo settings window (apps/desktop-mygo) as WinOne.exe: icon
# check, tests, vet, Windows resources, then a production binary without
# MyGo's inspector. Go 1.27.3 is MyGo v0.7.3's minimum; the Host and
# modules keep their own toolchain.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$output = if ([IO.Path]::IsPathRooted($OutputDirectory)) { [IO.Path]::GetFullPath($OutputDirectory) } else { [IO.Path]::GetFullPath((Join-Path $root $OutputDirectory)) }
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'WinOne.exe')
if ($LASTEXITCODE -ne 0) { throw 'OutputDirectory must be inside a Git-ignored directory' }
$go = if ([IO.Path]::IsPathRooted($GoExe)) { $GoExe } else { Join-Path $root $GoExe }
$go = (Get-Command $go -ErrorAction Stop).Source
if ((& $go version) -notmatch 'go1\.27\.([3-9]|[1-9][0-9])') { throw 'MyGo v0.7.3 needs Go 1.27.3 or later' }
if (-not $Version) { $Version = (Get-Content -LiteralPath (Join-Path $root 'apps/desktop/frontend/package.json') -Raw -Encoding UTF8 | ConvertFrom-Json).version }
if ($Version -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') { throw 'Invalid version' }
if ($UpdateRepository -and $UpdateRepository -notmatch '^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?/[A-Za-z0-9_.-]{1,100}$') { throw 'UpdateRepository must be owner/repository' }

# Provider marks come from the result window's inventory, sampled by
# build-provider-icons.ps1 (Git-ignored output).
if (-not (Test-Path -LiteralPath (Join-Path $root 'apps/result-ui/assets/provider-bitmaps/manifest.json'))) { throw 'Run build/build-provider-icons.ps1 before building the MyGo window' }
& node (Join-Path $PSScriptRoot 'generate-mygo-icons.mjs') --check
if ($LASTEXITCODE -ne 0) { throw 'MyGo icons are stale' }

$saved = @{ GOROOT = $env:GOROOT; CGO_ENABLED = $env:CGO_ENABLED; GOTOOLCHAIN = $env:GOTOOLCHAIN; GOCACHE = $env:GOCACHE }
try {
    $env:GOROOT = Split-Path -Parent (Split-Path -Parent $go)
    $env:CGO_ENABLED = '0'
    $env:GOTOOLCHAIN = 'local'
    if (-not $env:GOCACHE) { $env:GOCACHE = Join-Path $root 'dist/resources/cache/go' }
    Push-Location (Join-Path $root 'apps/desktop-mygo')
    try {
        if (-not $SkipTests) {
            & $go test ./... -count=1
            if ($LASTEXITCODE -ne 0) { throw 'MyGo window tests failed' }
        }
        & $go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'MyGo window vet failed' }
        & $go vet -tags mygo_noinspector ./...
        if ($LASTEXITCODE -ne 0) { throw 'MyGo window production vet failed' }
        & $go run github.com/akavel/rsrc@v0.10.2 -manifest (Join-Path $root 'build/windows/app.manifest') -ico (Join-Path $root 'internal/branding/icon.ico') -o cmd/winone/resource_windows.syso
        if ($LASTEXITCODE -ne 0) { throw 'MyGo window resources failed' }
        New-Item -ItemType Directory -Path $output -Force | Out-Null
        $flags = "-s -w -H windowsgui -X github.com/egoist/mygo.production=1 -X winone/desktopmygo/core.Version=$Version -X winone/desktopmygo/core.UpdateRepository=$UpdateRepository"
        & $go build -trimpath -tags mygo_noinspector -ldflags $flags -o (Join-Path $output 'WinOne.exe') ./cmd/winone
        if ($LASTEXITCODE -ne 0) { throw 'MyGo window build failed' }
        # Every linked module must have its recorded license before packaging.
        & (Join-Path $PSScriptRoot 'verify-desktop-mygo-notices.ps1') -Executable (Join-Path $output 'WinOne.exe') -GoExe $go
    } finally { Pop-Location }
} finally {
    foreach ($name in $saved.Keys) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
}
