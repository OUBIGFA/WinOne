[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][int[]]$RootProcessIds,
    [Parameter(Mandatory=$true)][string]$OutputPath,
    [string]$Scenario = 'manual',
    [ValidateRange(2,3600)][int]$DurationSeconds = 30,
    [ValidateRange(250,10000)][int]$IntervalMilliseconds = 1000,
    [string[]]$BinaryPaths = @(),
    [long]$WindowHandle = 0,
    [int]$WindowProcessId = 0
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Import-Module (Join-Path $PSScriptRoot 'resource-metrics.psm1') -Force
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$destination = [IO.Path]::GetFullPath($OutputPath)
if (Test-Path -LiteralPath $destination) { throw 'Report already exists; choose a new output path to preserve the previous baseline' }
if ($destination.StartsWith($workspace + [IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) {
    & git -C $workspace check-ignore --quiet -- $destination
    if ($LASTEXITCODE -ne 0) { throw 'Resource reports inside the repository must be Git-ignored' }
}
$inventory = @(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
$known = @{}
$roots = @()
foreach ($rootId in ($RootProcessIds | Select-Object -Unique)) {
    $matches = @($inventory | Where-Object ProcessId -eq $rootId)
    if ($matches.Count -ne 1 -or $null -eq $matches[0].CreationDate) { throw "Root process $rootId is missing or cannot be identified" }
    $entry = $matches[0]
    $known[[string]$rootId] = [string]$entry.CreationDate.ToUniversalTime().Ticks
    $roots += [pscustomobject]@{ProcessId=$rootId;Name=$entry.Name;CreatedUtc=$entry.CreationDate.ToUniversalTime().ToString('o')}
}
if ($known.Count -eq 0) { throw 'At least one live root process is required' }
if ($WindowHandle -ne 0 -and $WindowProcessId -notin $RootProcessIds) { throw 'Window context must belong to a measured root process' }
$computer = Get-CimInstance Win32_ComputerSystem
$system = Get-CimInstance Win32_OperatingSystem
$artifacts = @()
foreach ($binary in $BinaryPaths) {
    $file = Get-Item -LiteralPath $binary
    $artifacts += [pscustomobject]@{Name=$file.Name;Bytes=$file.Length;Sha256=(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
}
$samples = [System.Collections.Generic.List[object]]::new()
$previous = @{}
$timer = [Diagnostics.Stopwatch]::StartNew()
$lastElapsed = 0.0
do {
    $started = $timer.Elapsed.TotalMilliseconds
    $inventory = @(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,CreationDate)
    $family = Get-TrackedProcessFamily -Inventory $inventory -Known $known
    $counters = @{}
    Get-CimInstance Win32_PerfRawData_PerfProc_Process | ForEach-Object { $counters[[string]$_.IDProcess] = $_ }
    $sample = Get-ProcessResourceRows -Family $family -Counters $counters -Previous $previous -LogicalProcessors $computer.NumberOfLogicalProcessors
    $windowContext = $null
    $contextErrors = @()
    if ($WindowHandle -ne 0) {
        try {
            if ($WindowProcessId -notin @($sample.Processes.ProcessId)) { throw 'Window owner identity was not verified' }
            $windowContext = Get-WindowResourceContext -WindowHandle $WindowHandle -ProcessId $WindowProcessId
        } catch { $contextErrors += $_.Exception.Message }
    }
    $elapsed = $timer.Elapsed.TotalSeconds
    $samples.Add([pscustomobject]@{
        ElapsedSeconds=$elapsed;IntervalSeconds=($elapsed-$lastElapsed);CapturedUtc=[datetime]::UtcNow.ToString('o')
        Complete=($sample.Complete -and $contextErrors.Count -eq 0);Processes=$sample.Processes;Errors=@($sample.Errors)+$contextErrors;Totals=$sample.Totals;Window=$windowContext
    })
    $lastElapsed = $elapsed
    $remaining = $IntervalMilliseconds - ($timer.Elapsed.TotalMilliseconds-$started)
    if ($remaining -gt 0 -and $timer.Elapsed.TotalSeconds -lt $DurationSeconds) { Start-Sleep -Milliseconds ([int]$remaining) }
} while ($timer.Elapsed.TotalSeconds -lt $DurationSeconds)
$timer.Stop()
$complete = @($samples | Where-Object Complete)
$cpuSamples = @($complete | Where-Object { $null -ne $_.Totals.CpuPercent })
$cpuWeighted = 0.0
$cpuSeconds = 0.0
foreach ($sample in $cpuSamples) { $cpuWeighted += $sample.Totals.CpuPercent * $sample.IntervalSeconds; $cpuSeconds += $sample.IntervalSeconds }
$averageCpu = $null
if ($cpuSeconds -gt 0) { $averageCpu = $cpuWeighted / $cpuSeconds }
$maxWorking = $null
$maxPrivate = $null
if ($complete.Count -gt 0) {
    $maxWorking = ($complete | ForEach-Object { $_.Totals.PrivateWorkingSetBytes } | Measure-Object -Maximum).Maximum / 1MB
    $maxPrivate = ($complete | ForEach-Object { $_.Totals.PrivateBytes } | Measure-Object -Maximum).Maximum / 1MB
}
$report = [pscustomobject]@{
    SchemaVersion=1;Scenario=$Scenario;CapturedUtc=[datetime]::UtcNow.ToString('o')
    DurationSeconds=$timer.Elapsed.TotalSeconds;RequestedIntervalMilliseconds=$IntervalMilliseconds
    CpuNormalization='Percent of total logical CPU capacity; first or incomplete intervals are null'
    Scope='Explicit root identities and observed descendants; pass detached/elevated workers as additional roots'
    Hardware=[pscustomobject]@{LogicalProcessors=$computer.NumberOfLogicalProcessors;PhysicalMemoryBytes=$computer.TotalPhysicalMemory;ProcessorNames=@(Get-CimInstance Win32_Processor | Select-Object -ExpandProperty Name);WindowsVersion=$system.Version;WindowsBuild=$system.BuildNumber}
    Binaries=$artifacts;Roots=$roots
    Summary=[pscustomobject]@{Samples=$samples.Count;CompleteSamples=$complete.Count;CpuSamples=$cpuSamples.Count;AverageCpuPercent=$averageCpu;SampledMaxPrivateWorkingSetMiB=$maxWorking;SampledMaxPrivateBytesMiB=$maxPrivate}
    Samples=@($samples)
}
[IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($destination)) | Out-Null
[IO.File]::WriteAllText($destination,($report | ConvertTo-Json -Depth 12),[Text.UTF8Encoding]::new($false))
Write-Output "Saved resource report: $destination"
$report.Summary | Format-List
