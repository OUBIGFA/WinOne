[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$CargoExe = 'cargo',
    [string]$GoExe = 'go',
    [string]$DotnetExe = 'dotnet',
    [string]$TargetDirectory = 'dist/resources/cache/rust-selection',
    [switch]$NativeOnly
)
# Explicit foreground tests use only generated text in their own window. This
# prototype does not register hooks or enter the product's default build.
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
function Absolute([string]$value) { if ([IO.Path]::IsPathRooted($value)) { [IO.Path]::GetFullPath($value) } else { [IO.Path]::GetFullPath((Join-Path $root $value)) } }
$output = Absolute $OutputDirectory
$readerDirectory = Join-Path (Split-Path -Parent $output) ((Split-Path -Leaf $output) + '-reader')
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$target = Absolute $TargetDirectory
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $target -ReusableCache
if (Test-Path -LiteralPath $output) { throw 'Choose a new output directory' }
foreach ($path in @($output, $target)) {
    & git -C $root check-ignore -q --no-index -- (Join-Path $path 'probe-output.json')
    if ($LASTEXITCODE -ne 0) { throw 'Selection probe outputs must be ignored' }
}
$cargo = (Get-Command $CargoExe -ErrorAction Stop).Source
$savedRustc = $env:RUSTC
$savedNativeOnly = $env:WINONE_SELECTION_NATIVE_ONLY
try {
    $env:RUSTC = Join-Path (Split-Path $cargo -Parent) 'rustc.exe'
    $env:WINONE_SELECTION_NATIVE_ONLY = if ($NativeOnly) { '1' } else { '0' }
    # Cargo uses stderr for normal progress. Exit codes define success.
    $ErrorActionPreference = 'Continue'
    & (Join-Path $root 'build/build-selection.ps1') -CargoExe $cargo -GoExe $GoExe -TargetDirectory $target -OutputDirectory $readerDirectory
    & $DotnetExe build (Join-Path $root 'tests/selection-probe/SelectionProbe.csproj') -c Release -p:RestoreLockedMode=true --nologo -v:q
    if ($LASTEXITCODE -ne 0) { throw 'Selection fixture build failed' }
    & (Join-Path $root 'tests/selection-probe/bin/Release/net10.0-windows/WinOne.Selection.Probe.exe') --reader (Join-Path $readerDirectory 'WinOne.Selection.exe') --output $output
    if ($LASTEXITCODE -ne 0) { throw 'Selection reader integration failed; inspect the preserved output' }
} finally {
    $env:RUSTC = $savedRustc
    $env:WINONE_SELECTION_NATIVE_ONLY = $savedNativeOnly
}
