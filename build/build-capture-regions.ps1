param([string]$MSBuild, [string]$PlatformToolset, [switch]$TestLifetime, [string]$OutputDirectory = 'dist/build/capture')
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$output = if ([IO.Path]::IsPathRooted($OutputDirectory)) { [IO.Path]::GetFullPath($OutputDirectory) } else { [IO.Path]::GetFullPath((Join-Path $root $OutputDirectory)) }
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'regions-output.probe')
if ($LASTEXITCODE -ne 0) { throw 'OutputDirectory must be Git-ignored' }
if (-not $MSBuild) {
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
    $installation = & $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
    if (-not $installation) { throw 'Visual Studio C++ build tools and Windows SDK are required' }
    $MSBuild = Join-Path $installation 'MSBuild/Current/Bin/MSBuild.exe'
}
if (-not $PlatformToolset) {
    $msbuildRoot = Split-Path (Split-Path (Split-Path $MSBuild -Parent) -Parent) -Parent
    $toolsets = @(Get-ChildItem (Join-Path $msbuildRoot 'Microsoft/VC/*/Platforms/x64/PlatformToolsets/v*') -Directory | Where-Object Name -Match '^v[0-9]+$' | Sort-Object { [int]$_.Name.Substring(1) } -Descending)
    if ($toolsets.Count -eq 0) { throw 'No installed x64 C++ platform toolset was found; specify PlatformToolset' }
    $PlatformToolset = $toolsets[0].Name
}
$intermediate = Join-Path $root '_temp/build/capture-regions'
if ($TestLifetime) { $intermediate = Join-Path $root '_temp/build/capture-regions-lifetime' }
[IO.Directory]::CreateDirectory($output) | Out-Null
[IO.Directory]::CreateDirectory($intermediate) | Out-Null
& $MSBuild (Join-Path $root 'modules/capture/native/Regions.vcxproj') /t:Build /p:Configuration=Release /p:Platform=x64 "/p:LifetimeProbe=$($TestLifetime.IsPresent.ToString().ToLowerInvariant())" "/p:PlatformToolset=$PlatformToolset" "/p:OutDir=$($output.Replace('\','/'))/" "/p:IntDir=$($intermediate.Replace('\','/'))/" /verbosity:minimal /nologo
if ($LASTEXITCODE -ne 0) { throw 'Capture region helper build failed' }
