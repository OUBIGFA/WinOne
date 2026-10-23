# Physical input regression for a runner-owned WinOne window. CDP alone cannot
# establish that a transparent native child leaves mouse input unobstructed.
param([Parameter(Mandatory)][string]$Specification)
$ErrorActionPreference='Stop'
$spec=Get-Content -LiteralPath $Specification -Raw -Encoding UTF8 | ConvertFrom-Json
$root=[IO.Path]::GetFullPath($spec.taskRoot).TrimEnd([char]92)+[IO.Path]::DirectorySeparatorChar
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $root
foreach($path in @($Specification,$spec.resultPath,$spec.targetExe)){if(-not [IO.Path]::GetFullPath($path).StartsWith($root,[StringComparison]::OrdinalIgnoreCase)){throw 'Input path escapes fixture'}}
$elevated=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if($elevated -ne $spec.expectedTargetElevated){throw 'Input integrity differs from target'}
Add-Type -TypeDefinition @'
using System;using System.Runtime.InteropServices;using System.Text;
public class ComponentNativeInput {
 [StructLayout(LayoutKind.Sequential)]public struct Point{public int x,y;}
 [StructLayout(LayoutKind.Sequential)]public struct Rect{public int left,top,right,bottom;}
 [StructLayout(LayoutKind.Sequential)]struct Mouse{public int x,y;public uint data,flags,time;public IntPtr extra;}
 [StructLayout(LayoutKind.Explicit)]struct Union{[FieldOffset(0)]public Mouse mouse;}
 [StructLayout(LayoutKind.Sequential)]struct Input{public uint type;public Union value;}
 [DllImport("user32.dll")]public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
 [DllImport("user32.dll")]public static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")]public static extern bool SetForegroundWindow(IntPtr h);
 [DllImport("user32.dll")]public static extern bool GetWindowRect(IntPtr h,out Rect r);
 [DllImport("user32.dll")]public static extern bool ClientToScreen(IntPtr h,ref Point p);
 [DllImport("user32.dll")]public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")]public static extern bool GetCursorPos(out Point p);
 [DllImport("user32.dll")]public static extern short GetAsyncKeyState(int key);
 [DllImport("user32.dll")]public static extern IntPtr WindowFromPoint(Point p);
 [DllImport("user32.dll")]public static extern IntPtr GetAncestor(IntPtr h,uint flags);
 [DllImport("user32.dll",SetLastError=true)]static extern uint SendInput(uint count,Input[] input,int size);
 public static void Send(uint flags,int data=0){var input=new Input{value=new Union{mouse=new Mouse{flags=flags,data=unchecked((uint)data)}}};if(SendInput(1,new[]{input},Marshal.SizeOf(typeof(Input)))!=1)throw new System.ComponentModel.Win32Exception();}
 public static void Move(Point point){if(!SetCursorPos(point.x,point.y))throw new System.ComponentModel.Win32Exception();Point actual;GetCursorPos(out actual);if(actual.x!=point.x||actual.y!=point.y)throw new Exception("Mouse did not reach the requested fixture point");}
}
'@
[void][ComponentNativeInput]::SetThreadDpiAwarenessContext([IntPtr]::new(-4))
$target=Get-Process -Id $spec.targetPid
if($target.Path -ne [IO.Path]::GetFullPath($spec.targetExe) -or [IO.Path]::GetFileName($target.Path) -ne 'WinOne.exe'){throw 'Unexpected target executable'}
$window=$target.MainWindowHandle
if($window -eq [IntPtr]::Zero){throw 'No fixture window'}
if($spec.action -notin @('click','wheel','drag')){throw 'Unsupported mouse action'}
if([Math]::Abs($spec.dx) -gt 200 -or [Math]::Abs($spec.dy) -gt 200){throw 'Drag exceeds fixture limit'}
$before=[ComponentNativeInput+Rect]::new();[void][ComponentNativeInput]::GetWindowRect($window,[ref]$before)
$from=[ComponentNativeInput+Point]::new();[void][ComponentNativeInput]::ClientToScreen($window,[ref]$from);$from.x+=[int]$spec.x;$from.y+=[int]$spec.y
$cursor=[ComponentNativeInput+Point]::new();[void][ComponentNativeInput]::GetCursorPos([ref]$cursor)
$pressed=$false
try{
 if(([ComponentNativeInput]::GetAsyncKeyState(1) -band 0x8000) -ne 0){throw 'Mouse is already held'}
 [void][ComponentNativeInput]::SetForegroundWindow($window)
 [ComponentNativeInput]::Move($from);Start-Sleep -Milliseconds 100
 if([ComponentNativeInput]::GetAncestor([ComponentNativeInput]::WindowFromPoint($from),2) -ne $window){throw 'Fixture point is covered'}
 if($spec.action -eq 'wheel'){[ComponentNativeInput]::Send(0x800,-480)}else{
  [ComponentNativeInput]::Send(2);$pressed=$true
  if($spec.action -eq 'drag'){
   for($step=1;$step -le 10;$step++){
    if([ComponentNativeInput]::GetForegroundWindow() -ne $window){throw 'Fixture lost foreground during drag'}
    $point=[ComponentNativeInput+Point]::new();$point.x=$from.x+[int]($spec.dx*$step/10);$point.y=$from.y+[int]($spec.dy*$step/10)
    [ComponentNativeInput]::Move($point);Start-Sleep -Milliseconds 35
   }
  }
  [ComponentNativeInput]::Send(4);$pressed=$false
 }
 Start-Sleep -Milliseconds 150
 $after=[ComponentNativeInput+Rect]::new();[void][ComponentNativeInput]::GetWindowRect($window,[ref]$after)
 [IO.File]::WriteAllText($spec.resultPath,(@{action=$spec.action;before=$before;after=$after;inputElevated=$elevated}|ConvertTo-Json -Depth 3),[Text.UTF8Encoding]::new($false))
}finally{if($pressed){[ComponentNativeInput]::Send(4)};[void][ComponentNativeInput]::SetCursorPos($cursor.x,$cursor.y);$target.Dispose()}
