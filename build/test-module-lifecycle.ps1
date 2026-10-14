param(
    [Parameter(Mandatory)][string]$BundleDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$GoExe='go',
    [string]$DotnetExe='dotnet',
    [switch]$RunSystemChecks,
    [switch]$PrivateChild,
    # Real DWM pin borders are tied to Windows' current virtual desktop. This
    # explicit mode still operates only owned fixture windows and profiles.
    [switch]$CurrentDesktop
)
$ErrorActionPreference='Stop'
if(-not $RunSystemChecks){throw 'Use -RunSystemChecks to open owned test windows and check temporary shortcut registrations'}
$root=Split-Path -Parent $PSScriptRoot
$bundle=(Resolve-Path -LiteralPath $BundleDirectory).Path
$output=if([IO.Path]::IsPathRooted($OutputDirectory)){[IO.Path]::GetFullPath($OutputDirectory)}else{[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))}
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$GoExe=(Get-Command $GoExe -ErrorAction Stop).Source
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'result.json')
if($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose a new ignored test directory'}
. (Join-Path $PSScriptRoot 'private-test-desktop.ps1')
if(-not $PrivateChild -and -not $CurrentDesktop){
    $desktop=[WinOnePrivateTestDesktop]::new();$child=$null
    try{
        $args='-NoProfile -File "'+$PSCommandPath+'" -RunSystemChecks -PrivateChild -BundleDirectory "'+$bundle+'" -OutputDirectory "'+$output+'" -GoExe "'+$GoExe+'" -DotnetExe "'+$DotnetExe+'"'
        $child=$desktop.Start((Get-Command powershell.exe).Source,$args,$root)
        if(-not $child.WaitForExit(180000)){throw 'Private module lifecycle runner timed out'}
        if(Test-Path -LiteralPath (Join-Path $output 'tests.out.log')){Get-Content -LiteralPath (Join-Path $output 'tests.out.log') -Encoding UTF8}
        if($child.ExitCode-ne 0){if(Test-Path -LiteralPath (Join-Path $output 'tests.err.log')){Get-Content -LiteralPath (Join-Path $output 'tests.err.log') -Encoding UTF8};throw 'Private module lifecycle checks failed'}
        return
    }finally{if($child){if(-not $child.HasExited){$child.Kill();$child.WaitForExit()};$child.Dispose()};$desktop.Dispose()}
}
if(-not $CurrentDesktop -and -not [WinOnePrivateTestDesktop]::CurrentName().StartsWith('WinOne-Test-')){throw 'Module tests require the runner-owned private desktop'}
foreach($file in @('WinOne.Host.exe','WinOne.WinTray.exe','capture/WinOne.Capture.exe','always-on-top/WinOne.AlwaysOnTop.exe','selection/WinOne.Selection.exe')){if(-not(Test-Path -LiteralPath (Join-Path $bundle $file))){throw "Missing module: $file"}}
New-Item -ItemType Directory -Path $output | Out-Null
& $DotnetExe build (Join-Path $root 'tests/window-fixture/WindowFixture.csproj') -c Release --nologo -v:q
if($LASTEXITCODE -ne 0){throw 'Owned-window fixture build failed'}
& $DotnetExe build (Join-Path $root 'tests/console-fixture/ConsoleFixture.csproj') -c Release --nologo -v:q
if($LASTEXITCODE -ne 0){throw 'Owned-console fixture build failed'}
$test=Join-Path $output 'module-lifecycle.test.exe'
Push-Location $root
try { & $GoExe test -c -o $test ./tests/integration; if($LASTEXITCODE -ne 0){throw 'Module integration test build failed'} }
finally {Pop-Location}
$start=[Diagnostics.ProcessStartInfo]::new($test,'-test.v -test.timeout=180s -test.run=^TestHost(ModuleDisablePersistsAndReleasesWorkers|OverlappingModuleLifetimes)$')
$start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true;$start.WorkingDirectory=$root
$start.EnvironmentVariables['WINONE_MODULE_HOST_EXE']=Join-Path $bundle 'WinOne.Host.exe'
$start.EnvironmentVariables['WINONE_OVERLAP_BUNDLE']=$bundle
$start.EnvironmentVariables['WINONE_OVERLAP_OUTPUT']=Join-Path $output 'overlap'
$start.EnvironmentVariables['WINONE_WINDOW_FIXTURE']=Join-Path $root 'tests/window-fixture/bin/Release/net10.0-windows/WinOne.WindowFixture.exe'
$start.EnvironmentVariables['WINONE_CONSOLE_FIXTURE']=Join-Path $root 'tests/console-fixture/bin/Release/net10.0-windows/WinOne.ConsoleFixture.exe'
$start.EnvironmentVariables['WINONE_OVERLAP_MEASURE_SCRIPT']=Join-Path $root 'tests/resource-metrics/measure-module-overlap.ps1'
$temporary=Join-Path $output 'temp';New-Item -ItemType Directory -Path $temporary | Out-Null
$start.EnvironmentVariables['TEMP']=$temporary;$start.EnvironmentVariables['TMP']=$temporary
$process=[Diagnostics.Process]::Start($start)
$stdout=$process.StandardOutput.ReadToEndAsync();$stderr=$process.StandardError.ReadToEndAsync()
while(-not $process.WaitForExit(1000)){}
$code=$process.ExitCode
[IO.File]::WriteAllText((Join-Path $output 'tests.out.log'),$stdout.Result,[Text.UTF8Encoding]::new($false))
[IO.File]::WriteAllText((Join-Path $output 'tests.err.log'),$stderr.Result,[Text.UTF8Encoding]::new($false))
$stdout.Result
if($code -ne 0){$stderr.Result;throw "Module lifecycle tests failed: $code"}
'PASS: module shortcuts, overlapping lifetimes, restoration and resource sampling'
