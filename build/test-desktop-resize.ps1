param(
    [Parameter(Mandatory)][string]$DesktopExe,
    [Parameter(Mandatory)][string]$HostExe,
    [Parameter(Mandatory)][string]$WinTrayExe,
    [Parameter(Mandatory)][string]$PlaywrightModule,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [int]$DebugPort=19261,
    [switch]$RunVisibleInteraction
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
if(-not $RunVisibleInteraction){throw 'Pass -RunVisibleInteraction to test only the isolated window with real mouse input'}
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'result.json')
if($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose a new ignored test directory'}
New-Item -ItemType Directory -Path $output | Out-Null
Copy-Item -LiteralPath (Resolve-Path -LiteralPath $DesktopExe).Path -Destination (Join-Path $output 'WinOne.exe')
Copy-Item -LiteralPath (Resolve-Path -LiteralPath $HostExe).Path -Destination (Join-Path $output 'WinOne.Host.exe')
Copy-Item -LiteralPath (Resolve-Path -LiteralPath $WinTrayExe).Path -Destination (Join-Path $output 'WinOne.WinTray.exe')
Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class ComponentDesktopProbe {
 [StructLayout(LayoutKind.Sequential,CharSet=CharSet.Unicode)] public struct Startup {public int size;public string reserved,desktop,title;public int x,y,width,height,charsX,charsY,fill,flags;public short show,reservedSize;public IntPtr reservedBytes,input,output,error;}
 [StructLayout(LayoutKind.Sequential)] public struct ProcessInfo {public IntPtr process,thread;public int pid,tid;}
 [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool CreateProcessW(string app,StringBuilder command,IntPtr processSecurity,IntPtr threadSecurity,bool inherit,uint flags,IntPtr environment,string cwd,ref Startup startup,out ProcessInfo process);
 [DllImport("kernel32.dll")] public static extern bool CloseHandle(IntPtr handle);
 [DllImport("kernel32.dll",SetLastError=true)] public static extern bool GetExitCodeProcess(IntPtr process,out uint code);
 public static ProcessInfo Start(string exe,string args,string desktop,string cwd){var startup=new Startup{size=Marshal.SizeOf(typeof(Startup)),desktop=desktop};ProcessInfo info;if(!CreateProcessW(exe,new StringBuilder("\""+exe+"\" "+args),IntPtr.Zero,IntPtr.Zero,false,0x08000000,IntPtr.Zero,cwd,ref startup,out info))throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());return info;}
}
'@
$name='default'
Copy-Item -LiteralPath (Join-Path (Split-Path -Parent $DesktopExe) 'capture'),(Join-Path (Split-Path -Parent $DesktopExe) 'always-on-top') -Destination $output -Recurse
$profile=Join-Path $output 'profile'
New-Item -ItemType Directory -Path $profile|Out-Null
[IO.File]::WriteAllText((Join-Path $profile 'settings.json'),'{"schemaVersion":1,"theme":"light","language":"zh-CN","captureShortcut":"","pinShortcut":"","startup":{"runAtLogon":false,"startMinimized":true}}',[Text.UTF8Encoding]::new($false))
$pipe='\\.\pipe\WinOne-components-test-'+[guid]::NewGuid().ToString('N')
$previousArguments=$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS
$previousModule=$env:WINONE_PLAYWRIGHT_MODULE
$previousTarget=$env:WINONE_NATIVE_SHOT_PID
$child=$null;$process=$null
try{
    $env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS="--remote-debugging-port=$DebugPort"
    $env:WINONE_PLAYWRIGHT_MODULE=[IO.Path]::GetFullPath($PlaywrightModule)
    $arguments='--pipe "'+$pipe+'" --data-dir "'+$profile+'"'
    $child=[ComponentDesktopProbe]::Start((Join-Path $output 'WinOne.exe'),$arguments,$name,$root)
    $process=[Diagnostics.Process]::GetProcessById($child.pid)
    $env:WINONE_NATIVE_SHOT_PID=[string]$child.pid
    $testArgs='"'+(Join-Path $root 'apps/desktop/frontend/tests/rust-resize-check.mjs')+'" "http://127.0.0.1:'+ $DebugPort +'" "'+$output+'"'
    $nodeInfo=[ComponentDesktopProbe]::Start((Get-Command node.exe).Source,$testArgs,$name,$root)
    $nodeProcess=$null
    try{
      $nodeProcess=[Diagnostics.Process]::GetProcessById($nodeInfo.pid)
      if(-not $nodeProcess.WaitForExit(60000)){throw 'Frame check did not finish'}
      [uint32]$nodeCode=0
      [void][ComponentDesktopProbe]::GetExitCodeProcess($nodeInfo.process,[ref]$nodeCode)
      if($nodeCode -ne 0){throw ('Frame check failed: '+(Get-Content -LiteralPath (Join-Path $output 'node-error.txt') -Raw))}
    }finally{
      # Stop the input driver before disposing its owned application window.
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
    $env:WINONE_NATIVE_SHOT_PID=$previousTarget
    if($process){if(-not $process.HasExited){$process.Kill();$process.WaitForExit()};$process.Dispose()}
    if($child){[void][ComponentDesktopProbe]::CloseHandle($child.thread);[void][ComponentDesktopProbe]::CloseHandle($child.process)}
    # Only this fresh profile's fixture Host is owned by this check.
    Get-CimInstance Win32_Process -Filter "Name = 'WinOne.Host.exe'" | Where-Object {$_.ExecutablePath -eq (Join-Path $output 'WinOne.Host.exe') -and $_.CommandLine.Contains($profile)} | ForEach-Object {Stop-Process -Id $_.ProcessId}

}
