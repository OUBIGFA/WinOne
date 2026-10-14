param(
    [Parameter(Mandatory)][string]$BundleDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$fixtureStarted=[datetime]::UtcNow
$staleCreated=$fixtureStarted.AddDays(-1)
$fixtureState=@{InjectedInventories=0}

# Run the real benchmark against its isolated window. A stale process whose
# parent PID has been reused must not become part of that window's family.
# This shim changes only inventory data; it never creates or closes a user app.
function Get-CimInstance {
    [CmdletBinding()]
    param([Parameter(Position=0)][string]$ClassName,[string[]]$Property,[string]$Filter)
    $rows=@(CimCmdlets\Get-CimInstance @PSBoundParameters)
    if($ClassName -ne 'Win32_Process' -or -not $Property){return $rows}
    if($Property -contains 'ParentProcessId'){
        $roots=@($rows | Where-Object {$_.Name -eq 'WinOne.exe' -and $_.CreationDate.ToUniversalTime() -ge $fixtureStarted})
        if($roots.Count -ne 1){throw 'Expected one newly launched benchmark window'}
        $fixtureState.InjectedInventories++
        $rows+=[pscustomobject]@{ProcessId=4000000001;ParentProcessId=$roots[0].ProcessId;Name='msedgewebview2.exe';CreationDate=$staleCreated}
        $rows+=[pscustomobject]@{ProcessId=4000000002;ParentProcessId=4000000001;Name='msedgewebview2.exe';CreationDate=$staleCreated.AddSeconds(1)}
    }elseif($Property -contains 'CreationDate'){
        $rows+=[pscustomobject]@{ProcessId=4000000001;CreationDate=$staleCreated}
        $rows+=[pscustomobject]@{ProcessId=4000000002;CreationDate=$staleCreated.AddSeconds(1)}
    }
    return $rows
}

& (Join-Path $PSScriptRoot '../../build/benchmark-component-startup.ps1') -BundleDirectory $BundleDirectory -OutputDirectory $OutputDirectory -Runs 2
if($fixtureState.InjectedInventories -lt 2){throw 'Stale-parent inventory was not exercised in both launches'}
$workspace=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$result=Get-Content -Raw -Encoding UTF8 -LiteralPath (Join-Path (Join-Path $workspace $OutputDirectory) 'result.json') | ConvertFrom-Json
if(-not $result.Completed -or $result.Failure -or $result.Runs.Count -ne 2){throw 'Benchmark did not complete both real launches'}
if(@($result.Runs | Where-Object {-not $_.ObservedWebViewsExited}).Count){throw 'An owned WebView survived close'}
'PASS: stale parent PID and its descendants do not contaminate startup cleanup'
