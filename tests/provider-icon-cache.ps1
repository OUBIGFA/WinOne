param(
    [string]$SeedDirectory = 'dist/resources/assets/provider-icons',
    [string]$OutputDirectory = ('_temp/provider-icon-cache-' + [Guid]::NewGuid().ToString('N').Substring(0, 8))
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
& (Join-Path $root 'build/assert-temporary-path.ps1') -Path $OutputDirectory
$fixture = if ([IO.Path]::IsPathRooted($OutputDirectory)) { [IO.Path]::GetFullPath($OutputDirectory) } else { Join-Path $root $OutputDirectory }
$seed = if ([IO.Path]::IsPathRooted($SeedDirectory)) { $SeedDirectory } else { Join-Path $root $SeedDirectory }
if (-not (Test-Path -LiteralPath (Join-Path $seed 'inputs.sha256'))) {
    throw 'Prepare the real icon cache with build/build-provider-icons.ps1 before running this check'
}
if (Test-Path -LiteralPath $fixture) { throw 'Use a new, empty test output directory' }

# Use the real cache and scripts in an isolated checkout layout. Unavailable
# compiler names prove that a cache hit needs neither a toolchain nor downloads.
$files = @('build/build-provider-icons.ps1', 'build/build-diagrams.ps1', 'build/assert-temporary-path.ps1',
    'apps/diagram-renderer/Cargo.toml', 'apps/diagram-renderer/Cargo.lock', 'apps/diagram-renderer/build.rs')
$files += Get-ChildItem -LiteralPath (Join-Path $root 'apps/diagram-renderer/src') -Recurse -File -Filter '*.rs' | ForEach-Object { $_.FullName.Substring($root.Length + 1) }
$files += Get-ChildItem -LiteralPath (Join-Path $root 'apps/result-ui/assets/providers') -File -Filter '*.svg' | ForEach-Object { $_.FullName.Substring($root.Length + 1) }
foreach ($relative in $files) {
    $destination = Join-Path $fixture $relative
    [IO.Directory]::CreateDirectory((Split-Path -Parent $destination)) | Out-Null
    Copy-Item -LiteralPath (Join-Path $root $relative) -Destination $destination
}
$cache = Join-Path $fixture 'dist/resources/assets/provider-icons'
[IO.Directory]::CreateDirectory($cache) | Out-Null
Get-ChildItem -LiteralPath $seed -File | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $cache }
$script = Join-Path $fixture 'build/build-provider-icons.ps1'
$parameters = @{ CargoExe = '__winone_cache_test_no_cargo__'; GoExe = '__winone_cache_test_no_go__'; WorkDirectory = '_temp/render' }
$output = Join-Path $fixture 'apps/result-ui/assets/provider-bitmaps'
$manifest = Get-Content -LiteralPath (Join-Path $seed 'manifest.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$first = $manifest.files[0].name
$outputPng = Join-Path $output ($first + '.png')

& $script @parameters
if (@(Get-ChildItem -LiteralPath $output -Filter '*.png').Count -ne @($manifest.files).Count) { throw 'Cache restore omitted icons' }
foreach ($entry in $manifest.files) {
    if ((Get-FileHash -LiteralPath (Join-Path $output ($entry.name + '.png'))).Hash.ToLowerInvariant() -ne $entry.pngSHA256) { throw 'Cache restore changed an icon' }
}
$writeTime = (Get-Item -LiteralPath $outputPng).LastWriteTimeUtc
& $script @parameters
if ((Get-Item -LiteralPath $outputPng).LastWriteTimeUtc -ne $writeTime) { throw 'An unchanged embedded icon was rewritten' }
if (Test-Path -LiteralPath (Join-Path $fixture '_temp/render')) { throw 'A cache hit created rendering output' }

[IO.File]::WriteAllText($outputPng, 'damaged embedded image', [Text.UTF8Encoding]::new($false))
& $script @parameters
if ((Get-FileHash -LiteralPath $outputPng).Hash.ToLowerInvariant() -ne $manifest.files[0].pngSHA256) { throw 'Valid cache did not restore a damaged embedded icon' }

function Assert-StaleCacheRejected([string]$relative) {
    $path = Join-Path $fixture $relative
    $original = [IO.File]::ReadAllBytes($path)
    $before = (Get-FileHash -LiteralPath $outputPng).Hash
    try {
        [IO.File]::WriteAllText($path, 'changed cache test input', [Text.UTF8Encoding]::new($false))
        $rejected = $false
        try { & $script @parameters } catch {
            # Reaching the unavailable compiler proves that stale input was
            # rejected instead of being copied into the embedded resources.
            if ($_.Exception.Message -notlike '*__winone_cache_test_no_cargo__*') { throw }
            $rejected = $true
        }
        if (-not $rejected) { throw "Stale cache was accepted after changing $relative" }
        if ((Get-FileHash -LiteralPath $outputPng).Hash -ne $before) { throw 'A failed rebuild modified embedded resources' }
    } finally { [IO.File]::WriteAllBytes($path, $original) }
}

foreach ($relative in @(
    ('dist/resources/assets/provider-icons/' + $first + '.png'),
    'dist/resources/assets/provider-icons/manifest.json',
    'dist/resources/assets/provider-icons/inputs.sha256',
    ('apps/result-ui/assets/providers/' + $first + '.svg'),
    'apps/diagram-renderer/Cargo.lock',
    'apps/diagram-renderer/src/bin/winone-icon-assets.rs'
)) { Assert-StaleCacheRejected $relative }

$rejected = $false
try { & $script @parameters -CacheDirectory 'apps/unsafe-cache' } catch {
    if ($_.Exception.Message -notlike 'Temporary output must*') { throw }
    $rejected = $true
}
if (-not $rejected) { throw 'Cache writer accepted a source directory' }
Write-Output "Provider icon cache verified: offline restore, repeat reuse, embedded repair, 6 stale inputs, output boundary ($fixture)"
