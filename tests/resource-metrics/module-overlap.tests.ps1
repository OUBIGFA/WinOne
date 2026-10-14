$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'module-overlap-metrics.ps1')
function Fixture {
    $samples=@()
    foreach($index in 1,2){
        $rows=@();$names=@('WinOne.Host.exe','WinOne.Capture.exe','WinOne.AlwaysOnTop.exe','WinOne.WinTray.exe','WinOne.PinFixture.exe','WinOne.ManagedFixture.exe','conhost.exe','WinOne.Capture.Regions.exe')
        for($i=1;$i -le 8;$i++){$rows+=[pscustomobject]@{ProcessId=$i;ParentProcessId=$(if($i -eq 7){6}else{1});Name=$names[$i-1];PrivateWorkingSetBytes=10;PrivateBytes=20;Handles=3;GdiObjects=1;UserObjects=2;CpuPercent=$(if($index -eq 1){$null}else{1.0})}}
        $samples+=[pscustomobject]@{Complete=$true;Window=[pscustomobject]@{Visible=$true;Minimized=$false};Processes=$rows;ElapsedSeconds=$index;IntervalSeconds=1.0}
    }
    [pscustomobject]@{Samples=$samples;Hardware=@{};Binaries=@()}
}
$context=[pscustomobject]@{productRoots=@(1,2,3,4);fixtures=@(@{pid=5},@{pid=6});consoleOwner=6}
$raw=Fixture;$result=ConvertTo-ModuleOverlapReport $raw $context
if($result.Samples[0].Totals.ProcessCount -ne 5 -or $result.Samples[0].Totals.PrivateWorkingSetBytes -ne 50 -or $null -ne $result.Samples[0].Totals.CpuPercent -or $result.Summary.AverageCpuPercent -ne 5){throw 'Fixture separation or unknown CPU handling is wrong'}
if(8 -notin $result.Samples[0].Processes.ProcessId -or 7 -in $result.Samples[0].Processes.ProcessId){throw 'Product helper and fixture conhost were confused'}
$raw=Fixture;foreach($sample in $raw.Samples){$sample.Processes[6].ParentProcessId=2};$result=ConvertTo-ModuleOverlapReport $raw $context
if(7 -notin $result.Samples[0].Processes.ProcessId){throw 'A product-owned console helper was hidden'}
$raw=Fixture;foreach($sample in $raw.Samples){$sample.Processes[6].Name='UnknownHelper.exe'};$result=ConvertTo-ModuleOverlapReport $raw $context
if(7 -notin $result.Samples[0].Processes.ProcessId){throw 'An unknown fixture descendant was silently excluded'}
foreach($case in @('missing-root','incomplete','duplicate','wrong-fixture','hidden-window')){
    $raw=Fixture
    switch($case){
        'missing-root'{$raw.Samples[0].Processes=@($raw.Samples[0].Processes|Where-Object ProcessId -ne 4)}
        'incomplete'{$raw.Samples[0].Complete=$false}
        'duplicate'{$raw.Samples[0].Processes+= $raw.Samples[0].Processes[7]}
        'wrong-fixture'{$raw.Samples[0].Processes[5].Name='WinOne.RealWorker.exe'}
        'hidden-window'{$raw.Samples[0].Window.Visible=$false}
    }
    $rejected=$false;try{$null=ConvertTo-ModuleOverlapReport $raw $context}catch{$rejected=$true};if(-not $rejected){throw "Invalid sample accepted: $case"}
}
'PASS: fixture/conhost separation, product helpers, unknown CPU and invalid sample rejection'
