param(
    [Parameter(Mandatory)][string]$DesktopExe,
    [Parameter(Mandatory)][string]$HostExe,
    [Parameter(Mandatory)][string]$WinTrayExe,
    [Parameter(Mandatory)][string]$PlaywrightModule,
    [Parameter(Mandatory)][string]$ProbeExe,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [int]$DebugPort=19368,
    [string]$TestScript='rust-directory-links-ole-check.mjs',
    [switch]$CurrentDesktop,
    [switch]$Elevated,
    [switch]$FullBundle
)
$ErrorActionPreference='Stop'
if(-not $CurrentDesktop){throw 'Real OLE input requires explicitly selecting -CurrentDesktop'}
$root=Split-Path -Parent $PSScriptRoot
$output=[IO.Path]::GetFullPath((Join-Path $root $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
$ownedProbe=Join-Path $output 'WinOne.OleDropProbe.exe'
if(-not ('WinOneOleRestore' -as [type])){
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class WinOneOleRestore {
 [StructLayout(LayoutKind.Sequential)] public struct Point { public int x,y; }
 [DllImport("user32.dll")] public static extern bool GetCursorPos(out Point point);
 [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr window);
 [DllImport("user32.dll")] public static extern bool IsWindow(IntPtr window);
 [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
}
'@
}
$cursor=[WinOneOleRestore+Point]::new()
if(-not [WinOneOleRestore]::GetCursorPos([ref]$cursor)){throw 'Could not record the current cursor position'}
$foreground=[WinOneOleRestore]::GetForegroundWindow()
$previousProbe=$env:WINONE_OLE_DROP_PROBE
try{
    $env:WINONE_OLE_DROP_PROBE=(Resolve-Path -LiteralPath $ProbeExe).Path
    & (Join-Path $PSScriptRoot 'test-desktop-components.ps1') -DesktopExe $DesktopExe -HostExe $HostExe -WinTrayExe $WinTrayExe -PlaywrightModule $PlaywrightModule -OutputDirectory $OutputDirectory -DebugPort $DebugPort -TestScript $TestScript -CurrentDesktop -Elevated:$Elevated -FullBundle:$FullBundle -DriverTimeoutSeconds 120
}finally{
    # Only this fresh runner directory can own a probe. Never enumerate by
    # title or stop a source window from another invocation.
    foreach($owned in @(Get-CimInstance Win32_Process -Filter "Name = 'WinOne.OleDropProbe.exe'" | Where-Object {$_.ExecutablePath -and [string]::Equals($_.ExecutablePath,$ownedProbe,[StringComparison]::OrdinalIgnoreCase)})){
        $process=Get-Process -Id $owned.ProcessId -ErrorAction SilentlyContinue
        if($process){
            [void]$process.CloseMainWindow()
            if(-not $process.WaitForExit(1000)){Stop-Process -Id $owned.ProcessId}
        }
    }
    [void][WinOneOleRestore]::SetCursorPos($cursor.x,$cursor.y)
    # Restore focus only if it is still ours (or already at the original
    # window). A user's explicit switch to another app takes precedence.
    $current=[WinOneOleRestore]::GetForegroundWindow();[uint32]$foregroundPid=0
    [void][WinOneOleRestore]::GetWindowThreadProcessId($current,[ref]$foregroundPid)
    $currentProcess=Get-CimInstance Win32_Process -Filter "ProcessId = $foregroundPid" -ErrorAction SilentlyContinue
    $stillOwned=$currentProcess -and $currentProcess.ExecutablePath -and $currentProcess.ExecutablePath.StartsWith($output+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)
    if(($current -eq $foreground -or $stillOwned) -and [WinOneOleRestore]::IsWindow($foreground)){[void][WinOneOleRestore]::SetForegroundWindow($foreground)}
    $env:WINONE_OLE_DROP_PROBE=$previousProbe
}
