param([Parameter(Mandatory)][int]$OwnedProcessId,[Parameter(Mandatory)][string]$Executable,[ValidateSet('state','close','exit','resize')][string]$Action='state',[ValidateRange(100,4096)][int]$Width=1280,[ValidateRange(100,4096)][int]$Height=940)
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$process=Get-Process -Id $OwnedProcessId -ErrorAction SilentlyContinue
if(-not $process){@{alive=$false;visible=$false}|ConvertTo-Json -Compress;exit 0}
if(-not [string]::Equals($process.Path,[IO.Path]::GetFullPath($Executable),[StringComparison]::OrdinalIgnoreCase)){throw 'The window does not belong to this test executable'}
# These messages target only the runner-owned HWND on its private desktop.
Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class BackgroundWindowProbe {
 delegate bool EnumWindow(IntPtr window,IntPtr data);
 [DllImport("user32.dll")] static extern bool EnumWindows(EnumWindow callback,IntPtr data);
 [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr window,StringBuilder name,int count);
 [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
 [DllImport("user32.dll",SetLastError=true)] public static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int width,int height,uint flags);
 [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr window);
 [DllImport("user32.dll",SetLastError=true)] public static extern bool PostMessage(IntPtr window,uint message,UIntPtr w,IntPtr l);
 public static IntPtr Find(uint pid){IntPtr found=IntPtr.Zero;EnumWindows(delegate(IntPtr w,IntPtr ignored){uint owner;GetWindowThreadProcessId(w,out owner);var name=new StringBuilder(128);GetClassName(w,name,128);if(owner==pid&&name.ToString()=="WinOne.Desktop")found=w;return true;},IntPtr.Zero);return found;}
}
'@
$window=[BackgroundWindowProbe]::Find([uint32]$OwnedProcessId)
if($window -eq [IntPtr]::Zero){throw 'The test process has no settings window'}
if($Action -eq 'resize'){
 # Keep focus and Z-order unchanged; only this validated fixture window moves.
 [void][BackgroundWindowProbe]::SetProcessDPIAware()
 if(-not [BackgroundWindowProbe]::SetWindowPos($window,[IntPtr]::Zero,40,40,$Width,$Height,0x14)){throw 'The test window could not be resized'}
}elseif($Action -ne 'state'){
 $marker=if($Action -eq 'exit'){[UIntPtr]::new([uint32]1)}else{[UIntPtr]::Zero}
 if(-not [BackgroundWindowProbe]::PostMessage($window,0x10,$marker,[IntPtr]::Zero)){throw 'The close request was not sent'}
}
@{alive=$true;visible=[BackgroundWindowProbe]::IsWindowVisible($window);window=$window.ToInt64()}|ConvertTo-Json -Compress
