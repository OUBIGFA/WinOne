param([Parameter(Mandatory)][string]$CaptureExe,
      [Parameter(Mandatory)][string]$OutputDirectory,
      [string]$PythonExe='python',
      [ValidateSet('light','dark')][string]$Theme='light',
      [ValidateSet('zh-CN','en-US')][string]$Language='zh-CN',
      [int]$Width=0,[double]$Scale=1,
      [string]$RecordingRuntime,
      [switch]$PhysicalInput,
      [string]$AgentDesktopExe,
      [ValidateSet('fresh','double-copy','snap','smoke','annotations','resume','recording-options','toolbar','translation','number','settings','scroll','save-dialog','save-failure','pin','repeat','dialog-owner-exit','recording','parent-exit','recording-parent')][string]$Suite='smoke')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath $output){throw 'Choose a new private capture test output'}
$executable=(Resolve-Path -LiteralPath $CaptureExe).Path
$python=(Get-Command $PythonExe -ErrorAction Stop).Source
if($PhysicalInput -and ($Suite -ne 'recording-options' -or -not $AgentDesktopExe)){throw 'Physical input requires recording-options and an explicit agent-desktop executable'}
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
$desktop=[WinOnePrivateTestDesktop]::new([bool]$PhysicalInput)
try {
    $probe=Join-Path $root 'tests/capture-native/ui_probe.py'
    $arguments='"'+$probe+'" --executable "'+$executable+'" --output "'+$output+'" --suite '+$Suite+' --theme '+$Theme+' --language '+$Language+' --width '+$Width+' --scale '+$Scale.ToString([Globalization.CultureInfo]::InvariantCulture)
    if($RecordingRuntime){$arguments+=' --recording-runtime "'+(Resolve-Path -LiteralPath $RecordingRuntime).Path+'"'}
    # Explicit opt-in only: the shared watchdog restores the original desktop
    # even if the candidate or driver stops responding.
    if($PhysicalInput){
        $arguments+=' --physical-input --agent-desktop "'+(Resolve-Path -LiteralPath $AgentDesktopExe).Path+'"'
        $desktop.BeginForegroundTest(30)
    }
    $process=$desktop.Start($python,$arguments,$root)
    try {
        if(-not $process.WaitForExit(60000)){throw 'Private native capture probe timed out'}
        $resultPath=Join-Path $output 'result.json'
        if(-not(Test-Path -LiteralPath $resultPath)){throw 'Private native capture probe produced no result'}
        $result=Get-Content -LiteralPath $resultPath -Raw -Encoding UTF8 | ConvertFrom-Json
        if($process.ExitCode -ne 0 -or -not $result.passed){throw "Private native capture failed: $($result.error)"}
        "PASS: private native capture $Suite ($($result.events.Count) observed steps)"
    } finally {
        if(-not $process.HasExited){$process.Kill();$process.WaitForExit()}
        $process.Dispose()
    }
} finally {$desktop.Dispose()}
