function ConvertTo-ModuleOverlapReport {
    param([object]$Raw,[object]$Context)
    $fixtureIds=@($Context.fixtures | ForEach-Object {[int]$_.pid})
    $productIds=@($Context.productRoots | ForEach-Object {[int]$_})
    if($productIds.Count -ne 4 -or $fixtureIds.Count -ne 2 -or @($fixtureIds|Where-Object {$_ -in $productIds}).Count){throw 'Unexpected workload identities'}
    if([int]$Context.consoleOwner -notin $fixtureIds){$fixtureIds+=[int]$Context.consoleOwner}
    if(@($fixtureIds|Where-Object {$_ -in $productIds -or $_ -le 0}).Count){throw 'Invalid owned-console identity'}
    if(@($Raw.Samples).Count -eq 0){throw 'No resource samples'}
    $samples=@()
    foreach($sample in $Raw.Samples){
        if($sample.Complete -isnot [bool] -or -not $sample.Complete){throw 'Incomplete resource sample'}
        if($null -eq $sample.Window -or -not $sample.Window.Visible -or $sample.Window.Minimized){throw 'Native capture was not visible throughout sampling'}
        $rows=@($sample.Processes)
        if(@($rows.ProcessId|Select-Object -Unique).Count -ne $rows.Count){throw 'Duplicate process identity'}
        foreach($id in ($productIds+$fixtureIds)){
            if(@($rows|Where-Object ProcessId -eq $id).Count -ne 1){throw 'An active workload process exited during sampling'}
        }
        foreach($process in @($rows|Where-Object {$_.ProcessId -in $fixtureIds})){
            if($process.Name -notin @('WinOne.PinFixture.exe','WinOne.ManagedFixture.exe','conhost.exe')){throw 'Fixture exclusion names a different executable'}
        }
        $excluded=@($fixtureIds)
        # A fresh console has a conhost child as well as the client window
        # identity. Separate only these verified fixture descendants; unknown
        # helpers, including product-owned conhost processes, remain counted.
        do {
            $children=@($rows|Where-Object {$_.ParentProcessId -in $excluded -and $_.Name -eq 'conhost.exe' -and $_.ProcessId -notin $excluded})
            foreach($child in $children){if($child.ProcessId -in $productIds){throw 'Fixture exclusion would hide a product root'};$excluded+=$child.ProcessId}
        } while($children.Count)
        $product=@($rows|Where-Object {$_.ProcessId -notin $excluded})
        $totals=[ordered]@{ProcessCount=$product.Count;CpuPercent=$null}
        foreach($field in @('PrivateWorkingSetBytes','PrivateBytes','Handles','GdiObjects','UserObjects')){$totals[$field]=($product|Measure-Object -Property $field -Sum).Sum}
        if(@($product|Where-Object {$null -eq $_.CpuPercent}).Count -eq 0){$totals.CpuPercent=($product|Measure-Object CpuPercent -Sum).Sum}
        $samples+=[pscustomobject]@{ElapsedSeconds=$sample.ElapsedSeconds;IntervalSeconds=$sample.IntervalSeconds;Complete=$sample.Complete;Processes=$product;ExcludedFixtureProcesses=@($rows|Where-Object {$_.ProcessId -in $excluded});Totals=[pscustomobject]$totals}
    }
    $cpu=@($samples|Where-Object {$null -ne $_.Totals.CpuPercent});$weighted=0.0;$seconds=0.0
    foreach($sample in $cpu){$weighted+=$sample.Totals.CpuPercent*$sample.IntervalSeconds;$seconds+=$sample.IntervalSeconds}
    if($seconds -le 0){throw 'No complete CPU intervals'}
    [pscustomobject]@{
        Scope='Host (including the enabled selection listener) and all observed product descendants during generated capture editing, real pin border and a hosted console. Held test programs and their conhost are listed separately in every sample. Unfiltered data remains in resources-raw.json. No settings WebView, selection reader/result, or OCR/AI task is active.'
        ExcludedWindowFixtures=$Context.fixtures;ConsoleWindowOwner=$Context.consoleOwner
        Hardware=$Raw.Hardware;Binaries=$Raw.Binaries
        Summary=[pscustomobject]@{Samples=$samples.Count;AverageCpuPercent=$weighted/$seconds;MaxPrivateWorkingSetMiB=($samples|ForEach-Object {$_.Totals.PrivateWorkingSetBytes}|Measure-Object -Maximum).Maximum/1MB;MaxPrivateBytesMiB=($samples|ForEach-Object {$_.Totals.PrivateBytes}|Measure-Object -Maximum).Maximum/1MB}
        Samples=$samples
    }
}
