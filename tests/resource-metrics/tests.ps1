$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot '../../build/resource-metrics.psm1') -Force
function Assert-That([bool]$Condition,[string]$Message) { if (-not $Condition) { throw $Message } }
$born=[datetime]::UtcNow.AddMinutes(-5)
function Entry($Id,$Parent,$Created) { [pscustomobject]@{ProcessId=$Id;ParentProcessId=$Parent;CreationDate=$Created;Name="process-$Id"} }
$inventory=@((Entry 10 0 $born),(Entry 20 10 $born.AddSeconds(1)),(Entry 30 20 $born.AddSeconds(2)),(Entry 40 0 $born))
$known=@{'10'=[string]$born.Ticks}
$family=Get-TrackedProcessFamily -Inventory $inventory -Known $known
Assert-That ($family.Members.Count -eq 3) 'Descendants were omitted or an unrelated process was included'
$inventory=@((Entry 10 0 $born.AddMinutes(1)),(Entry 20 10 $born.AddSeconds(1)),(Entry 30 20 $born.AddSeconds(2)),(Entry 50 10 $born.AddMinutes(2)))
$family=Get-TrackedProcessFamily -Inventory $inventory -Known $known
Assert-That ($family.Members.Count -eq 2) 'PID reuse changed ownership, or surviving children were lost'
$self=Get-CimInstance Win32_Process -Filter "ProcessId=$PID"
$family=[pscustomobject]@{Members=@($self);Errors=@()}
$counters=@{}
Get-CimInstance Win32_PerfRawData_PerfProc_Process | ForEach-Object { $counters[[string]$_.IDProcess]=$_ }
$sample=Get-ProcessResourceRows -Family $family -Counters $counters -Previous @{} -LogicalProcessors ([Environment]::ProcessorCount)
Assert-That $sample.Complete ($sample.Errors -join '; ')
Assert-That ($sample.Totals.PrivateBytes -gt 0) 'Private Bytes was not measured'
Assert-That ($null -eq $sample.Totals.CpuPercent) 'First sample fabricated a CPU rate'
Get-Command Get-WindowResourceContext -ErrorAction Stop | Out-Null
$rejected=$false
try { Get-WindowResourceContext -WindowHandle 0 -ProcessId $PID | Out-Null } catch { $rejected=$true }
Assert-That $rejected 'Window context accepted an unowned or missing window'
Get-Command Test-HostOnlyResourceReport -ErrorAction Stop | Out-Null
$report=[pscustomobject]@{Samples=@([pscustomobject]@{Complete=$true;Processes=@([pscustomobject]@{ProcessId=10;Name='WinOne.Host.exe'})})}
Assert-That (Test-HostOnlyResourceReport -Report $report -HostProcessId 10) 'A host-only report was rejected'
$report.Samples[0].Processes += [pscustomobject]@{ProcessId=20;Name='WinOne.exe'}
Assert-That (-not (Test-HostOnlyResourceReport -Report $report -HostProcessId 10)) 'A reopened UI was accepted as a closed-window baseline'
function Open-Sample {
    [pscustomobject]@{Complete=$true;Window=[pscustomobject]@{Visible=$true;Minimized=$false;ClientWidth=1000;ClientHeight=700;Dpi=96};Processes=@(
        [pscustomobject]@{ProcessId=10;Name='WinOne.Host.exe'},[pscustomobject]@{ProcessId=20;Name='WinOne.exe'},[pscustomobject]@{ProcessId=30;Name='msedgewebview2.exe'}
    )}
}
$report=[pscustomobject]@{Samples=@((Open-Sample),(Open-Sample))}
Assert-That (Test-OpenWindowResourceReport $report 10 20) 'A complete visible-window group was rejected'
foreach ($failure in @('incomplete','minimized','hidden','missing-window','missing-webview','missing-host','missing-desktop','zero-size','missing-field')) {
    $report=[pscustomobject]@{Samples=@((Open-Sample),(Open-Sample))}
    $sample=$report.Samples[1]
    switch ($failure) {
        'incomplete' {$sample.Complete=$false}
        'minimized' {$sample.Window.Minimized=$true}
        'hidden' {$sample.Window.Visible=$false}
        'missing-window' {$sample.Window=$null}
        'missing-webview' {$sample.Processes=@($sample.Processes|Where-Object ProcessId -ne 30)}
        'missing-host' {$sample.Processes=@($sample.Processes|Where-Object ProcessId -ne 10)}
        'missing-desktop' {$sample.Processes=@($sample.Processes|Where-Object ProcessId -ne 20)}
        'zero-size' {$sample.Window.ClientWidth=0}
        'missing-field' {$sample.PSObject.Properties.Remove('Window')}
    }
    Assert-That (-not (Test-OpenWindowResourceReport $report 10 20)) "Invalid open-window sample accepted: $failure"
}
Assert-That (-not (Test-OpenWindowResourceReport ([pscustomobject]@{Samples=@()}) 10 20)) 'Empty window report accepted'
Assert-That (-not (Test-OpenWindowResourceReport ([pscustomobject]@{}) 10 20)) 'Missing sample collection accepted'
Write-Output 'PASS: process ownership, live counters, window identity, and open/closed-window scenario validation'
