param(
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$WorkDirectory,
    [string]$SourceDirectory,
    [string]$DependenciesDirectory,
    [string]$NativeOcrDotnetExe,
    [string]$CargoExe='cargo',
    [string]$NodeExe='node',
    [string]$OcrRuntimeDirectory=$env:WINONE_OCR_RUNTIME_DIRECTORY,
    [switch]$RunTests,
    [switch]$BuildProbe
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
if(-not $WorkDirectory){$WorkDirectory=Join-Path (Split-Path -Parent $output) 'capture-native'}
$parameters=@{OutputDirectory=$output;WorkDirectory=$WorkDirectory;CargoExe=$CargoExe;NodeExe=$NodeExe;RunTests=$RunTests;BuildProbe=$BuildProbe}
foreach($key in @('SourceDirectory','DependenciesDirectory','NativeOcrDotnetExe','OcrRuntimeDirectory')){
    $value=Get-Variable -Name $key -ValueOnly
    if($value){$parameters[$key]=$value}
}
& (Join-Path $PSScriptRoot 'build-capture-native.ps1') @parameters
