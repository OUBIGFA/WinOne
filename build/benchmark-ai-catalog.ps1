param([string]$GoExe='go', [Parameter(Mandatory)][string]$OutputDirectory)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
New-Item -ItemType Directory -Force -Path $output | Out-Null
$priorTemp=$env:TEMP;$priorTmp=$env:TMP
Push-Location $root
try {
    $env:TEMP=$output;$env:TMP=$output
    & $GoExe test ./internal/ai -run '^TestCatalogMergePreservesOrderAndOwnership$' -bench '^BenchmarkCatalogReads$' -benchmem -benchtime=200ms -count=3 | Tee-Object -FilePath (Join-Path $output 'benchmark.txt')
    if($LASTEXITCODE){throw 'AI catalog benchmark failed'}
} finally { $env:TEMP=$priorTemp;$env:TMP=$priorTmp;Pop-Location }
