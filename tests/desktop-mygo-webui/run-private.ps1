param(
    [Parameter(Mandatory)][string]$ProbeExe,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [int]$TimeoutSeconds = 90
)
# Runs apps/desktop-mygo/cmd/webuiprobe (built with -tags webui) on a
# private Win32 desktop: its window never shows on the user's desktop and
# takes no input from it. The probe writes probe.json and panel.png to
# OutputDirectory, which must be new and Git-ignored (under _temp/).
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$probe = [IO.Path]::GetFullPath($ProbeExe)
$output = [IO.Path]::GetFullPath($OutputDirectory)
& (Join-Path $root 'build/assert-temporary-path.ps1') -Path $output
if (Test-Path -LiteralPath $output) { throw 'Choose a new output directory' }
& git -C $root check-ignore -q --no-index -- (Join-Path $output 'probe.json')
if ($LASTEXITCODE -ne 0) { throw 'Probe output must be inside a Git-ignored directory' }
Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class WinOneWebUIProbe {
 [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)] public struct Startup {
  public int size; public string reserved,desktop,title;
  public int x,y,width,height,charsX,charsY,fill,flags; public short show,reservedSize;
  public IntPtr reservedBytes,input,output,error;
 }
 [StructLayout(LayoutKind.Sequential)] public struct ProcessInfo { public IntPtr process,thread; public int pid,tid; }
 [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] public static extern IntPtr CreateDesktopW(string name,IntPtr device,IntPtr mode,uint flags,uint access,IntPtr security);
 [DllImport("user32.dll",SetLastError=true)] public static extern bool CloseDesktop(IntPtr desktop);
 [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool CreateProcessW(string app,StringBuilder command,IntPtr processSecurity,IntPtr threadSecurity,bool inherit,uint flags,IntPtr environment,string cwd,ref Startup startup,out ProcessInfo process);
 [DllImport("kernel32.dll",SetLastError=true)] public static extern uint WaitForSingleObject(IntPtr handle,uint timeout);
 [DllImport("kernel32.dll",SetLastError=true)] public static extern bool GetExitCodeProcess(IntPtr process,out uint code);
 [DllImport("kernel32.dll",SetLastError=true)] public static extern bool TerminateProcess(IntPtr process,uint code);
 [DllImport("kernel32.dll")] public static extern bool CloseHandle(IntPtr handle);
 public static ProcessInfo Start(string exe,string args,string desktop,string cwd) {
  var startup=new Startup { size=Marshal.SizeOf(typeof(Startup)),desktop=desktop };
  ProcessInfo process;
  if(!CreateProcessW(exe,new StringBuilder("\""+exe+"\" "+args),IntPtr.Zero,IntPtr.Zero,false,0,IntPtr.Zero,cwd,ref startup,out process))
   throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
  return process;
 }
}
'@
$name = 'WinOne-WebUI-' + [Guid]::NewGuid().ToString('N')
$desktop = [WinOneWebUIProbe]::CreateDesktopW($name, [IntPtr]::Zero, [IntPtr]::Zero, 0, 0xc7, [IntPtr]::Zero)
if ($desktop -eq [IntPtr]::Zero) { throw 'Private test desktop creation failed' }
$child = $null
# MyGo keeps WebView2's data under %LOCALAPPDATA%\<app>\WebView2: the probe
# inherits a LOCALAPPDATA inside its output, so nothing lands in the
# user's profile.
$savedLocal = $env:LOCALAPPDATA
try {
    $env:LOCALAPPDATA = Join-Path $output 'local'
    New-Item -ItemType Directory -Path $env:LOCALAPPDATA -Force | Out-Null
    $child = [WinOneWebUIProbe]::Start($probe, '"' + $output + '"', $name, $root)
    $env:LOCALAPPDATA = $savedLocal
    $wait = [WinOneWebUIProbe]::WaitForSingleObject($child.process, $TimeoutSeconds * 1000)
    if ($wait -ne 0) {
        # Only this run's probe is ended when its deadline passes.
        [void][WinOneWebUIProbe]::TerminateProcess($child.process, 1)
        [void][WinOneWebUIProbe]::WaitForSingleObject($child.process, 5000)
        throw ('Probe wait failed or timed out: ' + $wait)
    }
    $code = 0
    [void][WinOneWebUIProbe]::GetExitCodeProcess($child.process, [ref]$code)
    if ($code -ne 0) { throw "Probe failed with exit code $code; see $output\probe.json" }
    Write-Output "Probe passed: $output"
} finally {
    $env:LOCALAPPDATA = $savedLocal
    if ($child) { [void][WinOneWebUIProbe]::CloseHandle($child.thread); [void][WinOneWebUIProbe]::CloseHandle($child.process) }
    [void][WinOneWebUIProbe]::CloseDesktop($desktop)
}
