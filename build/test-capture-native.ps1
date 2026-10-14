param(
    [Parameter(Mandatory)][string]$ShareXSourceDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$DotnetExe = 'dotnet',
    [ValidateRange(1,100)][int]$RetakeIterations = 1,
    [ValidateSet('workflow','scroll-workflow','autosave','toolbar','facade','pointer','text-recognition')][string[]]$Suites = @('workflow','scroll-workflow')
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$source=(Resolve-Path -LiteralPath $ShareXSourceDirectory).Path
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if (Test-Path -LiteralPath $output) { throw 'Choose a new output directory' }
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'probe.json')
if ($LASTEXITCODE -ne 0) { throw 'Test output must be Git-ignored' }
if ($source -eq (Join-Path $root 'dist/resources/sources/ShareX-21.0.0')) { throw 'Use an isolated ShareX copy for native probes' }
. (Join-Path $PSScriptRoot 'sharex-native-host.ps1')
Initialize-ShareXNativeHost $source
New-Item -ItemType Directory -Path $output | Out-Null
& $DotnetExe build (Join-Path $root 'tests/capture-host-probe/NativeCaptureProbe.csproj') -c Release "-p:ShareXSourceDirectory=$source" --nologo -v:q
if ($LASTEXITCODE -ne 0) { throw 'Native capture probe build failed' }
$savedIterations=$env:WINONE_RETAKE_ITERATIONS
try {
    $env:WINONE_RETAKE_ITERATIONS=[string]$RetakeIterations
    foreach ($suite in $Suites) {
        & (Join-Path $root 'tests/capture-host-probe/run-private.ps1') -Suite $suite -OutputDirectory (Join-Path $output $suite)
        if ($LASTEXITCODE -ne 0) { throw "Native capture suite failed: $suite" }
    }
} finally {$env:WINONE_RETAKE_ITERATIONS=$savedIterations}
