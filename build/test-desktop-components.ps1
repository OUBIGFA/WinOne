param(
    [Parameter(Mandatory)][string]$DesktopExe,
    [Parameter(Mandatory)][string]$HostExe,
    [Parameter(Mandatory)][string]$WinTrayExe,
    [Parameter(Mandatory)][string]$PlaywrightModule,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [int]$DebugPort=19261,
    [string]$TestScript='rust-components-check.mjs',
    [string]$FixtureProfile='',
    # Windows rejects runas consent from a private desktop. Select this only
    # for an explicit interactive/UAC check of this runner-owned instance.
    [switch]$CurrentDesktop,
    # Module workflows need their complete runtime without requiring a
    # performance sample merely to copy those files into the private fixture.
    [switch]$FullBundle,
    [ValidateRange(0,60)][int]$ResourceSampleSeconds=0,
    # Longer drivers may select more layout/lifecycle scenarios. This changes
    # only the total runner budget, not per-action or product latency checks.
    [ValidateRange(10,300)][int]$DriverTimeoutSeconds=60
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
if($TestScript -notmatch '^[a-z0-9-]+\.mjs$' -or -not(Test-Path -LiteralPath (Join-Path $root ('apps/desktop/frontend/tests/'+$TestScript)))){throw 'Choose an existing frontend test driver'}
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'result.json')
if($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose a new ignored test directory'}
New-Item -ItemType Directory -Path $output | Out-Null
Copy-Item -LiteralPath (Resolve-Path -LiteralPath $DesktopExe).Path -Destination (Join-Path $output 'WinOne.exe')
Copy-Item -LiteralPath (Resolve-Path -LiteralPath $HostExe).Path -Destination (Join-Path $output 'WinOne.Host.exe')
Copy-Item -LiteralPath (Resolve-Path -LiteralPath $WinTrayExe).Path -Destination (Join-Path $output 'WinOne.WinTray.exe')
$aiSource=Join-Path (Split-Path -Parent (Resolve-Path -LiteralPath $HostExe).Path) 'shared/ai'
if($FullBundle -or $ResourceSampleSeconds -gt 0){
    # Module workflows and resource samples need the actual bundle, including every worker;
    # the three-file settings-only fixture would undercount module resources.
    $bundle=Split-Path -Parent (Resolve-Path -LiteralPath $DesktopExe).Path
    if(-not(Test-Path -LiteralPath (Join-Path $bundle 'manifest.json')) -and -not(Test-Path -LiteralPath (Join-Path $bundle 'build-info.json'))){throw 'Full module checks require a verified build or packaged bundle'}
    Get-ChildItem -LiteralPath $bundle | ForEach-Object {Copy-Item -LiteralPath $_.FullName -Destination $output -Recurse -Force}
}elseif(Test-Path -LiteralPath $aiSource){New-Item -ItemType Directory -Force -Path (Join-Path $output 'shared') | Out-Null;Copy-Item -LiteralPath $aiSource -Destination (Join-Path $output 'shared/ai') -Recurse}
Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class ComponentDesktopProbe {
 [StructLayout(LayoutKind.Sequential,CharSet=CharSet.Unicode)] public struct Startup {public int size;public string reserved,desktop,title;public int x,y,width,height,charsX,charsY,fill,flags;public short show,reservedSize;public IntPtr reservedBytes,input,output,error;}
 [StructLayout(LayoutKind.Sequential)] public struct ProcessInfo {public IntPtr process,thread;public int pid,tid;}
 [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] public static extern IntPtr CreateDesktopW(string name,IntPtr device,IntPtr mode,uint flags,uint access,IntPtr security);
 [DllImport("user32.dll",SetLastError=true)] public static extern bool CloseDesktop(IntPtr desktop);
 [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool CreateProcessW(string app,StringBuilder command,IntPtr processSecurity,IntPtr threadSecurity,bool inherit,uint flags,IntPtr environment,string cwd,ref Startup startup,out ProcessInfo process);
 [DllImport("kernel32.dll")] public static extern bool CloseHandle(IntPtr handle);
 [DllImport("kernel32.dll",SetLastError=true)] public static extern bool GetExitCodeProcess(IntPtr process,out uint code);
 public static ProcessInfo Start(string exe,string args,string desktop,string cwd){var startup=new Startup{size=Marshal.SizeOf(typeof(Startup)),desktop=desktop};ProcessInfo info;if(!CreateProcessW(exe,new StringBuilder("\""+exe+"\" "+args),IntPtr.Zero,IntPtr.Zero,false,0x08000000,IntPtr.Zero,cwd,ref startup,out info))throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());return info;}
}
'@
$name='WinOne-Components-'+[guid]::NewGuid().ToString('N')
$desktop=[IntPtr]::Zero
if($CurrentDesktop){$name=$null}else{
    $desktop=[ComponentDesktopProbe]::CreateDesktopW($name,[IntPtr]::Zero,[IntPtr]::Zero,0,0xc7,[IntPtr]::Zero)
    if($desktop -eq [IntPtr]::Zero){throw 'Could not create background test desktop'}
}
$profile=Join-Path $output 'profile'
if($FixtureProfile){
    $fixture=(Resolve-Path -LiteralPath $FixtureProfile).Path
    if(-not $fixture.StartsWith($root+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Fixture profile must belong to the workspace'}
    & git -C $root check-ignore -q --no-index -- (Join-Path $fixture 'configuration.json')
    if($LASTEXITCODE -ne 0){throw 'Fixture profile must be Git-ignored'}
    Copy-Item -LiteralPath $fixture -Destination $profile -Recurse
}
$pipe='\\.\pipe\WinOne-components-test-'+[guid]::NewGuid().ToString('N')
$previousArguments=$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS
$previousModule=$env:WINONE_PLAYWRIGHT_MODULE
$previousPipe=$env:WINONE_COMPONENTS_PIPE;$previousProfile=$env:WINONE_COMPONENTS_PROFILE
$previousPID=$env:WINONE_COMPONENTS_PID
$child=$null;$process=$null
try{
    $env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS="--remote-debugging-port=$DebugPort"
    $env:WINONE_PLAYWRIGHT_MODULE=[IO.Path]::GetFullPath($PlaywrightModule)
    $env:WINONE_COMPONENTS_PIPE=$pipe;$env:WINONE_COMPONENTS_PROFILE=$profile
    # The Host's legacy wake flag must now open the product Home page.
    $arguments='--pipe "'+$pipe+'" --data-dir "'+$profile+'" --open-settings'
    $child=[ComponentDesktopProbe]::Start((Join-Path $output 'WinOne.exe'),$arguments,$name,$root)
    $process=[Diagnostics.Process]::GetProcessById($child.pid)
    $env:WINONE_COMPONENTS_PID=[string]$child.pid
    if($ResourceSampleSeconds -gt 0){
        # Include the launched Host, workers and WebView descendants. This is
        # an isolated startup/idle sample, not physical input latency.
        & (Join-Path $PSScriptRoot 'measure-resources.ps1') -RootProcessIds @($child.pid) -OutputPath (Join-Path $output 'resources.json') -DurationSeconds $ResourceSampleSeconds -Scenario 'isolated-settings-process-family' -BinaryPaths @((Join-Path $output 'WinOne.exe'),(Join-Path $output 'WinOne.Host.exe'))
    }
    $test=$TestScript
    # Reopen children must inherit the same real desktop as the owned UI.
    $testArgs='"'+(Join-Path $root ('apps/desktop/frontend/tests/'+$test))+'" "http://127.0.0.1:'+ $DebugPort +'" "'+$output+'"'
    $nodeInfo=[ComponentDesktopProbe]::Start((Get-Command node.exe).Source,$testArgs,$name,$root)
    $nodeProcess=$null
    try{
        $nodeProcess=[Diagnostics.Process]::GetProcessById($nodeInfo.pid)
        if(-not $nodeProcess.WaitForExit($DriverTimeoutSeconds*1000)){throw 'Component UI driver timed out'}
        [uint32]$nodeCode=0
        [void][ComponentDesktopProbe]::GetExitCodeProcess($nodeInfo.process,[ref]$nodeCode)
        if($nodeCode -ne 0){throw ('Component UI check failed: '+(Get-Content -LiteralPath (Join-Path $output 'node-error.txt') -Raw -Encoding UTF8))}
    }finally{
        if($nodeProcess){if(-not $nodeProcess.HasExited){$nodeProcess.Kill();$nodeProcess.WaitForExit()};$nodeProcess.Dispose()}
        [void][ComponentDesktopProbe]::CloseHandle($nodeInfo.thread);[void][ComponentDesktopProbe]::CloseHandle($nodeInfo.process)
    }
    if(-not $process.WaitForExit(10000)){throw 'Component UI did not exit after closing'}
    [uint32]$exitCode=0
    if(-not [ComponentDesktopProbe]::GetExitCodeProcess($child.process,[ref]$exitCode)){throw 'Could not read component UI exit code'}
    if($exitCode -ne 0){throw "Component UI exited with $exitCode"}
    'PASS: component UI process exited normally'
}finally{
    $env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=$previousArguments
    $env:WINONE_PLAYWRIGHT_MODULE=$previousModule
    $env:WINONE_COMPONENTS_PIPE=$previousPipe;$env:WINONE_COMPONENTS_PROFILE=$previousProfile
    $env:WINONE_COMPONENTS_PID=$previousPID
    if($process){if(-not $process.HasExited){$process.Kill();$process.WaitForExit()};$process.Dispose()}
    if($child){[void][ComponentDesktopProbe]::CloseHandle($child.thread);[void][ComponentDesktopProbe]::CloseHandle($child.process)}
    # Only this fresh profile's fixture Host is owned by this check.
    Get-CimInstance Win32_Process -Filter "Name = 'WinOne.Host.exe'" | Where-Object {$_.ExecutablePath -eq (Join-Path $output 'WinOne.Host.exe') -and $_.CommandLine.Contains($profile)} | ForEach-Object {Stop-Process -Id $_.ProcessId}
    if($desktop -ne [IntPtr]::Zero){[void][ComponentDesktopProbe]::CloseDesktop($desktop)}
}
