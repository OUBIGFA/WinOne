param([Parameter(Mandatory)][string]$OutputDirectory,
      [Parameter(Mandatory)][string]$WorkDirectory,
      [string]$SourceDirectory,
      [string]$DependenciesDirectory,
      [string]$NodeExe='node',
      [string]$NativeOcrDotnetExe,
      [string]$CargoExe='cargo',
      [string]$OcrRuntimeDirectory,
      [string]$VcpkgToolchain,
      [string]$NativeBuildDirectory,
      [string]$RecordingRuntimeDirectory,
      [switch]$RunTests,
      [switch]$BuildProbe)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
function Absolute([string]$value){if([IO.Path]::IsPathRooted($value)){[IO.Path]::GetFullPath($value)}else{[IO.Path]::GetFullPath((Join-Path $root $value))}}
$output=Absolute $OutputDirectory;$work=Absolute $WorkDirectory
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $work
if(Test-Path -LiteralPath (Join-Path $output 'coreclr.dll')){throw 'Choose a fresh capture output; old .NET payload is still present'}
if(-not $SourceDirectory){$SourceDirectory=Join-Path $root 'dist/resources/sources/Tools3000-v1.4.11'}
if(-not $DependenciesDirectory){$DependenciesDirectory=Join-Path $root 'dist/resources/cache/capture-native/installed'}
$source=Absolute $SourceDirectory;$dependencies=Absolute $DependenciesDirectory
$vswhere=Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
$vs=& $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if(-not $vs){throw 'Native capture needs the installed Windows C++ compiler'}
$cmake=Join-Path $vs 'Common7/IDE/CommonExtensions/Microsoft/CMake/CMake/bin/cmake.exe'
if(-not $VcpkgToolchain){$VcpkgToolchain=Join-Path $vs 'VC/vcpkg/scripts/buildsystems/vcpkg.cmake'}
$major=([version](& $vswhere -path $vs -property installationVersion)).Major
$generator=switch($major){17{'Visual Studio 17 2022'}18{'Visual Studio 18 2026'}default{throw 'Unsupported native compiler generation'}}
New-Item -ItemType Directory -Force -Path $output,$work|Out-Null
$adapted=Join-Path $work 'adapted'
& $NodeExe (Join-Path $PSScriptRoot 'prepare-capture-native.mjs') $source $adapted
if($LASTEXITCODE){throw 'Native community source verification failed'}
$objects=if($NativeBuildDirectory){Absolute $NativeBuildDirectory}else{Join-Path $work 'objects'}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $objects
$probeOption=if($BuildProbe -or $RunTests){'ON'}else{'OFF'}
& $cmake -S (Join-Path $root 'modules/capture-native') -B $objects -G $generator -A x64 `
    "-DCMAKE_GENERATOR_INSTANCE=$vs" "-DCMAKE_TOOLCHAIN_FILE=$VcpkgToolchain" '-DVCPKG_MANIFEST_MODE=OFF' `
    '-DVCPKG_TARGET_TRIPLET=x64-windows-winone' "-DVCPKG_INSTALLED_DIR=$($dependencies.Replace('\','/'))" "-DWINONE_TOOLS3000_SOURCE=$($adapted.Replace('\','/'))" "-DWINONE_BUILD_CAPTURE_PROBE=$probeOption"
if($LASTEXITCODE){throw 'Native capture configuration failed'}
$targets=@('WinOne.Capture')
if($BuildProbe){$targets+='WinOne.NativeCapture.Probe'}
if($RunTests){$targets+=@('WinOne.Capture.DraftTests','WinOne.Capture.CommunityTests','WinOne.Capture.RecordingTests')}
& $cmake --build $objects --config Release --target @targets --parallel 4
if($LASTEXITCODE){throw 'Native capture compilation failed'}
$compiled=Join-Path $objects 'Release'
# The app-local dependency copier follows the executable import closure.
# Development libraries, test binaries and symbols are never product payload.
Copy-Item -LiteralPath (Join-Path $compiled 'WinOne.Capture.exe') -Destination (Join-Path $output 'WinOne.Capture.exe') -Force
Get-ChildItem -LiteralPath $compiled -Filter '*.dll' -File | ForEach-Object {Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $output $_.Name) -Force}
$ocrOutput=Join-Path $work 'ocr'
$ocrParameters=@{OutputDirectory=$ocrOutput;RunTests=$RunTests}
if($NativeOcrDotnetExe){$ocrParameters.DotnetExe=$NativeOcrDotnetExe}
& (Join-Path $PSScriptRoot 'build-capture-ocr-native.ps1') @ocrParameters
foreach($name in @('WinOne.Ocr.exe','WinOne.NativeImage.dll')){Copy-Item -LiteralPath (Join-Path $ocrOutput $name) -Destination (Join-Path $output $name) -Force}
if($RecordingRuntimeDirectory){
    $recording=Absolute $RecordingRuntimeDirectory
    $record=Get-Content -LiteralPath (Join-Path $recording 'recording-runtime.json') -Raw -Encoding utf8|ConvertFrom-Json
    if((Get-FileHash -LiteralPath (Join-Path $recording 'WinOne.Recording.dll') -Algorithm SHA256).Hash.ToLowerInvariant() -ne $record.sha256){throw 'Existing recording runtime checksum differs'}
    foreach($name in @('WinOne.Recording.dll','recording-runtime.json')){Copy-Item -LiteralPath (Join-Path $recording $name) -Destination (Join-Path $output $name) -Force}
}else{
    & (Join-Path $PSScriptRoot 'build-recording.ps1') -OutputDirectory $output -CargoExe $CargoExe -SkipTests:(-not $RunTests)
}
if(-not $OcrRuntimeDirectory){$OcrRuntimeDirectory=Join-Path $root 'dist/resources/runtimes/rapidocrlite-5.0.0a16'}
& (Join-Path $PSScriptRoot 'copy-ocr.ps1') -RuntimeDirectory $OcrRuntimeDirectory -CaptureDirectory $output
# Existing CRT DLLs are copied from the redistributable directory belonging to
# this compiler. The final dependency scan verifies that no non-system DLL is missing.
$redist=Get-ChildItem -LiteralPath (Join-Path $vs 'VC/Redist/MSVC') -Directory | Where-Object {$_.Name -match '^\d+\.'} | Sort-Object Name -Descending | Select-Object -First 1
$crt=Join-Path $redist.FullName 'x64/Microsoft.VC143.CRT'
if(-not(Test-Path -LiteralPath $crt)){$crt=Join-Path $redist.FullName 'x64/Microsoft.VC145.CRT'}
if(-not(Test-Path -LiteralPath $crt)){throw 'Native compiler redistributable directory is missing'}
foreach($name in @('vcruntime140.dll','vcruntime140_1.dll','msvcp140.dll','msvcp140_1.dll','msvcp140_2.dll','msvcp140_atomic_wait.dll','msvcp140_codecvt_ids.dll')){
    $file=Join-Path $crt $name
    if(Test-Path -LiteralPath $file){Copy-Item -LiteralPath $file -Destination (Join-Path $output $name) -Force}
}
if($BuildProbe){
    foreach($file in Get-ChildItem -LiteralPath $output -File | Where-Object {$_.Extension -eq '.dll' -or $_.Name -eq 'WinOne.Ocr.exe'}){
        Copy-Item -LiteralPath $file.FullName -Destination (Join-Path $compiled $file.Name) -Force
    }
    & (Join-Path $PSScriptRoot 'copy-ocr.ps1') -RuntimeDirectory $OcrRuntimeDirectory -CaptureDirectory $compiled
}
if($RunTests){
    & (Join-Path $compiled 'WinOne.Capture.DraftTests.exe') (Join-Path $work ('draft-tests-'+[guid]::NewGuid().ToString('N')))
    if($LASTEXITCODE){throw 'Native draft tests failed'}
    & (Join-Path $compiled 'WinOne.Capture.CommunityTests.exe') (Join-Path $work ('number-'+[guid]::NewGuid().ToString('N')+'.png'))
    if($LASTEXITCODE){throw 'Native community behavior tests failed'}
    & (Join-Path $compiled 'WinOne.Capture.RecordingTests.exe') $output
    if($LASTEXITCODE){throw 'Native recording ABI checks failed'}
}
$native=@{schemaVersion=1;implementation='Tools3000 C++ + NativeAOT OCR + existing Rust recording';version='v1.4.11';sourceCommit='f0e968ba937e7e3df9f3f5a8d627be8b19e398dd';ocrCompiler='10.0.404';ocrRuntime='10.0.15'}
$native|ConvertTo-Json|Set-Content -LiteralPath (Join-Path $output 'native-runtime.json') -Encoding utf8
& (Join-Path $PSScriptRoot 'verify-capture-native.ps1') -CaptureDirectory $output -Report (Join-Path $work 'native-runtime-verification.json')
