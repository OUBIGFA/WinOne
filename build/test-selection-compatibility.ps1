param(
 [Parameter(Mandatory)][string]$OutputDirectory,
 [string]$ReaderDirectory='_temp/build/current/selection',
 [string]$BundleDirectory,
 [string]$Modes='paragraph-double,paragraph-triple',
 [switch]$AllowDesktopSwitch,
 [switch]$ReaderOnly,
 [switch]$SkipBuild
)
# Explicit native input on our own desktop. The 30-second watchdog restores
# the user's input desktop even if a provider or test process stops replying.
$ErrorActionPreference='Stop'
if($ReaderOnly -and $BundleDirectory){throw 'Choose reader checks or automatic toolbar checks'}
$root=Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'use-local-resources.ps1')
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
if(Test-Path -LiteralPath $output){throw 'Choose a new test output directory'}
$work=Join-Path (Split-Path -Parent $output) 'selection-harness'
New-Item -ItemType Directory -Force -Path $work|Out-Null
$reader=[IO.Path]::GetFullPath((Join-Path $root $ReaderDirectory))
if(-not $SkipBuild){
 & (Join-Path $PSScriptRoot 'build-selection.ps1') -OutputDirectory $reader -CargoExe $WinOneBuildParameters.CargoExe -GoExe $WinOneBuildParameters.GoExe -TargetDirectory $WinOneBuildParameters.SelectionTargetDirectory
 & $WinOneBuildParameters.GoExe build -o (Join-Path $work 'selection-driver.exe') ./tests/selection-driver
 if($LASTEXITCODE-ne 0){throw 'Selection driver build failed'}
 $previous=$env:GOROOT
 try{
  $env:GOROOT=Split-Path -Parent (Split-Path -Parent $WinOneBuildParameters.ResultGoExe)
  & $WinOneBuildParameters.ResultGoExe -C (Join-Path $root 'apps/desktop-mygo') build -o (Join-Path $work 'selection-fixture.exe') ./tests/selection-fixture
  if($LASTEXITCODE-ne 0){throw 'Native selection fixture build failed'}
 }finally{$env:GOROOT=$previous}
 & dotnet build (Join-Path $root 'tests/selection-probe/SelectionProbe.csproj') -c Release -p:RestoreLockedMode=true -o (Join-Path $work 'probe') --nologo -v:q
 if($LASTEXITCODE-ne 0){throw 'Selection probe build failed'}
}
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
$desktop=[WinOnePrivateTestDesktop]::new([bool]$AllowDesktopSwitch)
$before=[WinOnePrivateTestDesktop]::InputName()
$environmentNames=@('WINONE_SELECTION_PIPELINE_BUNDLE','WINONE_SELECTION_PIPELINE_URL','WINONE_SELECTION_AUTOMATIC_FIXTURE','WINONE_SELECTION_AUTOMATIC_MODES','WINONE_SELECTION_PIPELINE_DIAGNOSTICS')
$previousEnvironment=@{}
foreach($name in $environmentNames){$previousEnvironment[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
try{
 if($AllowDesktopSwitch){$desktop.BeginForegroundTest(30)}
 $probe=Join-Path $work 'probe/WinOne.Selection.Probe.exe'
 $arguments='--compatibility "'+(Join-Path $work 'selection-driver.exe')+'" "'+(Join-Path $reader 'WinOne.Selection.exe')+'" "'+(Join-Path $work 'selection-fixture.exe')+'" "'+$output+'"'
 if($ReaderOnly){$arguments='--reader "'+(Join-Path $reader 'WinOne.Selection.exe')+'" --output "'+$output+'"'}
 if($BundleDirectory){
  $env:WINONE_SELECTION_PIPELINE_BUNDLE=[IO.Path]::GetFullPath((Join-Path $root $BundleDirectory))
  $env:WINONE_SELECTION_PIPELINE_URL='http://127.0.0.1:1/v1'
  $env:WINONE_SELECTION_AUTOMATIC_FIXTURE=Join-Path $work 'selection-fixture.exe'
  $env:WINONE_SELECTION_AUTOMATIC_MODES=$Modes
  $env:WINONE_SELECTION_PIPELINE_DIAGNOSTICS='1'
  $arguments='--pipeline "'+$output+'"'
 }
 $process=$desktop.Start($probe,('--guard '+$arguments),$root)
 if(-not $process.WaitForExit(40000)){throw 'Selection test guard did not finish; desktop restored by watchdog'}
 if($process.ExitCode-ne 0){foreach($log in @((Join-Path $output 'failure.txt'),($output+'.guard.log'))){if(Test-Path -LiteralPath $log){Get-Content -LiteralPath $log -Encoding UTF8}};throw "Native selection compatibility test failed or timed out: $($process.ExitCode)"}
 $report=if($BundleDirectory){'automatic.json'}elseif($ReaderOnly){'results.json'}else{'result.json'}
 Get-Content -LiteralPath (Join-Path $output $report) -Encoding UTF8
}finally{
 foreach($name in $environmentNames){[Environment]::SetEnvironmentVariable($name,$previousEnvironment[$name],'Process')}
 $desktop.Dispose()
 if([WinOnePrivateTestDesktop]::InputName()-ne $before){throw 'Input desktop was not restored'}
}
