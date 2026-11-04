[CmdletBinding()]
param(
    [Parameter(Mandatory)][string[]]$Paths,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$DefenderExe=(Join-Path $env:ProgramFiles 'Windows Defender/MpCmdRun.exe')
)
$ErrorActionPreference='Stop'
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$reportPath=Join-Path $output 'result.json'
if(Test-Path -LiteralPath $reportPath){throw 'Security report already exists; preserve it and choose a new output directory'}
if(-not (Test-Path -LiteralPath $DefenderExe -PathType Leaf)){throw 'Microsoft Defender scan tool is unavailable'}
$status=Get-MpComputerStatus
$preferences=Get-MpPreference
if(-not $status.AntivirusEnabled -or -not $status.RealTimeProtectionEnabled -or $status.AMRunningMode -ne 'Normal' -or $preferences.DisableBehaviorMonitoring -or $preferences.DisableIOAVProtection){throw 'Active Defender, real-time and behavior protection are required; no security settings were changed'}
[IO.Directory]::CreateDirectory($output)|Out-Null
$scans=[Collections.Generic.List[object]]::new()
$started=Get-Date
foreach($requested in $Paths){
    $path=[IO.Path]::GetFullPath($requested)
    $scan=[ordered]@{path=$path;startedAt=(Get-Date).ToString('o');passed=$false;exitCode=$null;sha256=$null;inventorySHA256=$null;error=$null}
    $index=$scans.Count+1
    $log=Join-Path $output (('{0:D2}-' -f $index)+([IO.Path]::GetFileName($path) -replace '[^A-Za-z0-9_.-]','_')+'.txt')
    $scan.log=[IO.Path]::GetFileName($log)
    try {
        $item=Get-Item -LiteralPath $path
        if(-not $item.PSIsContainer){$scan.sha256=(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()}
        else {
            # Bind a directory scan to the package/build inventory when present.
            foreach($name in @('manifest.json','build-info.json')){
                $inventory=Join-Path $path $name
                if(Test-Path -LiteralPath $inventory -PathType Leaf){$scan.inventorySHA256=(Get-FileHash -LiteralPath $inventory -Algorithm SHA256).Hash.ToLowerInvariant();break}
            }
        }
        # This is a diagnostic custom scan, not disabling real-time protection.
        # Microsoft documents that it ignores file exclusions, scans archives
        # and reports detections to stdout without restoring/removing any file.
        # https://learn.microsoft.com/defender-endpoint/command-line-arguments-microsoft-defender-antivirus
        $previous=$ErrorActionPreference
        try {
            $ErrorActionPreference='Continue'
            $text=& $DefenderExe -Scan -ScanType 3 -File $path -DisableRemediation 2>&1 | Out-String
            $scan.exitCode=$LASTEXITCODE
        } finally {$ErrorActionPreference=$previous}
        [IO.File]::WriteAllText($log,$text,[Text.UTF8Encoding]::new($false))
        # Exit 0 alone can also mean successful remediation. Require the actual
        # clean summary from this non-remediating scan, never just the exit code.
        $scan.passed=$scan.exitCode -eq 0 -and $text -match '(?i)found no threats\.?\s*$'
        if($scan.sha256 -and (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $scan.sha256){$scan.passed=$false;$scan.error='Scanned file changed during verification'}
        if(-not $scan.passed -and -not $scan.error){$scan.error='Defender did not report a completed, clean scan; see the command log'}
    } catch {
        # A quarantine or disappearance after a clean summary must invalidate
        # that summary too, not retain the earlier passed flag.
        $scan.passed=$false
        $scan.error=$_.Exception.Message
        if(-not (Test-Path -LiteralPath $log)){[IO.File]::WriteAllText($log,$scan.error,[Text.UTF8Encoding]::new($false))}
    }
    $scan.finishedAt=(Get-Date).ToString('o')
    $scans.Add([pscustomobject]$scan)
    Write-Output ("Defender scan: {0}; passed={1}; exit={2}" -f $path,$scan.passed,$scan.exitCode)
}
$after=Get-MpComputerStatus
$passed=@($scans|Where-Object {-not $_.passed}).Count -eq 0 -and $after.AntivirusEnabled -and $after.RealTimeProtectionEnabled -and $after.AMRunningMode -eq 'Normal'
$report=[ordered]@{
    startedAt=$started.ToString('o');finishedAt=(Get-Date).ToString('o');passed=$passed
    method='MpCmdRun custom scan with -DisableRemediation; real-time protection left enabled'
    scanner=[IO.Path]::GetFullPath($DefenderExe)
    securityIntelligenceBefore=$status.AntivirusSignatureVersion;securityIntelligenceAfter=$after.AntivirusSignatureVersion
    protection=($preferences|Select-Object DisableRealtimeMonitoring,DisableBehaviorMonitoring,DisableIOAVProtection,DisableArchiveScanning,ExclusionPath,ExclusionProcess,ExclusionExtension,ThreatIDDefaultAction_Ids,ThreatIDDefaultAction_Actions)
    scans=$scans.ToArray()
    limitations=@('Static scan only; verify installation and runtime behavior separately.','Custom scans ignore file exclusions; real-time exclusions may require administrator access to inspect.','No security settings, allowed threats, exclusions or quarantined files were modified.','No sample was explicitly uploaded for external review; existing Defender cloud policy was not changed.')
}
[IO.File]::WriteAllText($reportPath,($report|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
if(-not $passed){throw "Windows security verification failed: $reportPath"}
Write-Output "Windows static security scans verified: $reportPath"
