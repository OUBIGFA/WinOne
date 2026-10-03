[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Package,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [string]$GoExe='go'
)
$ErrorActionPreference='Stop'
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$packagePath=[IO.Path]::GetFullPath($Package)
$output=[IO.Path]::GetFullPath($OutputDirectory)
if(Test-Path -LiteralPath $output){throw 'Installer preparation directory already exists'}
& git -C $root check-ignore --quiet --no-index -- (Join-Path $output 'installer.probe')
if($LASTEXITCODE -ne 0){throw 'Installer preparation must use a Git-ignored directory'}
[IO.Directory]::CreateDirectory($output)|Out-Null
Push-Location $root
try{
    & $GoExe run ./build/package --verify $packagePath --nsis-files (Join-Path $output 'files.nsh')
    if($LASTEXITCODE -ne 0){throw 'Package verification failed'}
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [IO.Compression.ZipFile]::ExtractToDirectory($packagePath,(Join-Path $output 'payload'))
}finally{Pop-Location}
Write-Output "Installer payload prepared: $output"
