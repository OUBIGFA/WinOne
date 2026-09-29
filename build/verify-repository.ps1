$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
$tracked = @(& git ls-files)
if ($LASTEXITCODE -ne 0) { throw 'Unable to inspect Git tracking' }
$forbidden = $tracked | Where-Object { $_ -match '(^|/)(_temp|node_modules|dist|bin|obj|coverage|test-results|playwright-report)/|\.syso$|\.local\.(ps1|json)$|(^|/)\.env($|\.)' -and $_ -notmatch '(^|/)\.env\.example$' }
if ($forbidden) { throw "Local files are tracked:`n$($forbidden -join "`n")" }
& git diff --check
if ($LASTEXITCODE -ne 0) { throw 'Working diff contains whitespace errors' }
& git diff --cached --check
if ($LASTEXITCODE -ne 0) { throw 'Staged diff contains whitespace errors' }
foreach ($sample in '_temp/probe.txt', 'apps/desktop/_temp/probe.txt', 'dist/WinOne.exe', 'apps/desktop/frontend/node_modules/probe.js') {
    & git check-ignore -q --no-index -- $sample
    if ($LASTEXITCODE -ne 0) { throw "Missing ignore rule: $sample" }
}
Write-Output 'Repository tracking verified'
