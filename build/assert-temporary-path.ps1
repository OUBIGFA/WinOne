param([Parameter(Mandatory)][string]$Path, [switch]$ReusableCache)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$fullPath = if ([IO.Path]::IsPathRooted($Path)) { [IO.Path]::GetFullPath($Path) } else { [IO.Path]::GetFullPath((Join-Path $root $Path)) }
$temporaryRoot = Join-Path $root '_temp'
$prefix = $temporaryRoot + [IO.Path]::DirectorySeparatorChar
$resourceRoot = Join-Path $root 'dist'
$isReusableCache = $ReusableCache -and $fullPath.StartsWith($resourceRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -and $fullPath.TrimEnd('\', '/') -ne $resourceRoot

# This validates a writer's intended output, not the contents of dist. Reusable
# caches may use a chosen fixed subdirectory without registering a name here.
if ((-not $fullPath.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase) -or $fullPath.TrimEnd('\', '/') -eq $temporaryRoot) -and -not $isReusableCache) {
    throw "Temporary output must use the repository's _temp/<task>/ directory: $Path"
}
# Do not let a junction turn an otherwise valid temporary path into a write to
# dist, source files, or an installation outside this checkout.
$ancestor = $fullPath
while ($ancestor -and $ancestor -ne $root) {
    if (Test-Path -LiteralPath $ancestor) {
        $entry = Get-Item -LiteralPath $ancestor -Force
        if ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) {
            throw "Temporary output must not traverse a linked directory: $Path"
        }
    }
    $ancestor = Split-Path -Parent $ancestor
}
