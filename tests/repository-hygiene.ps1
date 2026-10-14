$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$guard = Join-Path $root 'build/assert-temporary-path.ps1'

# Validate generated output destinations only. User files already in dist are
# outside this check; path validation must not create or remove any files.
foreach ($path in @('_temp/hygiene-test/build', '_temp/hygiene-test/report.json', (Join-Path $root '_temp/hygiene-test'))) {
    & $guard -Path $path
}
foreach ($path in @('dist/test', 'dist/release', 'dist/logo', 'dist/resources/cache/rust-desktop', '_temp', '_temp/../dist/test', '_temporary/test', 'tests/output', (Join-Path (Split-Path -Parent $root) '_temp/test'))) {
    $rejected = $false
    try { & $guard -Path $path } catch { $rejected = $_.Exception.Message -like 'Temporary output must*' }
    if (-not $rejected) { throw "Output boundary was not enforced: $path" }
}
& $guard -Path 'dist/resources/cache/rust-desktop' -ReusableCache
& $guard -Path 'dist/custom-resources/compiler-cache' -ReusableCache
foreach ($path in @('dist', 'dist/', 'dist/custom-resources/../../outside')) {
    $rejected = $false
    try { & $guard -Path $path -ReusableCache } catch { $rejected = $_.Exception.Message -like 'Temporary output must*' }
    if (-not $rejected) { throw "Cache output must use a subdirectory of dist: $path" }
}
if (Test-Path -LiteralPath (Join-Path $root '_temp/hygiene-test')) { throw 'Path validation created an output directory' }
Write-Output 'Temporary output boundaries verified (17 cases; no files created)'
