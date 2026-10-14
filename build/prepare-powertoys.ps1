$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$source = (Get-Content -LiteralPath (Join-Path $root 'third_party/sources.json') -Raw -Encoding UTF8 | ConvertFrom-Json).powertoys
$checkout = Join-Path $root 'dist/resources/sources/PowerToys'
if (-not (Test-Path -LiteralPath (Join-Path $checkout '.git'))) {
    New-Item -ItemType Directory -Path $checkout -Force | Out-Null
    & git -C $checkout init -q
    if ($LASTEXITCODE -ne 0) { throw 'Cannot initialize PowerToys source cache' }
    & git -C $checkout remote add origin $source.repository
    & git -C $checkout fetch --depth 1 --filter=blob:none origin $source.commit
    if ($LASTEXITCODE -ne 0) { throw 'Cannot fetch pinned PowerToys source' }
    & git -C $checkout sparse-checkout init --cone
    & git -C $checkout sparse-checkout set src/modules/alwaysontop src/common src/interop tools/build deps
    & git -C $checkout checkout --detach -q FETCH_HEAD
    if ($LASTEXITCODE -ne 0) { throw 'Cannot check out PowerToys source' }
}
$revision = & git -C $checkout rev-parse HEAD
if ($revision -ne $source.commit) { throw 'PowerToys source revision mismatch' }
return $checkout
