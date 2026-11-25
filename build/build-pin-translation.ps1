param([Parameter(Mandatory)][string]$OutputDirectory,
      [string]$OpenCVArchive=$env:WINONE_PIN_OPENCV_ARCHIVE)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$target=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $target
if (-not $OpenCVArchive) { $OpenCVArchive=Join-Path $root 'dist/resources/sources/opencv-4.12.0.tar.gz' }
$sha='44c106d5bb47efec04e531fd93008b3fcd1d27138985c5baf4eafac0e1ec9e9d'
if ((Get-FileHash -LiteralPath $OpenCVArchive).Hash -ne $sha) { throw 'OpenCV 4.12.0 source archive checksum mismatch' }
$vswhere=Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
$vs=& $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if (-not $vs) { throw 'Visual Studio C++ x64 tools are missing' }
$cmake=Join-Path $vs 'Common7/IDE/CommonExtensions/Microsoft/CMake/CMake/bin/cmake.exe'
$vsMajor=([version](& $vswhere -path $vs -property installationVersion)).Major
$generator=switch ($vsMajor) { 17 {'Visual Studio 17 2022'} 18 {'Visual Studio 18 2026'} default {throw "Unsupported Visual Studio generation $vsMajor"} }
$cache=Join-Path $root "dist/resources/cache/pin-translation-$($sha.Substring(0,12))-vs$vsMajor"
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $cache -ReusableCache
[void][IO.Directory]::CreateDirectory($cache)
# Serialize extraction and compilation across concurrent package builds.
$lockPath=Join-Path $cache 'build.lock'
while ($true) {
    try {
        $guard=[IO.File]::Open($lockPath,'OpenOrCreate','ReadWrite','None')
        break
    } catch [IO.IOException] {
        if (($_.Exception.HResult -band 65535) -notin @(32,33)) { throw }
        Start-Sleep -Milliseconds 250
    }
}
function Invoke-CMake([string[]]$Arguments,[string]$Failure) {
    $previous=$ErrorActionPreference
    try {
        $ErrorActionPreference='Continue'
        & $cmake @Arguments 2>&1 | ForEach-Object { $_.ToString() }
        $code=$LASTEXITCODE
    } finally { $ErrorActionPreference=$previous }
    if($code){throw $Failure}
}
try {
    $source=Join-Path $cache 'opencv-4.12.0'
    $ready=Join-Path $cache 'source.ready'
    if (-not (Test-Path -LiteralPath $ready)) {
        & tar -xf $OpenCVArchive -C $cache
        if ($LASTEXITCODE) { throw 'OpenCV source extraction failed' }
        [IO.File]::WriteAllText($ready,$sha)
    }
    $objects=Join-Path $cache 'build'
    Invoke-CMake @('-S',(Join-Path $root 'modules/capture/pin-translation'),'-B',$objects,'-G',$generator,'-A','x64',"-DCMAKE_GENERATOR_INSTANCE=$vs","-DOPENCV_SOURCE=$source",'-DCMAKE_CONFIGURATION_TYPES=Release') 'Pin translation configuration failed'
    Invoke-CMake @('--build',$objects,'--config','Release','--target','WinOne.PinTranslation','--parallel','4') 'Pin translation native build failed'
    [void][IO.Directory]::CreateDirectory($target)
    Copy-Item -LiteralPath (Join-Path $objects 'Release/WinOne.PinTranslation.dll') -Destination (Join-Path $target 'WinOne.PinTranslation.dll') -Force
} finally { $guard.Dispose() }
