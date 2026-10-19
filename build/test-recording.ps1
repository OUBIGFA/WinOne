param(
    [Parameter(Mandatory)][string]$ShareXSourceDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$CargoExe='cargo',
    [string]$DotnetExe='dotnet',
    [switch]$RunSystemChecks
)
$ErrorActionPreference='Stop'
function Invoke-RecordingCheck([string]$Executable,[string[]]$Arguments,[string]$Failure){
    # Preserve the complete compiler/test diagnostic on Windows PowerShell 5;
    # stderr alone must not abort the pipeline before its exit code is read.
    $previous=$ErrorActionPreference
    try{$ErrorActionPreference='Continue';& $Executable @Arguments 2>&1 | ForEach-Object {$_.ToString()};$code=$LASTEXITCODE}finally{$ErrorActionPreference=$previous}
    if($code -ne 0){throw "$Failure (exit $code)"}
}
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath $output){throw 'Choose a new recording test output directory'}
New-Item -ItemType Directory -Path $output | Out-Null
$native=Join-Path $output 'native';$runner=Join-Path $output 'runner'
& (Join-Path $PSScriptRoot 'build-recording.ps1') -CargoExe $CargoExe -OutputDirectory $native
& (Join-Path $root 'dist/resources/cache/rust-recording/release/examples/encode_probe.exe') (Join-Path $output 'synthetic')
if($LASTEXITCODE -ne 0){throw 'Synthetic MP4/GIF encode/decode verification failed'}
. (Join-Path $PSScriptRoot 'sharex-native-host.ps1')
Initialize-ShareXNativeHost (Resolve-Path -LiteralPath $ShareXSourceDirectory).Path
Invoke-RecordingCheck $DotnetExe @('publish',(Join-Path $root 'tests/recording/RecordingTests.csproj'),'-c','Release',"-p:ShareXSourceDirectory=$((Resolve-Path -LiteralPath $ShareXSourceDirectory).Path)",'-o',$runner,'--nologo','-v:q') 'Recording UI test build failed'
Copy-Item -LiteralPath (Join-Path $native 'WinOne.Recording.dll') -Destination $runner -Force
Invoke-RecordingCheck $DotnetExe @((Join-Path $runner 'WinOne.Recording.Tests.dll'),(Join-Path $output 'unit')) 'Recording selection / toolbar / ABI tests failed'
if($RunSystemChecks){
    # Opt-in: the test paints and records only an opaque, repository-owned
    # window. No real WinOne profile, autostart entry or user application changes.
    Invoke-RecordingCheck $DotnetExe @((Join-Path $runner 'WinOne.Recording.Tests.dll'),(Join-Path $output 'live'),'--live') 'WGC recording lifecycle tests failed'
    # File creation alone does not prove capture correctness. Decode the real
    # panel and direct-WGC clips before this entry point reports success.
    Invoke-RecordingCheck 'node' @((Join-Path $PSScriptRoot 'check-recording-media.mjs'),(Join-Path $output 'synthetic'),(Join-Path $output 'live/clips'),(Join-Path $output 'decoded'),'--panel') 'Recording content, outline exclusion or duration verification failed'
}
Write-Output "Recording verification passed: $output (system checks: $RunSystemChecks)"
