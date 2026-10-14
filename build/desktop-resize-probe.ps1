param([int]$TargetPid,[string]$OutputPath,[switch]$ResizeWindow,[switch]$Drag,[int]$Width=800,[int]$Height=560)
$ErrorActionPreference='Stop'
Add-Type @'
using System;using System.Runtime.InteropServices;
public static class ResizeCursorProbe{
 [StructLayout(LayoutKind.Sequential)]public struct Point{public int x,y;}
 [StructLayout(LayoutKind.Sequential)]public struct Rect{public int left,top,right,bottom;}
 [StructLayout(LayoutKind.Sequential)]public struct Cursor{public int size,flags;public IntPtr cursor;public Point point;}
 [DllImport("user32.dll")]public static extern bool SetProcessDPIAware();
 [DllImport("user32.dll")]public static extern bool SetForegroundWindow(IntPtr h);
 [DllImport("user32.dll")]public static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")]public static extern IntPtr WindowFromPoint(Point p);
 [DllImport("user32.dll")]public static extern IntPtr GetAncestor(IntPtr h,uint flags);
 [DllImport("user32.dll")]public static extern bool GetCursorPos(out Point p);
 [DllImport("user32.dll")]public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")]public static extern bool GetWindowRect(IntPtr h,out Rect r);
 [DllImport("user32.dll")]public static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int width,int height,uint flags);
 [DllImport("user32.dll")]public static extern bool GetCursorInfo(ref Cursor c);
 [DllImport("user32.dll")]public static extern IntPtr LoadCursorW(IntPtr h,int id);
 [DllImport("user32.dll")]public static extern void mouse_event(uint flags,uint x,uint y,uint data,UIntPtr extra);
}
'@
[void][ResizeCursorProbe]::SetProcessDPIAware()
$h=(Get-Process -Id $TargetPid).MainWindowHandle
if($h -eq [IntPtr]::Zero){throw 'Owned window unavailable'}
if($ResizeWindow){[void][ResizeCursorProbe]::SetWindowPos($h,[IntPtr]::Zero,100,100,$Width,$Height,4)}
$r=New-Object ResizeCursorProbe+Rect;[void][ResizeCursorProbe]::GetWindowRect($h,[ref]$r)
$before=New-Object ResizeCursorProbe+Point;[void][ResizeCursorProbe]::GetCursorPos([ref]$before)
[void][ResizeCursorProbe]::SetForegroundWindow($h)
[void][ResizeCursorProbe]::SetWindowPos($h,[IntPtr](-1),0,0,0,0,0x13)
$w=$r.right-$r.left;$height=$r.bottom-$r.top
$points=@(@('nw',2,2,32642),@('n',[int]($w/2),2,32645),@('ne',($w-3),2,32643),@('w',2,[int]($height/2),32644),@('e',($w-3),[int]($height/2),32644),@('sw',2,($height-3),32643),@('s',[int]($w/2),($height-3),32645),@('se',($w-3),($height-3),32642))
$results=@()
try{foreach($point in $points){
 [void][ResizeCursorProbe]::SetCursorPos($r.left+[int]($w/2),$r.top+[int]($height/2));Start-Sleep -Milliseconds 30
 [void][ResizeCursorProbe]::SetCursorPos($r.left+[int]$point[1],$r.top+[int]$point[2]);Start-Sleep -Milliseconds 100
 $c=New-Object ResizeCursorProbe+Cursor;$c.size=[Runtime.InteropServices.Marshal]::SizeOf($c);[void][ResizeCursorProbe]::GetCursorInfo([ref]$c)
 $expected=[ResizeCursorProbe]::LoadCursorW([IntPtr]::Zero,[int]$point[3])
 $foreground=([ResizeCursorProbe]::GetForegroundWindow() -eq $h)
 $overWindow=([ResizeCursorProbe]::GetAncestor([ResizeCursorProbe]::WindowFromPoint($c.point),2) -eq $h)
 $dragged=$null
 if($Drag -and $expected -eq $c.cursor){
  $dx=if($point[0].Contains('w')){-20}elseif($point[0].Contains('e')){20}else{0}
  $dy=if($point[0].Contains('n')){-20}elseif($point[0].Contains('s')){20}else{0}
  [ResizeCursorProbe]::mouse_event(2,0,0,0,[UIntPtr]::Zero);Start-Sleep -Milliseconds 30
  [void][ResizeCursorProbe]::SetCursorPos(($r.left+[int]$point[1]+$dx),($r.top+[int]$point[2]+$dy));Start-Sleep -Milliseconds 100
  [ResizeCursorProbe]::mouse_event(4,0,0,0,[UIntPtr]::Zero);Start-Sleep -Milliseconds 50
  $after=New-Object ResizeCursorProbe+Rect;[void][ResizeCursorProbe]::GetWindowRect($h,[ref]$after)
  $dragged=($dx -eq 0 -or ($after.right-$after.left) -ge ($w+12)) -and ($dy -eq 0 -or ($after.bottom-$after.top) -ge ($height+12))
  [void][ResizeCursorProbe]::SetWindowPos($h,[IntPtr]::Zero,$r.left,$r.top,$w,$height,4);Start-Sleep -Milliseconds 80
 }
 $results+=[pscustomobject]@{Edge=$point[0];Expected=$expected.ToInt64();Actual=$c.cursor.ToInt64();Pass=($expected -eq $c.cursor);Dragged=$dragged;Foreground=$foreground;OverWindow=$overWindow;X=$c.point.x;Y=$c.point.y;TargetX=$r.left+[int]$point[1];TargetY=$r.top+[int]$point[2]}
}}finally{[ResizeCursorProbe]::mouse_event(4,0,0,0,[UIntPtr]::Zero);[void][ResizeCursorProbe]::SetWindowPos($h,[IntPtr](-2),0,0,0,0,0x13);[void][ResizeCursorProbe]::SetCursorPos($before.x,$before.y)}
[IO.File]::WriteAllText($OutputPath,($results|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
