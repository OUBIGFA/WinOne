[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$BundleDirectory,
    [string]$OutputDirectory = '_temp/package-build/output',
    [string]$GoExe = 'go'
)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$bundle = [IO.Path]::GetFullPath($BundleDirectory)
$output = [IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore --quiet --no-index -- (Join-Path $output 'package-output.probe')
if ($LASTEXITCODE -ne 0) { throw 'Package output must be Git-ignored' }
Push-Location $root
try {
    & $GoExe run ./build/package --bundle $bundle --license (Join-Path $root 'LICENSE') --output $output
    if ($LASTEXITCODE -ne 0) { throw 'Package verification failed; incomplete artifacts must not be distributed' }
} finally { Pop-Location }
