param([Parameter(Mandatory)][string]$ContextFile,[switch]$Closed)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$context=Get-Content -LiteralPath $ContextFile -Raw -Encoding UTF8 | ConvertFrom-Json
$output=Split-Path -Parent ([IO.Path]::GetFullPath($ContextFile))
$fixtureIds=@($context.fixtures | ForEach-Object {[int]$_.pid})
$productIds=@($context.productRoots | ForEach-Object {[int]$_})
if($productIds.Count -ne 4 -or $fixtureIds.Count -ne 2 -or @($fixtureIds|Where-Object {$_ -in $productIds}).Count){throw 'Unexpected workload identities'}
if([int]$context.consoleOwner -notin $fixtureIds){$fixtureIds+=[int]$context.consoleOwner}
if(@($fixtureIds|Where-Object {$_ -in $productIds -or $_ -le 0}).Count){throw 'Invalid owned-console identity'}
if($Closed){
    $active=Get-Content -LiteralPath (Join-Path $output 'resources-product.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    $observed=@($active.Samples|ForEach-Object {$_.Processes}|Sort-Object ProcessId,CreatedUtc -Unique)
    $current=@(Get-CimInstance Win32_Process -Property ProcessId,CreationDate)
    foreach($process in $observed|Where-Object ProcessId -ne $productIds[0]){
        $stamp=([datetime]$process.CreatedUtc).ToUniversalTime().Ticks
        if(@($current|Where-Object {$_.ProcessId -eq $process.ProcessId -and $_.CreationDate.ToUniversalTime().Ticks -eq $stamp}).Count){throw 'An observed product worker or runtime helper survived module disable'}
    }
    $idlePath=Join-Path $output 'resources-disabled.json'
    & (Join-Path $root 'build/measure-resources.ps1') -RootProcessIds @($productIds[0]) -OutputPath $idlePath -Scenario 'modules-disabled-after-overlap-fixtures-restored' -DurationSeconds 10 -BinaryPaths @($context.binaries)
    Import-Module (Join-Path $root 'build/resource-metrics.psm1') -Force
    $idle=Get-Content -LiteralPath $idlePath -Raw -Encoding UTF8 | ConvertFrom-Json
    if(-not (Test-HostIdleBudgetReport -Report $idle -HostProcessId $productIds[0])){throw 'Post-workload Host idle budget or process cleanup failed'}
    'Observed product descendants exited; post-workload Host idle budget passed'
    return
}
$rawPath=Join-Path $output 'resources-raw.json'
& (Join-Path $root 'build/measure-resources.ps1') -RootProcessIds ($productIds+$fixtureIds) -OutputPath $rawPath -Scenario 'native-capture-pin-managed-console-with-owned-fixtures' -DurationSeconds 30 -BinaryPaths @($context.binaries) -WindowHandle ([long]$context.captureWindow) -WindowProcessId ([int]$context.capturePID)
$raw=Get-Content -LiteralPath $rawPath -Raw -Encoding UTF8 | ConvertFrom-Json
. (Join-Path $PSScriptRoot 'module-overlap-metrics.ps1')
$report=ConvertTo-ModuleOverlapReport $raw $context
[IO.File]::WriteAllText((Join-Path $output 'resources-product.json'),($report|ConvertTo-Json -Depth 12),[Text.UTF8Encoding]::new($false))
'Complete raw and product-only resource reports saved'
