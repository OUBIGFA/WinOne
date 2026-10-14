param([Parameter(Mandatory)][string]$BundleDirectory,[Parameter(Mandatory)][string]$OutputDirectory,[switch]$RunWindowChecks)
$ErrorActionPreference='Stop'
$workspace=Split-Path -Parent $PSScriptRoot
if(-not $RunWindowChecks){throw 'Pass -RunWindowChecks to test only the isolated settings window'}
$output=[IO.Path]::GetFullPath((Join-Path $workspace $OutputDirectory))
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $output
& git -C $workspace check-ignore -q --no-index -- (Join-Path $output 'result.json')
if($LASTEXITCODE -ne 0 -or (Test-Path -LiteralPath $output)){throw 'Choose fresh ignored output'}
New-Item -ItemType Directory -Path $output|Out-Null;$bundle=Join-Path $output 'bundle'
Copy-Item -LiteralPath (Resolve-Path -LiteralPath $BundleDirectory).Path -Destination $bundle -Recurse
$profile=Join-Path $output 'profile';New-Item -ItemType Directory -Path $profile|Out-Null
[IO.File]::WriteAllText((Join-Path $profile 'settings.json'),'{"schemaVersion":1,"theme":"light","language":"zh-CN","captureShortcut":"","pinShortcut":"","startup":{"runAtLogon":false,"startMinimized":true}}',[Text.UTF8Encoding]::new($false))
Add-Type @'
using System;using System.Runtime.InteropServices;
public static class PositionProbe{
 [StructLayout(LayoutKind.Sequential)]public struct Point{public int x,y;}
 [StructLayout(LayoutKind.Sequential)]public struct Rect{public int left,top,right,bottom;}
 [StructLayout(LayoutKind.Sequential)]public struct Monitor{public int size;public Rect monitor,work;public int flags;}
 [StructLayout(LayoutKind.Sequential)]public struct Placement{public int length,flags,show;public Point minimum,maximum;public Rect normal;}
 [DllImport("user32.dll")]public static extern bool SetProcessDPIAware();
 [DllImport("user32.dll")]public static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int w,int height,uint flags);
 [DllImport("user32.dll")]public static extern bool GetWindowRect(IntPtr h,out Rect r);
 [DllImport("user32.dll")]public static extern bool GetClientRect(IntPtr h,out Rect r);
 [DllImport("user32.dll")]public static extern bool ClientToScreen(IntPtr h,ref Point p);
 [DllImport("user32.dll")]public static extern IntPtr SendMessageW(IntPtr h,uint m,IntPtr w,IntPtr l);
 [DllImport("user32.dll")]public static extern bool PostMessageW(IntPtr h,uint m,IntPtr w,IntPtr l);
 [DllImport("user32.dll")]public static extern bool IsZoomed(IntPtr h);
 [DllImport("user32.dll")]public static extern bool IsIconic(IntPtr h);
 [DllImport("user32.dll")]public static extern IntPtr MonitorFromWindow(IntPtr h,uint flags);
 [DllImport("user32.dll")]public static extern bool GetMonitorInfoW(IntPtr h,ref Monitor info);
 [DllImport("user32.dll")]public static extern bool GetWindowPlacement(IntPtr h,ref Placement placement);
}
'@
[void][PositionProbe]::SetProcessDPIAware()
$pipe='\\.\pipe\WinOne-position-'+[guid]::NewGuid().ToString('N');$previousTrace=$env:WINONE_DESKTOP_STARTUP_TRACE;$env:WINONE_DESKTOP_STARTUP_TRACE='1';$process=$null
function Open-TestWindow {
 $start=[Diagnostics.ProcessStartInfo]::new((Join-Path $bundle 'WinOne.exe'),'--standard-session --open-settings --pipe "'+$pipe+'" --data-dir "'+$profile+'"')
 $start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardError=$true
 $candidate=[Diagnostics.Process]::Start($start);$null=$candidate.Handle;$deadline=[DateTime]::UtcNow.AddSeconds(15)
 while($true){$line=$candidate.StandardError.ReadLineAsync();while(-not $line.Wait(100)){if([DateTime]::UtcNow -gt $deadline){throw 'Window did not become ready'}};if($null -eq $line.Result){throw 'Window exited early'};if($line.Result -match 'ms ui-ready$'){break}}
 $candidate.Refresh();if($candidate.MainWindowHandle -eq [IntPtr]::Zero){throw 'No visible owned window'}
 Add-Member -InputObject $candidate -NotePropertyName ProbeWindow -NotePropertyValue $candidate.MainWindowHandle
 return $candidate
}
function Read-TestRect($target){$r=New-Object PositionProbe+Rect;[void][PositionProbe]::GetWindowRect($target.ProbeWindow,[ref]$r);return @($r.left,$r.top,$r.right,$r.bottom)}
function Read-TestClientRect($target){$r=New-Object PositionProbe+Rect;$origin=New-Object PositionProbe+Point;[void][PositionProbe]::GetClientRect($target.ProbeWindow,[ref]$r);[void][PositionProbe]::ClientToScreen($target.ProbeWindow,[ref]$origin);return @($origin.x,$origin.y,($origin.x+$r.right),($origin.y+$r.bottom))}
function Close-TestWindow($target){if(-not $target.CloseMainWindow() -or -not $target.WaitForExit(10000)){throw 'Owned UI did not close'};if($target.ExitCode -ne 0){throw 'Owned UI failed'};$target.Dispose()}
function Set-TestWindowState($target,[string]$state){
 # Queue normal system commands on the window's UI thread, like taskbar
 # actions. Cross-process ShowWindow can overlap an in-flight tray restore.
 $command=switch($state){'minimize'{0xf020};'maximize'{0xf030};'restore'{0xf120};default{throw 'Unknown window action'}}
 if(-not [PositionProbe]::PostMessageW($target.ProbeWindow,0x112,[IntPtr]$command,[IntPtr]::Zero)){throw 'Window command failed'}
 $deadline=[DateTime]::UtcNow.AddSeconds(5)
 do{$min=[PositionProbe]::IsIconic($target.ProbeWindow);$max=[PositionProbe]::IsZoomed($target.ProbeWindow);$ready=switch($state){'minimize'{$min};'maximize'{$max -and -not $min};'restore'{-not $min -and -not $max}};if($ready){break};if([DateTime]::UtcNow -gt $deadline){throw 'Window state transition failed'};Start-Sleep -Milliseconds 20}while($true)
}
function Read-WorkArea($target){$info=New-Object PositionProbe+Monitor;$info.size=[Runtime.InteropServices.Marshal]::SizeOf($info);if(-not [PositionProbe]::GetMonitorInfoW([PositionProbe]::MonitorFromWindow($target.ProbeWindow,2),[ref]$info)){throw 'Could not read monitor work area'};return $info.work}
function Record-Placement($target,$name){$info=New-Object PositionProbe+Placement;$info.length=[Runtime.InteropServices.Marshal]::SizeOf($info);[void][PositionProbe]::GetWindowPlacement($target.ProbeWindow,[ref]$info);[IO.File]::WriteAllText((Join-Path $output ($name+'.json')),($info|ConvertTo-Json -Depth 4),[Text.UTF8Encoding]::new($false))}
function Wait-TestRect($target,$expected){
 $deadline=[DateTime]::UtcNow.AddSeconds(5)
 do{$actual=Read-TestRect $target;if(($actual -join ',') -eq ($expected -join ',')){return $actual};if([DateTime]::UtcNow -gt $deadline){throw "Window transition did not reach $expected; actual $actual"};Start-Sleep -Milliseconds 20}while($true)
}
function Wake-TestWindow($target,$expected,[bool]$maximized=$false){
 $start=[Diagnostics.ProcessStartInfo]::new((Join-Path $bundle 'WinOne.exe'),'--standard-session --open-settings --pipe "'+$pipe+'" --data-dir "'+$profile+'"');$start.UseShellExecute=$false;$start.CreateNoWindow=$true
 $wake=[Diagnostics.Process]::Start($start);$null=$wake.Handle
 try{if(-not $wake.WaitForExit(10000) -or $wake.ExitCode -ne 0){throw 'Existing-window notification failed'}}finally{$wake.Dispose()}
 $deadline=[DateTime]::UtcNow.AddSeconds(5)
 do{
  $ready=-not [PositionProbe]::IsIconic($target.ProbeWindow)
  if($maximized){$ready=$ready -and [PositionProbe]::IsZoomed($target.ProbeWindow)}else{$ready=$ready -and (((Read-TestRect $target) -join ',') -eq ($expected -join ','))}
  if($ready){break};if([DateTime]::UtcNow -gt $deadline){throw 'Existing window did not finish restoring'};Start-Sleep -Milliseconds 20
 }while($true)
 if($target.HasExited){throw 'Opening from tray replaced the existing process'}
}
try{
 $process=Open-TestWindow
 [void][PositionProbe]::SetWindowPos($process.ProbeWindow,[IntPtr]::Zero,180,140,960,640,0x14)
 [void][PositionProbe]::SendMessageW($process.ProbeWindow,0x232,[IntPtr]::Zero,[IntPtr]::Zero)
 $expected=Read-TestRect $process
 Set-TestWindowState $process 'minimize';Wake-TestWindow $process $expected
 $woken=Read-TestRect $process
 if(($woken -join ',') -ne ($expected -join ',')){throw "Existing minimized window lost its position: $woken, expected $expected"}
 Close-TestWindow $process;$process=$null
 $process=Open-TestWindow;$actual=Read-TestRect $process
 [IO.File]::WriteAllText((Join-Path $output 'result.json'),(@{Expected=$expected;Actual=$actual;Preserved=(($expected -join ',') -eq ($actual -join ','))}|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
 if(($expected -join ',') -ne ($actual -join ',')){throw "Reopen forgot window placement: expected $expected, got $actual"}
 Set-TestWindowState $process 'maximize'
 Close-TestWindow $process;$process=$null
 $process=Open-TestWindow
 if(-not [PositionProbe]::IsZoomed($process.ProbeWindow)){throw 'Maximized state was not restored'}
 $work=Read-WorkArea $process
 # GetWindowRect includes Windows' invisible resize border when maximized.
 # The actual client surface must fit the monitor's work area exactly.
 $maximized=Read-TestClientRect $process
 if(($maximized -join ',') -ne (@($work.left,$work.top,$work.right,$work.bottom) -join ',')){throw "Maximized window exceeded the work area: $maximized"}
 Record-Placement $process 'before-maximize-minimize'
 Set-TestWindowState $process 'minimize'
 Record-Placement $process 'after-maximize-minimize'
 Wake-TestWindow $process $expected $true
 Record-Placement $process 'after-maximize-wake'
 if(-not [PositionProbe]::IsZoomed($process.ProbeWindow)){throw 'Tray reopen lost the maximized state'}
 Set-TestWindowState $process 'restore'
 $restored=Wait-TestRect $process $expected
 if(($expected -join ',') -ne ($restored -join ',')){throw "Maximize/restore changed normal position: $restored, expected $expected"}
 Set-TestWindowState $process 'minimize'
 Close-TestWindow $process;$process=$null
 $process=Open-TestWindow;$restored=Read-TestRect $process
 if(($expected -join ',') -ne ($restored -join ',')){throw 'Minimize/close lost normal position'}
 Close-TestWindow $process;$process=$null
 $file=Join-Path $profile 'settings-window.json';$saved=Get-Content -LiteralPath $file -Raw -Encoding UTF8|ConvertFrom-Json
 $saved.normal=@(100000,100000,100960,100640);$saved.maximized=$false
 [IO.File]::WriteAllText($file,($saved|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
 $process=Open-TestWindow;$visible=Read-TestRect $process;$work=Read-WorkArea $process
 if($visible[0] -lt $work.left -or $visible[1] -lt $work.top -or $visible[2] -gt $work.right -or $visible[3] -gt $work.bottom){throw "Off-screen placement was not recovered: $visible"}
 Close-TestWindow $process;$process=$null
 [IO.File]::WriteAllText((Join-Path $output 'result.json'),(@{Expected=$expected;Reopened=$actual;Maximized=$maximized;Restored=$restored;OffScreenRecovered=$visible;Passed=$true}|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
 'PASS: position, size, maximized/minimized state, and off-screen recovery'
}finally{
 if($process){if(-not $process.HasExited){[void]$process.CloseMainWindow();if(-not $process.WaitForExit(3000)){$process.Kill();$process.WaitForExit()}};$process.Dispose()}
 Get-CimInstance Win32_Process -Filter "Name = 'WinOne.Host.exe'"|Where-Object {$_.ExecutablePath -eq (Join-Path $bundle 'WinOne.Host.exe') -and $_.CommandLine.Contains($profile)}|ForEach-Object {Stop-Process -Id $_.ProcessId}
 $env:WINONE_DESKTOP_STARTUP_TRACE=$previousTrace
}
