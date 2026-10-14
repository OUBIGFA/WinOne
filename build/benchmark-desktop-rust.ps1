[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$HostExe,
    [Parameter(Mandatory=$true)][string]$DesktopExe,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [ValidateRange(2,30)][int]$Runs = 5,
    [ValidateRange(2,300)][int]$DurationSeconds = 10,
    [switch]$LogView
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = Split-Path -Parent $PSScriptRoot
$output = [IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'probe.json')
if ($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)) { throw 'Choose a new Git-ignored output directory' }
$HostExe = (Resolve-Path -LiteralPath $HostExe).Path
$DesktopExe = (Resolve-Path -LiteralPath $DesktopExe).Path
New-Item -ItemType Directory -Path $output | Out-Null
if (-not ('RustDesktopProbe' -as [type])) { Add-Type -Path (Join-Path $PSScriptRoot 'desktop-rust-probe.cs') }
$pipe = '\\.\pipe\WinOne-rust-test-' + [guid]::NewGuid().ToString('N')
$hostProcess = $null
$probe = $null
$samples = [Collections.Generic.List[object]]::new()
$savedLocal = $env:LOCALAPPDATA
$env:LOCALAPPDATA = Join-Path $output 'local-appdata'
try {
    $logText = ''
    if ($LogView) {
        $logDirectory = Join-Path $output 'profile/modules/capture'
        New-Item -ItemType Directory -Path $logDirectory -Force | Out-Null
        $logText = (([string][char]0x8FD0 + [char]0x884C) * 4000) + ('x' * 40000) + 'LOG-END'
        [IO.File]::WriteAllText((Join-Path $logDirectory 'worker.log'), $logText, [Text.UTF8Encoding]::new($false))
    }
    $arguments = @('--headless', '--standard-session', '--pipe', $pipe, '--data-dir', ('"' + (Join-Path $output 'profile') + '"'))
    $hostProcess = Start-Process -FilePath $HostExe -ArgumentList $arguments -WindowStyle Hidden -PassThru -RedirectStandardError (Join-Path $output 'host.stderr.log')
    # Named-pipe readiness is read-only; no product UI or module is activated.
    $deadline = [DateTime]::UtcNow.AddSeconds(10)
    do {
        if ($hostProcess.HasExited) { throw 'Host exited during initialization' }
        $connection = [IO.Pipes.NamedPipeClientStream]::new('.', $pipe.Substring(9), [IO.Pipes.PipeDirection]::InOut)
        try { $connection.Connect(100); $ready = $true } catch [TimeoutException] { $ready = $false } finally { $connection.Dispose() }
    } while (-not $ready -and [DateTime]::UtcNow -lt $deadline)
    if (-not $ready) { throw 'Host pipe did not become ready' }
    for ($iteration = 0; $iteration -lt $Runs; $iteration++) {
        $probe = [RustDesktopProbe]::new($DesktopExe, $pipe, (Join-Path $output 'profile'))
        $readyMilliseconds = $probe.ReadyMilliseconds
        $logMilliseconds = if ($LogView) { $probe.OpenCaptureLogs($logText) } else { $null }
        $ping = @(); for ($sample = 0; $sample -lt 20; $sample++) { $ping += $probe.Ping() }
        if ($iteration -eq 0) {
            $scenario = if ($LogView) { 'rust-private-desktop-log-view' } else { 'rust-private-desktop-empty-profile' }
            & (Join-Path $PSScriptRoot 'measure-resources.ps1') -RootProcessIds @($hostProcess.Id,$probe.Child.Id) -OutputPath (Join-Path $output 'open.json') -Scenario $scenario -DurationSeconds $DurationSeconds -BinaryPaths @($HostExe,$DesktopExe)
            $report = Get-Content -LiteralPath (Join-Path $output 'open.json') -Raw -Encoding UTF8 | ConvertFrom-Json
            if (@($report.Samples | Where-Object { -not $_.Complete }).Count -gt 0) { throw 'Incomplete process-group resource samples' }
        }
        $closeMilliseconds = $probe.CloseNormally()
        $samples.Add([pscustomobject]@{Run=$iteration+1;ReadyMilliseconds=$readyMilliseconds;LogViewMilliseconds=$logMilliseconds;MessageDispatchMilliseconds=$ping;CloseMilliseconds=$closeMilliseconds})
        $probe.Dispose(); $probe = $null
    }
    & (Join-Path $PSScriptRoot 'measure-resources.ps1') -RootProcessIds @($hostProcess.Id) -OutputPath (Join-Path $output 'closed.json') -Scenario 'rust-private-desktop-ui-closed' -DurationSeconds $DurationSeconds -BinaryPaths @($HostExe,$DesktopExe)
    $result = [pscustomobject]@{Scope='Private desktop, dedicated profile, warm Host. First run is not a verified cold-cache start. Log readiness and WM_NULL are not input-to-presentation latency.';LogView=[bool]$LogView;Runs=$samples.ToArray()}
    [IO.File]::WriteAllText((Join-Path $output 'lifecycle.json'),($result | ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
    $samples | Select-Object Run,ReadyMilliseconds,LogViewMilliseconds,CloseMilliseconds
} finally {
    if ($null -ne $probe) { $probe.Dispose() }
    if ($null -ne $hostProcess) {
        if (-not $hostProcess.HasExited) { $hostProcess.Kill(); $hostProcess.WaitForExit() }
        $hostProcess.Dispose()
    }
    $env:LOCALAPPDATA = $savedLocal
}
