[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$CaptureExecutable,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [string]$GoExe = 'go',
    [ValidateRange(1,100)][int]$Iterations = 5,
    [ValidateRange(0,100)][int]$WarmupIterations = 0,
    [switch]$DisableTieredCompilation,
    [switch]$HighQoS
)
$ErrorActionPreference = 'Stop'
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$output = [IO.Path]::GetFullPath($OutputDirectory)
$executable = (Resolve-Path -LiteralPath $CaptureExecutable).Path
if ($output.StartsWith($workspace+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) {
    & git -C $workspace check-ignore --quiet -- (Join-Path $output 'report.json')
    if ($LASTEXITCODE -ne 0) { throw 'Benchmark output inside the repository must be Git-ignored' }
}
if (Test-Path -LiteralPath $output) { throw 'Choose a new output directory to preserve existing reports' }
[IO.Directory]::CreateDirectory($output) | Out-Null
$temporary = Join-Path $output 'tmp'
[IO.Directory]::CreateDirectory($temporary) | Out-Null
$system = Get-CimInstance Win32_OperatingSystem
$computer = Get-CimInstance Win32_ComputerSystem
$processor = Get-CimInstance Win32_Processor | Select-Object -First 1
$environment = @{timestampUtc=[datetime]::UtcNow.ToString('o');os=$system.Caption;version=$system.Version;build=$system.BuildNumber;cpu=$processor.Name;logicalProcessors=$computer.NumberOfLogicalProcessors;physicalMemoryBytes=$computer.TotalPhysicalMemory}
[IO.File]::WriteAllText((Join-Path $output 'environment.json'),($environment | ConvertTo-Json),[Text.UTF8Encoding]::new($false))
$previousExecutable=$env:WINONE_CAPTURE_EXE
$previousOutput=$env:WINONE_CAPTURE_BENCHMARK_OUTPUT
$previousTemporary=$env:TMP
$previousIterations=$env:WINONE_CAPTURE_BENCHMARK_ITERATIONS
$previousTiering=$env:WINONE_CAPTURE_BENCHMARK_DISABLE_TIERING
$previousQoS=$env:WINONE_CAPTURE_BENCHMARK_HIGH_QOS
$previousWarmup=$env:WINONE_CAPTURE_BENCHMARK_WARMUP
Push-Location $workspace
try {
    $env:WINONE_CAPTURE_EXE=$executable
    $env:WINONE_CAPTURE_BENCHMARK_OUTPUT=Join-Path $output 'report.json'
    $env:TMP=$temporary
    $env:WINONE_CAPTURE_BENCHMARK_ITERATIONS=[string]$Iterations
    $env:WINONE_CAPTURE_BENCHMARK_DISABLE_TIERING=if($DisableTieredCompilation){'1'}else{'0'}
    $env:WINONE_CAPTURE_BENCHMARK_HIGH_QOS=if($HighQoS){'1'}else{'0'}
    $env:WINONE_CAPTURE_BENCHMARK_WARMUP=[string]$WarmupIterations
    & $GoExe test ./tests/integration -run '^TestCapture(MemoryProbeReadsLiveProcess|PreviewResourceBenchmark)$' -count=1 -timeout 8m -v
    if ($LASTEXITCODE -ne 0) { throw "Capture preview benchmark failed: $LASTEXITCODE" }
    Write-Output "Preview resource report: $env:WINONE_CAPTURE_BENCHMARK_OUTPUT"
} finally {
    $env:WINONE_CAPTURE_EXE=$previousExecutable
    $env:WINONE_CAPTURE_BENCHMARK_OUTPUT=$previousOutput
    $env:TMP=$previousTemporary
    $env:WINONE_CAPTURE_BENCHMARK_ITERATIONS=$previousIterations
    $env:WINONE_CAPTURE_BENCHMARK_DISABLE_TIERING=$previousTiering
    $env:WINONE_CAPTURE_BENCHMARK_HIGH_QOS=$previousQoS
    $env:WINONE_CAPTURE_BENCHMARK_WARMUP=$previousWarmup
    Pop-Location
}
