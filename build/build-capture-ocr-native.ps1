param([Parameter(Mandatory)][string]$OutputDirectory,
      [string]$DotnetExe,
      [switch]$RunTests)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(-not $DotnetExe){$DotnetExe=Join-Path $root 'dist/resources/tools/dotnet-10.0.404/dotnet.exe'}
$dotnet=(Get-Command $DotnetExe -ErrorAction Stop).Source
$vswhere=Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
$vs=& $vswhere -latest -prerelease -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if(-not $vs){throw 'Native OCR needs the installed Windows C++ compiler'}
$cmake=Join-Path $vs 'Common7/IDE/CommonExtensions/Microsoft/CMake/CMake/bin/cmake.exe'
$cache=Join-Path $root 'dist/resources/cache/capture-native-imaging'
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $cache -ReusableCache
$major=([version](& $vswhere -path $vs -property installationVersion)).Major
$generator=switch($major){17{'Visual Studio 17 2022'}18{'Visual Studio 18 2026'}default{throw 'Unsupported native compiler generation'}}
$priorNuget=$env:NUGET_PACKAGES
$priorTelemetry=$env:DOTNET_CLI_TELEMETRY_OPTOUT
try {
    $env:NUGET_PACKAGES=Join-Path $root 'dist/resources/cache/nuget-native-capture'
    $env:DOTNET_CLI_TELEMETRY_OPTOUT='1'
    & $cmake -S (Join-Path $root 'modules/capture-native/imaging') -B $cache -G $generator -A x64 "-DCMAKE_GENERATOR_INSTANCE=$vs"
    if($LASTEXITCODE){throw 'Native image configuration failed'}
    & $cmake --build $cache --config Release --parallel 4
    if($LASTEXITCODE){throw 'Native image build failed'}
    Push-Location (Join-Path $root 'modules/capture-ocr-native')
    try {
        if((& $dotnet --version) -ne '10.0.404'){throw 'Use the pinned .NET 10.0.404 SDK for native OCR'}
        & $dotnet publish WinOne.Ocr.csproj -c Release -r win-x64 -o $output --nologo
        if($LASTEXITCODE){throw 'Native OCR compilation failed'}
    } finally {Pop-Location}
    Copy-Item -LiteralPath (Join-Path $cache 'Release/WinOne.NativeImage.dll') -Destination (Join-Path $output 'WinOne.NativeImage.dll') -Force
    if($RunTests){
        $testOutput=Join-Path $output 'verification/native-ocr'
        Push-Location (Join-Path $root 'tests/capture-ocr-native')
        try {
            & $dotnet publish NativeOcrTests.csproj -c Release -r win-x64 -o $testOutput --nologo
            if($LASTEXITCODE){throw 'Native OCR test compilation failed'}
        } finally {Pop-Location}
        Copy-Item -LiteralPath (Join-Path $cache 'Release/WinOne.NativeImage.dll') -Destination (Join-Path $testOutput 'WinOne.NativeImage.dll') -Force
        & (Join-Path $testOutput 'NativeOcrTests.exe')
        if($LASTEXITCODE){throw 'Native OCR behavior tests failed'}
    }
} finally {$env:NUGET_PACKAGES=$priorNuget;$env:DOTNET_CLI_TELEMETRY_OPTOUT=$priorTelemetry}
