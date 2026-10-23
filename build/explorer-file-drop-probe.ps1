# Real Explorer source, restricted to a new fixture tab and its own paths.
param([Parameter(Mandatory)][string]$Specification)
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$spec=Get-Content -LiteralPath $Specification -Raw -Encoding UTF8 | ConvertFrom-Json
$root=[IO.Path]::GetFullPath($spec.taskRoot).TrimEnd([char]92)+[IO.Path]::DirectorySeparatorChar
& (Join-Path $PSScriptRoot 'assert-temporary-path.ps1') -Path $root
foreach($path in @($Specification,$spec.targetExe,$spec.resultPath)+@($spec.paths)){
 if(-not [IO.Path]::GetFullPath($path).StartsWith($root,[StringComparison]::OrdinalIgnoreCase)){throw 'Path is outside this fixture'}
}
$inputElevated=[Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if($inputElevated -ne [bool]$spec.expectedTargetElevated){throw 'Input driver must match target integrity; the Explorer source is verified separately'}
Add-Type -AssemblyName UIAutomationClient,UIAutomationTypes,WindowsBase
Add-Type -TypeDefinition @'
using System;using System.Text;using System.Runtime.InteropServices;
public static class ExplorerDropInput {
 [StructLayout(LayoutKind.Sequential)] public struct Point {public int x,y;}
 [StructLayout(LayoutKind.Sequential)] public struct Rect {public int left,top,right,bottom;}
 [StructLayout(LayoutKind.Sequential)] public struct Placement {public int size,flags,show;public Point min,max;public Rect normal;}
 [StructLayout(LayoutKind.Sequential)] struct Mouse {public int x,y;public uint data,flags,time;public IntPtr extra;}
 [StructLayout(LayoutKind.Sequential)] struct Key {public ushort code,scan;public uint flags,time;public IntPtr extra;}
 [StructLayout(LayoutKind.Explicit)] struct Union {[FieldOffset(0)]public Mouse mouse;[FieldOffset(0)]public Key key;}
 [StructLayout(LayoutKind.Sequential)]struct Input {public uint type;public Union value;}
 [DllImport("user32.dll")]public static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")]public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
 [DllImport("user32.dll")]public static extern bool AttachThreadInput(uint from,uint to,bool attach);
 [DllImport("kernel32.dll")]public static extern uint GetCurrentThreadId();
 [DllImport("user32.dll")]public static extern bool SetForegroundWindow(IntPtr window);
 [DllImport("user32.dll")]public static extern bool ShowWindow(IntPtr window,int command);
 [DllImport("user32.dll")]public static extern bool IsWindowVisible(IntPtr window);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]public static extern IntPtr FindWindowExW(IntPtr parent,IntPtr after,string name,string title);
 [DllImport("user32.dll")]public static extern bool GetWindowPlacement(IntPtr window,ref Placement placement);
 [DllImport("user32.dll")]public static extern bool SetWindowPlacement(IntPtr window,ref Placement placement);
 [DllImport("user32.dll")]public static extern int GetWindowLongW(IntPtr window,int index);
 [DllImport("dwmapi.dll")]static extern int DwmGetWindowAttribute(IntPtr window,uint attribute,out int value,int size);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern int GetClassNameW(IntPtr window,StringBuilder name,int length);
 public static object Inspect(IntPtr window){uint pid;GetWindowThreadProcessId(window,out pid);int cloaked;DwmGetWindowAttribute(window,14,out cloaked,4);var name=new StringBuilder(128);GetClassNameW(window,name,128);return new{handle=window.ToInt64(),pid=pid,name=name.ToString(),style=GetWindowLongW(window,-16),exStyle=GetWindowLongW(window,-20),cloaked=cloaked};}
 [DllImport("user32.dll")]public static extern bool SetWindowPos(IntPtr window,IntPtr after,int x,int y,int w,int h,uint flags);
 [DllImport("user32.dll")]public static extern bool GetWindowRect(IntPtr window,out Rect rect);
 [DllImport("user32.dll")]public static extern bool ClientToScreen(IntPtr window,ref Point point);
 [DllImport("user32.dll")]public static extern bool GetCursorPos(out Point point);
 [DllImport("user32.dll")]public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")]public static extern IntPtr WindowFromPoint(Point point);
 [DllImport("user32.dll")]public static extern IntPtr GetAncestor(IntPtr window,uint flags);
 [DllImport("user32.dll")]public static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
 [DllImport("user32.dll")]public static extern short GetAsyncKeyState(int key);
 [DllImport("user32.dll")]static extern int GetSystemMetrics(int index);
 [DllImport("user32.dll",SetLastError=true)]static extern uint SendInput(uint count,Input[] input,int size);
 public static void Escape(){var down=new Input{type=1,value=new Union{key=new Key{code=27}}};var up=new Input{type=1,value=new Union{key=new Key{code=27,flags=2}}};if(SendInput(2,new[]{down,up},Marshal.SizeOf(typeof(Input)))!=2){SendInput(1,new[]{up},Marshal.SizeOf(typeof(Input)));throw new System.ComponentModel.Win32Exception();}}
 [DllImport("kernel32.dll",SetLastError=true)]static extern IntPtr OpenProcess(uint access,bool inherit,int pid);
 [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)]static extern bool QueryFullProcessImageName(IntPtr process,uint flags,StringBuilder path,ref int count);
 [DllImport("kernel32.dll")]static extern bool CloseHandle(IntPtr handle);
 [DllImport("advapi32.dll",SetLastError=true)]static extern bool OpenProcessToken(IntPtr process,uint access,out IntPtr token);
 [DllImport("advapi32.dll",SetLastError=true)]static extern bool GetTokenInformation(IntPtr token,int kind,out int value,int size,out int needed);
 public static bool Verify(int pid,string expected){var process=OpenProcess(0x1000,false,pid);if(process==IntPtr.Zero)throw new System.ComponentModel.Win32Exception();IntPtr token=IntPtr.Zero;try{var path=new StringBuilder(32768);int count=path.Capacity;if(!QueryFullProcessImageName(process,0,path,ref count)||!String.Equals(path.ToString(),expected,StringComparison.OrdinalIgnoreCase))throw new Exception("Unexpected process identity");if(!OpenProcessToken(process,8,out token))throw new System.ComponentModel.Win32Exception();int value,needed;if(!GetTokenInformation(token,20,out value,4,out needed))throw new System.ComponentModel.Win32Exception();return value!=0;}finally{if(token!=IntPtr.Zero)CloseHandle(token);CloseHandle(process);}}
 public static void MouseEvent(uint flags,int x,int y){if((flags&1)!=0){int left=GetSystemMetrics(76),top=GetSystemMetrics(77),width=GetSystemMetrics(78),height=GetSystemMetrics(79);x=(int)((long)(x-left)*65535/(width-1));y=(int)((long)(y-top)*65535/(height-1));flags|=0xc000;}var input=new Input{type=0,value=new Union{mouse=new Mouse{x=x,y=y,flags=flags}}};if(SendInput(1,new[]{input},Marshal.SizeOf(typeof(Input)))!=1)throw new System.ComponentModel.Win32Exception();}
}
'@
[void][ExplorerDropInput]::SetThreadDpiAwarenessContext([IntPtr]::new(-4))
$target=[Diagnostics.Process]::GetProcessById($spec.targetPid)
$targetWindow=$target.MainWindowHandle
$elevated=[ExplorerDropInput]::Verify($spec.targetPid,[IO.Path]::GetFullPath($spec.targetExe))
if($targetWindow -eq [IntPtr]::Zero -or $elevated -ne $spec.expectedTargetElevated){throw 'Target identity/elevation mismatch'}
$folder=Split-Path -Parent $spec.paths[0]
foreach($path in $spec.paths){if(-not(Test-Path -LiteralPath $path) -or (Split-Path -Parent $path) -ne $folder){throw 'All dragged paths must exist in one fixture folder'}}
$shell=New-Object -ComObject Shell.Application
$before=@($shell.Windows() | ForEach-Object {[long][ExplorerDropInput]::GetAncestor([IntPtr][long]$_.HWND,2)})
$originalFrames=@{}
foreach($handle in $before | Select-Object -Unique){$saved=[ExplorerDropInput+Placement]::new();$saved.size=[Runtime.InteropServices.Marshal]::SizeOf($saved);if([ExplorerDropInput]::GetWindowPlacement([IntPtr]$handle,[ref]$saved)){$originalFrames[$handle]=@{placement=$saved;topmost=([ExplorerDropInput]::GetWindowLongW([IntPtr]$handle,-20) -band 8) -ne 0}}}
foreach($entry in $shell.Windows()){if($entry.Document.Folder.Self.Path -eq $folder){throw 'Fixture folder already has an Explorer tab'}}
$window=$null;$pressed=$false;$placement=$null;$sourceWindow=[IntPtr]::Zero;$cursor=[ExplorerDropInput+Point]::new()
[void][ExplorerDropInput]::GetCursorPos([ref]$cursor)
try {
 # Windows or a tab extension may merge a new window into an existing frame.
 # Bind to the unique new fixture folder, then let its HWND settle. Quit only
 # this IWebBrowser tab; preserve and restore the containing frame placement.
 $shell.ShellExecute($folder,'','','opennewwindow',1)
 $until=[DateTime]::UtcNow.AddSeconds(10)
 while(-not $window -and [DateTime]::UtcNow -lt $until){
  foreach($entry in $shell.Windows()){try{if($entry.Document.Folder.Self.Path -eq $folder){$window=$entry;break}}catch{}}
  if(-not $window){Start-Sleep -Milliseconds 100}
 }
 if(-not $window){throw 'A new owned Explorer window was not found'}
 # Merging replaces the IWebBrowser object as well as its HWND.
 $window=$null
 # A tab merger briefly exposes both the discarded frame and the real tab
 # for the same path. Prefer the existing frame; the discarded COM object
 # remains queryable but cannot receive physical mouse input.
 $mergeDeadline=[DateTime]::UtcNow.AddSeconds(5)
 do{
  $merged=$false;$window=$null
  foreach($entry in $shell.Windows()){try{if($entry.Document.Folder.Self.Path -eq $folder){$window=$entry;if([long][ExplorerDropInput]::GetAncestor([IntPtr][long]$entry.HWND,2) -in $before){$merged=$true;break}}}catch{}}
  if(-not $merged){Start-Sleep -Milliseconds 100}
 }while(-not $merged -and [DateTime]::UtcNow -lt $mergeDeadline)
 if(-not $window){throw 'The fixture Explorer tab disappeared during creation'}
 $sourceWindow=[ExplorerDropInput]::GetAncestor([IntPtr][long]$window.HWND,2)
 $placement=[ExplorerDropInput+Placement]::new();$placement.size=[Runtime.InteropServices.Marshal]::SizeOf($placement)
 if(-not [ExplorerDropInput]::GetWindowPlacement($sourceWindow,[ref]$placement)){throw 'Cannot preserve Explorer window placement'}
 $wasTopmost=([ExplorerDropInput]::GetWindowLongW($sourceWindow,-20) -band 8) -ne 0
 [uint32]$sourcePID=0;[void][ExplorerDropInput]::GetWindowThreadProcessId($sourceWindow,[ref]$sourcePID)
 if([ExplorerDropInput]::Verify($sourcePID,(Join-Path $env:WINDIR 'explorer.exe'))){throw 'Explorer is unexpectedly elevated'}
 $rect=[ExplorerDropInput+Rect]::new();[void][ExplorerDropInput]::GetWindowRect($targetWindow,[ref]$rect)
 # The hidden PowerShell driver must not pass SW_HIDE to the visible source.
 [void][ExplorerDropInput]::ShowWindow($sourceWindow,9)
 $destinationX=$rect.left+[int]($spec.x*$spec.scale)
 # Leave only a narrow overlap: a high-integrity topmost target must not
 # cover the selected source item even when foreground activation is denied.
 $sourceX=if($destinationX -lt ($rect.left+$rect.right)/2){$rect.right-40}else{$rect.left-450}
 $positioned=[ExplorerDropInput]::SetWindowPos($sourceWindow,[IntPtr]::new(-1),$sourceX,$rect.top+45,490,520,0)
 [uint32]$ignoredPID=0;$foregroundThread=[ExplorerDropInput]::GetWindowThreadProcessId([ExplorerDropInput]::GetForegroundWindow(),[ref]$ignoredPID);$currentThread=[ExplorerDropInput]::GetCurrentThreadId()
 $attached=$currentThread -ne $foregroundThread -and [ExplorerDropInput]::AttachThreadInput($currentThread,$foregroundThread,$true)
 try{$activated=[ExplorerDropInput]::SetForegroundWindow($sourceWindow)}finally{if($attached){[void][ExplorerDropInput]::AttachThreadInput($currentThread,$foregroundThread,$false)}}
 $sourceRect=[ExplorerDropInput+Rect]::new();[void][ExplorerDropInput]::GetWindowRect($sourceWindow,[ref]$sourceRect)
 [IO.File]::WriteAllText(($spec.resultPath+'.window.json'),(@{positioned=$positioned;activated=$activated;visible=[ExplorerDropInput]::IsWindowVisible($sourceWindow);sourceRect=$sourceRect;targetRect=$rect;foreground=[long][ExplorerDropInput]::GetForegroundWindow();before=$before} | ConvertTo-Json -Depth 3),[Text.UTF8Encoding]::new($false))
 for($i=0;$i -lt $spec.paths.Count;$i++){$item=$window.Document.Folder.ParseName([IO.Path]::GetFileName($spec.paths[$i]));$window.Document.SelectItem($item,$(if($i -eq 0){29}else{1}))}
 $element=[System.Windows.Automation.AutomationElement]::FromHandle($sourceWindow)
 $condition=[System.Windows.Automation.AndCondition]::new([System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.SelectionItemPattern]::IsSelectedProperty,$true),[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ControlTypeProperty,[System.Windows.Automation.ControlType]::ListItem))
 $selected=$null;$until=[DateTime]::UtcNow.AddSeconds(4)
 while(-not $selected -and [DateTime]::UtcNow -lt $until){$selected=$element.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$condition);if(-not $selected){Start-Sleep -Milliseconds 100}}
 if(-not $selected){throw 'Selected fixture item is not visible in Explorer'}
 $bounds=$selected.Current.BoundingRectangle
 $from=[ExplorerDropInput+Point]::new();$from.x=[int]($bounds.Left+[Math]::Min(70,$bounds.Width/2));$from.y=[int]($bounds.Top+$bounds.Height/2)
 # Some tab mergers keep IWebBrowser.HWND on a hidden surrogate even after
 # moving its Shell view into the existing frame. Resolve the physical frame
 # only when its selected item is the same fixture view we just selected.
 $physical=[ExplorerDropInput]::GetAncestor([ExplorerDropInput]::WindowFromPoint($from),2)
 if($physical -ne $sourceWindow -and [long]$physical -in $before){
  $actual=[System.Windows.Automation.AutomationElement]::FromHandle($physical).FindFirst([System.Windows.Automation.TreeScope]::Descendants,$condition)
  $selectedPaths=@($window.Document.SelectedItems() | ForEach-Object {$_.Path})
  if($actual -and $actual.Current.Name -eq $selected.Current.Name -and $selectedPaths.Count -eq $spec.paths.Count -and @($selectedPaths | Where-Object {$_ -notin $spec.paths}).Count -eq 0){
   $sourceWindow=$physical;$placement=$originalFrames[[long]$physical].placement;$wasTopmost=$originalFrames[[long]$physical].topmost
   [void][ExplorerDropInput]::SetForegroundWindow($sourceWindow)
  }
 }
 $to=[ExplorerDropInput+Point]::new();[void][ExplorerDropInput]::ClientToScreen($targetWindow,[ref]$to);$to.x+=[int]($spec.x*$spec.scale);$to.y+=[int]($spec.y*$spec.scale)
 [IO.File]::WriteAllText(($spec.resultPath+'.positions.json'),(@{source=[long]$sourceWindow;target=[long]$targetWindow;from=@($from.x,$from.y);to=@($to.x,$to.y);fromWindow=[long][ExplorerDropInput]::GetAncestor([ExplorerDropInput]::WindowFromPoint($from),2);toWindow=[long][ExplorerDropInput]::GetAncestor([ExplorerDropInput]::WindowFromPoint($to),2);selected=$selected.Current.Name;offscreen=$selected.Current.IsOffscreen} | ConvertTo-Json),[Text.UTF8Encoding]::new($false))
 if(([ExplorerDropInput]::GetAsyncKeyState(1) -band 0x8000) -ne 0){throw 'User mouse button is held'}
 [IO.File]::WriteAllText(($spec.resultPath+'.occlusion.json'),(@{source=[ExplorerDropInput]::Inspect($sourceWindow);cover=[ExplorerDropInput]::Inspect([ExplorerDropInput]::GetAncestor([ExplorerDropInput]::WindowFromPoint($from),2))} | ConvertTo-Json -Depth 3),[Text.UTF8Encoding]::new($false))
 if([ExplorerDropInput]::GetAncestor([ExplorerDropInput]::WindowFromPoint($from),2) -ne $sourceWindow -or [ExplorerDropInput]::GetAncestor([ExplorerDropInput]::WindowFromPoint($to),2) -ne $targetWindow){throw 'Source/destination point is not over its owned window'}
 [ExplorerDropInput]::MouseEvent(1,$from.x,$from.y);Start-Sleep -Milliseconds 150
 $actualCursor=[ExplorerDropInput+Point]::new();[void][ExplorerDropInput]::GetCursorPos([ref]$actualCursor)
 [IO.File]::WriteAllText(($spec.resultPath+'.cursor.json'),(@{requested=$from;actual=$actualCursor} | ConvertTo-Json -Depth 3),[Text.UTF8Encoding]::new($false))
 if([Math]::Abs($actualCursor.x-$from.x) -gt 2 -or [Math]::Abs($actualCursor.y-$from.y) -gt 2){throw 'Injected cursor position differs from the selected fixture item'}
 # SetForegroundWindow can be denied to a hidden runner. A real click on
 # this already-selected fixture item activates Explorer without changing
 # any user selection or relying on cross-integrity foreground permission.
 if([ExplorerDropInput]::GetForegroundWindow() -ne $sourceWindow){
  [ExplorerDropInput]::MouseEvent(2,0,0);$pressed=$true
  [ExplorerDropInput]::MouseEvent(4,0,0);$pressed=$false;Start-Sleep -Milliseconds 200
  for($i=0;$i -lt $spec.paths.Count;$i++){$item=$window.Document.Folder.ParseName([IO.Path]::GetFileName($spec.paths[$i]));$window.Document.SelectItem($item,$(if($i -eq 0){29}else{1}))}
 }
 $foreground=[ExplorerDropInput]::GetForegroundWindow()
 if($foreground -ne $sourceWindow -and [ExplorerDropInput]::GetAncestor($foreground,3) -ne $sourceWindow){throw "Explorer lost focus before drag: $foreground"}
 if([ExplorerDropInput]::GetAncestor([ExplorerDropInput]::WindowFromPoint($from),2) -ne $sourceWindow){throw 'Explorer was covered before mouse-down'}
 [ExplorerDropInput]::MouseEvent(2,0,0);$pressed=$true
 for($step=1;$step -le 12;$step++){
  $foreground=[ExplorerDropInput]::GetForegroundWindow();$foregroundOwner=[ExplorerDropInput]::GetAncestor($foreground,3)
  if($foreground -notin @($sourceWindow,$targetWindow) -and $foregroundOwner -notin @($sourceWindow,$targetWindow)){throw ("Input guard stopped after focus loss: step=$step "+([ExplorerDropInput]::Inspect($foreground)|ConvertTo-Json -Compress))}
  $point=[ExplorerDropInput+Point]::new();$point.x=$from.x+($to.x-$from.x)*$step/12;$point.y=$from.y+($to.y-$from.y)*$step/12
  if([ExplorerDropInput]::GetAncestor([ExplorerDropInput]::WindowFromPoint($point),2) -notin @($sourceWindow,$targetWindow)){throw 'Drag path left the owned windows'}
  [ExplorerDropInput]::MouseEvent(1,$point.x,$point.y);Start-Sleep -Milliseconds 90
 }
 $surface=[ExplorerDropInput]::FindWindowExW($targetWindow,[IntPtr]::Zero,'WinOne.FileDropTarget',$null)
 $surfaceDuringDrag=[ExplorerDropInput]::IsWindowVisible($surface)
 if($spec.cancel){
  # Cancel the real Shell drag while still over the target. The receiver must
  # disappear on capture end even when no WM_DROPFILES message is delivered.
  if($elevated -and -not $surfaceDuringDrag){throw 'Elevated drop surface was not visible during drag'}
  [ExplorerDropInput]::Escape();Start-Sleep -Milliseconds 150
 }
 [ExplorerDropInput]::MouseEvent(4,0,0);$pressed=$false;Start-Sleep -Milliseconds 250
 $hiddenDeadline=[DateTime]::UtcNow.AddSeconds(3)
 while([ExplorerDropInput]::IsWindowVisible($surface) -and [DateTime]::UtcNow -lt $hiddenDeadline){Start-Sleep -Milliseconds 50}
 $surfaceAfterDrag=[ExplorerDropInput]::IsWindowVisible($surface)
 if($surfaceAfterDrag){throw 'Drop surface remained visible after drag ended'}
 $result=@{state='completed';source='Explorer';sourcePid=$sourcePID;targetPid=$spec.targetPid;sourceElevated=$false;inputElevated=$inputElevated;targetElevated=$elevated;mousePressed=$false;cancelled=[bool]$spec.cancel;surfaceDuringDrag=$surfaceDuringDrag;surfaceAfterDrag=$surfaceAfterDrag;error=''}
 [IO.File]::WriteAllText($spec.resultPath,($result|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
} finally {
 if($pressed){[ExplorerDropInput]::MouseEvent(4,0,0)}
 foreach($entry in $shell.Windows()){try{$owned=$entry.Document.Folder.Self.Path -eq $folder}catch{$owned=$false};if($owned){$entry.Quit()}}
 if($placement -and [long]$sourceWindow -in $before){[void][ExplorerDropInput]::SetWindowPlacement($sourceWindow,[ref]$placement)}
 if($placement -and -not $wasTopmost){[void][ExplorerDropInput]::SetWindowPos($sourceWindow,[IntPtr]::new(-2),0,0,0,0,0x13)}
 [void][ExplorerDropInput]::SetCursorPos($cursor.x,$cursor.y)
 $target.Dispose()
}
