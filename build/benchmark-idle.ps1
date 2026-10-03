[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$BundleDirectory,
    [Parameter(Mandatory=$true)][string]$OutputDirectory,
    [string]$GoExe = 'go',
    [ValidateRange(2,3600)][int]$DurationSeconds = 30,
    [ValidateRange(0,30)][int]$WarmupSeconds = 4
)
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'resource-metrics.psm1') -Force
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$outputRoot = [IO.Path]::GetFullPath($OutputDirectory)
$hostExecutable = Join-Path ([IO.Path]::GetFullPath($BundleDirectory)) 'WinOne.Host.exe'
$workerExecutable = Join-Path ([IO.Path]::GetFullPath($BundleDirectory)) 'WinOne.WinTray.exe'
foreach ($file in @($hostExecutable,$workerExecutable)) { if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Missing test binary: $file" } }
if ($outputRoot.StartsWith($workspace + [IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) {
    & git -C $workspace check-ignore --quiet -- (Join-Path $outputRoot 'probe.json')
    if ($LASTEXITCODE -ne 0) { throw 'Benchmark output inside the repository must be Git-ignored' }
}
[IO.Directory]::CreateDirectory($outputRoot) | Out-Null
foreach ($name in @('host-disabled','enabled-idle')) {
    foreach($suffix in @('.json','-health.json')) {
        if (Test-Path -LiteralPath (Join-Path $outputRoot ($name+$suffix))) { throw 'Choose a new benchmark output directory; existing reports are preserved' }
    }
}
foreach ($caseEntry in @([pscustomobject]@{Name='host-disabled';Enabled=$false},[pscustomobject]@{Name='enabled-idle';Enabled=$true})) {
    $phaseDirectory = [IO.Path]::GetFullPath((Join-Path $outputRoot ($caseEntry.Name+'-'+[guid]::NewGuid().ToString('N'))))
    if (-not $phaseDirectory.StartsWith($outputRoot.TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Profile escaped the benchmark directory' }
    if (Test-Path -LiteralPath $phaseDirectory) { throw 'Temporary profile already exists' }
    [IO.Directory]::CreateDirectory($phaseDirectory) | Out-Null
    $moduleRecords = @{}
    foreach ($moduleId in @('capture','always-on-top','wintray')) { $moduleRecords[$moduleId]=@{enabled=$caseEntry.Enabled;pendingStop=$false} }
    [IO.File]::WriteAllText((Join-Path $phaseDirectory 'module-state.json'),(@{schemaVersion=1;revision=0;modules=$moduleRecords}|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
    [IO.File]::WriteAllText((Join-Path $phaseDirectory 'settings.json'),'{"schemaVersion":1,"theme":"system","language":"zh-CN","captureShortcut":"","pinShortcut":""}',[Text.UTF8Encoding]::new($false))
    $pipeName = '\\.\pipe\WinOne-resource-'+[guid]::NewGuid().ToString('N')
    $target = $null
    $completed = $false
    try {
        # The separate profile contains no user programs or real logon task settings.
        $target=Start-Process -FilePath $hostExecutable -ArgumentList @('--pipe',$pipeName,'--data-dir',('"'+$phaseDirectory+'"')) -PassThru -WindowStyle Hidden
        $null=$target.Handle
        Start-Sleep -Seconds $WarmupSeconds
        if ($target.HasExited) { throw 'Host exited before sampling' }
        Push-Location $workspace
        try {
            $expected=$caseEntry.Enabled.ToString().ToLowerInvariant()
            $health=& $GoExe run ./tests/resource-metrics/probe --pipe $pipeName "--expect-enabled=$expected"
            if ($LASTEXITCODE -ne 0) { throw 'Module health does not match the benchmark scenario' }
            [IO.File]::WriteAllText((Join-Path $outputRoot ($caseEntry.Name+'-health.json')),($health -join [Environment]::NewLine),[Text.UTF8Encoding]::new($false))
        } finally { Pop-Location }
        & (Join-Path $PSScriptRoot 'measure-resources.ps1') -RootProcessIds @($target.Id) -OutputPath (Join-Path $outputRoot ($caseEntry.Name+'.json')) -Scenario $caseEntry.Name -DurationSeconds $DurationSeconds -BinaryPaths @($hostExecutable,$workerExecutable)
        $report=Get-Content -LiteralPath (Join-Path $outputRoot ($caseEntry.Name+'.json')) -Raw -Encoding UTF8 | ConvertFrom-Json
        if (-not (Test-HostOnlyResourceReport -Report $report -HostProcessId $target.Id)) { throw 'Incomplete samples or unexpected idle workers; profile retained for diagnosis' }
        $completed=$true
    } finally {
        if ($null -ne $target) {
            if (-not $target.HasExited) { $target.Kill() }
            if (-not $target.WaitForExit(5000)) { throw 'Benchmark Host did not exit; profile retained' }
            # Keep the profile for diagnosing a failed child shutdown instead of
            # deleting files still needed by a live worker.
            $deadline=[datetime]::UtcNow.AddSeconds(5)
            do {
                $children=@(Get-CimInstance Win32_Process -Filter "ParentProcessId=$($target.Id)")
                if ($children.Count -eq 0) { break }
                Start-Sleep -Milliseconds 100
            } while ([datetime]::UtcNow -lt $deadline)
            $target.Dispose()
            if ($children.Count -gt 0) { throw 'A benchmark worker did not exit; its profile has been kept' }
        }
        if ($completed) {
            Add-Type -AssemblyName Microsoft.VisualBasic
            [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($phaseDirectory,[Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs,[Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin)
        }
    }
}
