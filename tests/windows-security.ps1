[CmdletBinding()]
param(
    [switch]$RunSystemChecks,
    [string]$OutputDirectory='_temp/windows-security-tests'
)
$ErrorActionPreference='Stop'
if(-not $RunSystemChecks){throw 'Use -RunSystemChecks to run explicit, non-remediating Defender scans'}
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $root 'build/assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath $output){throw 'Choose a new test output directory'}
[IO.Directory]::CreateDirectory($output)|Out-Null
$fixture=Join-Path $output 'benign.txt'
[IO.File]::WriteAllText($fixture,'WinOne Windows security verification fixture.',[Text.UTF8Encoding]::new($false))
$fixtureHash=(Get-FileHash -LiteralPath $fixture -Algorithm SHA256).Hash
$verifier=Join-Path $root 'build/verify-windows-security.ps1'
& $verifier -Paths @($fixture) -OutputDirectory (Join-Path $output 'clean')
$clean=Get-Content -LiteralPath (Join-Path $output 'clean/result.json') -Raw -Encoding UTF8 | ConvertFrom-Json
if(-not $clean.passed -or $clean.scans[0].sha256 -ne $fixtureHash.ToLowerInvariant()){throw 'Clean scan did not bind its result to the tested file'}
$missing=Join-Path $output 'missing.exe'
$rejected=$false
try {& $verifier -Paths @($missing) -OutputDirectory (Join-Path $output 'missing')} catch {$rejected=$true}
if(-not $rejected){throw 'An unreadable scan target was accepted'}
$failed=Get-Content -LiteralPath (Join-Path $output 'missing/result.json') -Raw -Encoding UTF8 | ConvertFrom-Json
if($failed.passed -or $failed.scans[0].passed -or -not $failed.scans[0].error){throw 'Failed scan was not reported as a failure'}
# Simulate a scanner's clean summary followed by disappearance of the exact
# test-owned file. This tests the real post-scan hash/error path without malware
# or changes to Defender's configuration; the fake cannot delete another path.
$fake=Join-Path $output 'scan-fixture.exe'
Add-Type -OutputAssembly $fake -OutputType ConsoleApplication -TypeDefinition @'
using System;
using System.IO;
public static class WinOneSecurityScanFixture {
    public static int Main(string[] args) {
        int at=Array.IndexOf(args,"-File");
        if(at<0 || at+1>=args.Length || !string.Equals(args[at+1],Environment.GetEnvironmentVariable("WINONE_SCAN_FIXTURE_TARGET"),StringComparison.OrdinalIgnoreCase)) return 2;
        File.Delete(args[at+1]);
        Console.WriteLine("Scanning "+args[at+1]+" found no threats.");
        return 0;
    }
}
'@
$vanishing=Join-Path $output 'vanishing.txt'
[IO.File]::WriteAllText($vanishing,'test-owned file',[Text.UTF8Encoding]::new($false))
$previousTarget=$env:WINONE_SCAN_FIXTURE_TARGET
$env:WINONE_SCAN_FIXTURE_TARGET=$vanishing
$rejected=$false
try {& $verifier -Paths @($vanishing) -DefenderExe $fake -OutputDirectory (Join-Path $output 'vanishing')} catch {$rejected=$true} finally {$env:WINONE_SCAN_FIXTURE_TARGET=$previousTarget}
if(-not $rejected){throw 'A clean scanner summary was accepted after the verified file disappeared'}
$vanished=Get-Content -LiteralPath (Join-Path $output 'vanishing/result.json') -Raw -Encoding UTF8 | ConvertFrom-Json
if($vanished.passed -or $vanished.scans[0].passed -or -not $vanished.scans[0].error){throw 'A post-scan read failure was marked as passing'}
$report=Join-Path $output 'clean/result.json'
$reportHash=(Get-FileHash -LiteralPath $report -Algorithm SHA256).Hash
$rejected=$false
try {& $verifier -Paths @($fixture) -OutputDirectory (Join-Path $output 'clean')} catch {$rejected=$true}
if(-not $rejected -or (Get-FileHash -LiteralPath $report -Algorithm SHA256).Hash -ne $reportHash){throw 'Existing verification evidence was overwritten'}
if((Get-FileHash -LiteralPath $fixture -Algorithm SHA256).Hash -ne $fixtureHash -or (Test-Path -LiteralPath $missing)){throw 'Verification changed its input files'}
Write-Output 'Windows security scan checks passed: clean file, missing target, post-scan disappearance and preserved evidence'
