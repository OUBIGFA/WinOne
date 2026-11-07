[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$BundleDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [ValidateRange(2,100)][int]$Runs=21,
    [switch]$EnforceWarmBudget,
    [switch]$ObserveGPUHandoff
)
# Explicit real-process benchmark on a repository-owned private desktop. No
# foreground switching, user profile, account or installed application is used.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
$bundle=(Resolve-Path -LiteralPath $BundleDirectory).Path
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'result.json')
if($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose a new Git-ignored output directory'}
[IO.Directory]::CreateDirectory($output)|Out-Null
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
$before=[WinOnePrivateTestDesktop]::InputName()
$desktop=$null;$process=$null
$saved=$env:WINONE_TEST_DESKTOP_NAME
$exitCode=$null;$after=$null
try{
    $desktop=[WinOnePrivateTestDesktop]::new()
    $env:WINONE_TEST_DESKTOP_NAME=$desktop.Name
    $measurements=Join-Path $output 'measurements'
    $log=Join-Path $output 'tests.log'
    $runner=Join-Path $output 'run.ps1'
    $command="& '"+(Join-Path $PSScriptRoot 'benchmark-component-startup.ps1').Replace("'","''")+"' -BundleDirectory '"+$bundle.Replace("'","''")+"' -OutputDirectory '"+$measurements.Replace("'","''")+"' -Runs "+$Runs+" -Frontend MyGo"
    if($EnforceWarmBudget){$command+=' -EnforceWarmBudget'}
    if($ObserveGPUHandoff){$command+=' -ObserveGPUHandoff'}
    $run="`$ErrorActionPreference='Stop'`ntry { "+$command+" *> '"+$log.Replace("'","''")+"'; exit 0 } catch { `$_ | Out-String | Add-Content -LiteralPath '"+$log.Replace("'","''")+"'; exit 1 }`n"
    [IO.File]::WriteAllText($runner,$run,[Text.UTF8Encoding]::new($false))
    $process=$desktop.Start((Get-Command powershell.exe).Source,('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$runner+'"'),$root)
    $null=$process.Handle
    if(-not $process.WaitForExit([Math]::Max(120000,$Runs*30000))){throw 'Native startup benchmark timed out; reports retained'}
    $exitCode=$process.ExitCode
    if($exitCode -ne 0){throw "Native startup benchmark failed; see $log"}
}finally{
    if($process){
        if(-not $process.HasExited){$process.Kill();[void]$process.WaitForExit(5000)}
        $process.Dispose()
    }
    if($desktop){$desktop.Dispose()}
    $env:WINONE_TEST_DESKTOP_NAME=$saved
    $after=[WinOnePrivateTestDesktop]::InputName()
    [IO.File]::WriteAllText((Join-Path $output 'desktop.json'),(@{before=$before;after=$after;exitCode=$exitCode;switched=$false}|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
    if($after -ne $before){throw 'Input desktop changed during a nonforeground benchmark'}
}
Write-Output "Native startup benchmark verified: $measurements"
