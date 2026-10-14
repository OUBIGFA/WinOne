$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
$tracked = @(& git ls-files)
if ($LASTEXITCODE -ne 0) { throw 'Unable to inspect Git tracking' }
$forbidden = $tracked | Where-Object { $_ -match '(^|/)(_temp|node_modules|dist|bin|obj|target|__pycache__|\.workbuddy|coverage|test-results|playwright-report)/|\.syso$|\.local\.(ps1|json)$|(^|/)\.env($|\.)' -and $_ -notmatch '(^|/)\.env\.example$' }
if ($forbidden) { throw "Local files are tracked:`n$($forbidden -join "`n")" }
& git diff --check
if ($LASTEXITCODE -ne 0) { throw 'Working diff contains whitespace errors' }
& git diff --cached --check
if ($LASTEXITCODE -ne 0) { throw 'Staged diff contains whitespace errors' }
foreach ($sample in '_temp/probe.txt', 'apps/desktop/_temp/probe.txt', 'dist/WinOne.exe', 'apps/desktop/frontend/node_modules/probe.js') {
    & git check-ignore -q --no-index -- $sample
    if ($LASTEXITCODE -ne 0) { throw "Missing ignore rule: $sample" }
}
$ignoreRules = @(Get-Content -LiteralPath '.gitignore' | Where-Object { $_.Trim() -and -not $_.Trim().StartsWith('#') })
if ($ignoreRules[-1] -ne '_temp/') { throw 'Keep _temp/ as the final ignore rule, after all negation rules' }

# Users may keep their own files anywhere in dist. Check the known installer
# when present; never classify other files by a directory or filename allowlist.
if (Test-Path -LiteralPath 'dist/release') {
    if (Test-Path -LiteralPath 'dist/release/WinOne-setup.exe') {
        $digest = (Get-FileHash -LiteralPath 'dist/release/WinOne-setup.exe' -Algorithm SHA256).Hash.ToLowerInvariant()
        $recorded = (Get-Content -LiteralPath 'dist/release/WinOne-setup.exe.sha256' -Raw).Trim()
        if ($recorded -notmatch ('^' + $digest + '\s+\*?WinOne-setup\.exe$')) { throw 'Release SHA-256 does not match the installer' }
        $release = Get-Content -LiteralPath 'dist/release/release-info.json' -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($release.sha256 -ne $digest -or $release.size -ne (Get-Item -LiteralPath 'dist/release/WinOne-setup.exe').Length) { throw 'Release metadata does not match the installer' }
    }
}
& (Join-Path $PSScriptRoot '../tests/repository-hygiene.ps1')
Write-Output 'Repository tracking and generated-output paths verified'
