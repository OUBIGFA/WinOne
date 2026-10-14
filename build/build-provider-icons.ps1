param(
    [string]$CargoExe = 'cargo',
    [string]$GoExe = 'go',
    [string]$TargetDirectory = 'dist/resources/cache/rust-diagrams',
    [string]$WorkDirectory = '_temp/provider-icons-build',
    [string]$CacheDirectory = 'dist/resources/assets/provider-icons'
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$source = Join-Path $root 'apps/result-ui/assets/providers'
$output = Join-Path $root 'apps/result-ui/assets/provider-bitmaps'
function Absolute([string]$path) {
    if ([IO.Path]::IsPathRooted($path)) { [IO.Path]::GetFullPath($path) } else { [IO.Path]::GetFullPath((Join-Path $root $path)) }
}
function Digest([string]$path) { (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() }
function MatchesDigest([string]$path, [string]$expected) {
    (Test-Path -LiteralPath $path -PathType Leaf) -and (Digest $path) -eq $expected
}
$work = Absolute $WorkDirectory
$cache = Absolute $CacheDirectory
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $work
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $cache -ReusableCache
foreach ($path in @($work, $cache, $output)) {
    & git -C $root check-ignore -q --no-index -- (Join-Path $path 'icon-output.probe')
    if ($LASTEXITCODE -ne 0) { throw 'Provider icon outputs must be Git-ignored' }
}
$sourceFiles = @(Get-ChildItem -LiteralPath $source -File -Filter '*.svg' | Sort-Object Name)
if ($sourceFiles.Count -eq 0) { throw 'No local provider SVG sources found' }

# Invalidate when either the artwork or the sampler changes, including its
# locked dependencies. Relative paths keep this cache portable across checkouts.
$inputFiles = @($sourceFiles.FullName)
$inputFiles += @('build/build-provider-icons.ps1', 'build/build-diagrams.ps1',
    'apps/diagram-renderer/Cargo.toml', 'apps/diagram-renderer/Cargo.lock', 'apps/diagram-renderer/build.rs') | ForEach-Object { Join-Path $root $_ }
$inputFiles += Get-ChildItem -LiteralPath (Join-Path $root 'apps/diagram-renderer/src') -Recurse -File -Filter '*.rs' | ForEach-Object { $_.FullName }
$inputLines = @($inputFiles | Sort-Object | ForEach-Object { $_.Substring($root.Length + 1).Replace('\', '/') + '=' + (Digest $_) }) -join "`n"
$hasher = [Security.Cryptography.SHA256]::Create()
try { $inputDigest = ([BitConverter]::ToString($hasher.ComputeHash([Text.Encoding]::UTF8.GetBytes($inputLines)))).Replace('-', '').ToLowerInvariant() } finally { $hasher.Dispose() }

function Read-CachedManifest {
    $stamp = Join-Path $cache 'inputs.sha256'
    $manifestPath = Join-Path $cache 'manifest.json'
    if (-not (Test-Path -LiteralPath $stamp -PathType Leaf) -or -not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) { return $null }
    if ([IO.File]::ReadAllText($stamp).Trim() -ne $inputDigest) { return $null }
    try { $candidate = Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8 | ConvertFrom-Json } catch {
        Write-Warning "Provider icon cache manifest is unreadable; rebuild from local SVGs: $manifestPath"
        return $null
    }
    if ($candidate.renderer -ne 'resvg 0.47.0' -or $candidate.size -ne 128 -or @($candidate.files).Count -ne $sourceFiles.Count) { return $null }
    for ($index = 0; $index -lt $sourceFiles.Count; $index++) {
        $entry = $candidate.files[$index]
        $svg = $sourceFiles[$index]
        if ($entry.name -ne $svg.BaseName -or $entry.sourceSHA256 -ne (Digest $svg.FullName)) { return $null }
        if (-not (MatchesDigest (Join-Path $cache ($svg.BaseName + '.png')) $entry.pngSHA256)) { return $null }
        if ($entry.name -eq 'kimi-color' -and $entry.lightForeground -ne '#1f1f1f') { return $null }
    }
    return $candidate
}

function Copy-IconAssets([string]$from, [string]$to, $manifest) {
    [IO.Directory]::CreateDirectory($to) | Out-Null
    foreach ($entry in $manifest.files) {
        $name = $entry.name + '.png'
        $destination = Join-Path $to $name
        if (-not (MatchesDigest $destination $entry.pngSHA256)) { Copy-Item -LiteralPath (Join-Path $from $name) -Destination $destination -Force }
    }
    $manifestPath = Join-Path $from 'manifest.json'
    $destination = Join-Path $to 'manifest.json'
    if (-not (MatchesDigest $destination (Digest $manifestPath))) { Copy-Item -LiteralPath $manifestPath -Destination $destination -Force }
}

$manifest = Read-CachedManifest
if ($null -ne $manifest) {
    Copy-IconAssets $cache $output $manifest
    Write-Output "Reused $($manifest.files.Count) verified provider icons from $cache"
    return
}

# Only a missing or stale cache invokes Cargo. Publish after every image was
# generated successfully; a failed rebuild must leave embedded assets intact.
Write-Output "Provider icon cache missing or stale; rendering local SVGs into $work"
& (Join-Path $PSScriptRoot 'build-diagrams.ps1') -IconsOnly -OutputDirectory $work -CargoExe $CargoExe -GoExe $GoExe -TargetDirectory $TargetDirectory
$entries = @($sourceFiles | ForEach-Object {
    $record = [ordered]@{ name = $_.BaseName; sourceSHA256 = Digest $_.FullName; pngSHA256 = Digest (Join-Path $work ($_.BaseName + '.png')) }
    if ($_.BaseName -eq 'kimi-color') { $record['lightForeground'] = '#1f1f1f' }
    $record
})
$manifest = [ordered]@{ renderer = 'resvg 0.47.0'; size = 128; files = $entries }
[IO.File]::WriteAllText((Join-Path $work 'manifest.json'), ($manifest | ConvertTo-Json -Depth 5), [Text.UTF8Encoding]::new($false))
Copy-IconAssets $work $cache $manifest
[IO.File]::WriteAllText((Join-Path $cache 'inputs.sha256'), $inputDigest, [Text.UTF8Encoding]::new($false))
Copy-IconAssets $cache $output $manifest
Write-Output "Prepared $($entries.Count) reusable provider icons in $cache"
