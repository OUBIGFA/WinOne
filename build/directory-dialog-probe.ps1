param(
 [Parameter(Mandatory)][int]$OwnerPID,
 [Parameter(Mandatory)][string]$ExpectedExe,
 [Parameter(Mandatory)][string]$TestRoot,
 [Parameter(Mandatory)][string]$Path,
 [ValidateSet('folder','file')][string]$Kind='folder'
)
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$root=[IO.Path]::GetFullPath($TestRoot).TrimEnd('\')+'\'
$selected=[IO.Path]::GetFullPath($Path)
if(-not $selected.StartsWith($root,[StringComparison]::OrdinalIgnoreCase) -or -not [IO.Path]::GetFullPath($ExpectedExe).StartsWith($root,[StringComparison]::OrdinalIgnoreCase)){throw 'The dialog and selected path must belong to this isolated test'}
if(-not(Test-Path -LiteralPath $selected)){throw 'The test selection does not exist'}
$ownerProcess=Get-Process -Id $OwnerPID
if($ownerProcess.Path -ne [IO.Path]::GetFullPath($ExpectedExe)){throw 'The dialog owner does not match the isolated executable'}
$null=$ownerProcess.Handle
Add-Type -AssemblyName UIAutomationClient,UIAutomationTypes
Add-Type @'
using System;using System.Text;using System.Collections.Generic;using System.Runtime.InteropServices;
public static class OwnedDialogState {
 public delegate bool Visit(IntPtr h,IntPtr p);
 [DllImport("user32.dll")]public static extern bool IsWindow(IntPtr h);
 [DllImport("user32.dll")]public static extern bool IsWindowEnabled(IntPtr h);
 [DllImport("user32.dll")]static extern bool IsWindowVisible(IntPtr h);
 [DllImport("user32.dll")]static extern bool EnumWindows(Visit v,IntPtr p);
 [DllImport("user32.dll")]static extern bool EnumChildWindows(IntPtr h,Visit v,IntPtr p);
 [DllImport("user32.dll")]static extern uint GetWindowThreadProcessId(IntPtr h,out uint pid);
 [DllImport("user32.dll")]static extern int GetDlgCtrlID(IntPtr h);
 [DllImport("user32.dll")]static extern IntPtr GetDlgItem(IntPtr h,int id);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern int GetClassName(IntPtr h,StringBuilder s,int n);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern IntPtr SendMessage(IntPtr h,uint m,IntPtr w,string text);
 [DllImport("user32.dll",EntryPoint="SendMessageW")]static extern IntPtr SendCommand(IntPtr h,uint m,IntPtr w,IntPtr l);
 static string Class(IntPtr h){var s=new StringBuilder(256);GetClassName(h,s,256);return s.ToString();}
 public static IntPtr Find(uint owner){IntPtr found=IntPtr.Zero;Visit check=(h,p)=>{uint pid;GetWindowThreadProcessId(h,out pid);if(pid==owner&&IsWindowVisible(h)&&Class(h)=="#32770"){found=h;return false;}return true;};EnumWindows(check,IntPtr.Zero);if(found!=IntPtr.Zero)return found;EnumWindows((h,p)=>{EnumChildWindows(h,check,IntPtr.Zero);return found==IntPtr.Zero;},IntPtr.Zero);return found;}
 public static IntPtr Button(IntPtr root){IntPtr found=IntPtr.Zero;EnumChildWindows(root,(h,p)=>{if(GetDlgCtrlID(h)==1&&Class(h)=="Button"&&IsWindowVisible(h)){found=h;return false;}return true;},IntPtr.Zero);return found;}
 public static IntPtr Edit(IntPtr root){foreach(int id in new[]{1148,1152}){var field=GetDlgItem(root,id);if(field==IntPtr.Zero)continue;if(Class(field)=="Edit")return field;IntPtr edit=IntPtr.Zero;EnumChildWindows(field,(h,p)=>{if(Class(h)=="Edit"){edit=h;return false;}return true;},IntPtr.Zero);if(edit!=IntPtr.Zero)return edit;}return IntPtr.Zero;}
 public static void SetText(IntPtr h,string text){if(SendMessage(h,12,IntPtr.Zero,text)==IntPtr.Zero)throw new Exception("Native filename edit rejected text");}
 public static void Click(IntPtr h){if(!IsWindow(h)||!IsWindowEnabled(h))throw new Exception("Native confirmation button unavailable");SendCommand(h,0xf5,IntPtr.Zero,IntPtr.Zero);}
 public static string Describe(uint owner){var rows=new List<string>();EnumWindows((h,p)=>{uint pid;GetWindowThreadProcessId(h,out pid);if(pid==owner)rows.Add(h+":"+Class(h)+":"+IsWindowVisible(h));return true;},IntPtr.Zero);return String.Join(";",rows);}
 public static string Controls(IntPtr root){var rows=new List<string>();EnumChildWindows(root,(h,p)=>{if(rows.Count<80)rows.Add(GetDlgCtrlID(h)+":"+Class(h));return true;},IntPtr.Zero);return String.Join(";",rows);}
}
'@
$ae=[Windows.Automation.AutomationElement]
$scope=[Windows.Automation.TreeScope]
$deadline=[DateTime]::UtcNow.AddSeconds(20)
$handle=[IntPtr]::Zero
while([DateTime]::UtcNow -lt $deadline){
 if($ownerProcess.HasExited){throw 'The isolated dialog owner exited'}
 $handle=[OwnedDialogState]::Find([uint32]$OwnerPID)
 if($handle -ne [IntPtr]::Zero){break};Start-Sleep -Milliseconds 80
}
if($handle -eq [IntPtr]::Zero){throw ('The isolated native file dialog did not open; windows='+[OwnedDialogState]::Describe([uint32]$OwnerPID))}
$nativeEdit=[OwnedDialogState]::Edit($handle)
$nativeButton=[OwnedDialogState]::Button($handle)
if($nativeEdit -ne [IntPtr]::Zero -and $nativeButton -ne [IntPtr]::Zero){
 # Standard common-dialog control IDs are stable across language and avoid
 # accessibility providers that expose only the embedded Explorer pane.
 [OwnedDialogState]::SetText($nativeEdit,$selected)
 $confirm={ [OwnedDialogState]::Click($nativeButton) }
}else{
 $dialog=$ae::FromHandle($handle)
 $buttons=$dialog.FindAll($scope::Descendants,[Windows.Automation.PropertyCondition]::new($ae::ControlTypeProperty,[Windows.Automation.ControlType]::Button))
 $button=@($buttons | Where-Object {$_.Current.IsEnabled -and ($_.Current.AutomationId -match '^(CommandButton_)?1$' -or $_.Current.Name -match '^(\u9009\u62e9\u6587\u4ef6\u5939|Select Folder|\u6253\u5f00|Open)')}) | Select-Object -First 1
 if(-not $button){throw ('The owned dialog has no confirmation control; native='+[OwnedDialogState]::Controls($handle))}
 $edits=$dialog.FindAll($scope::Descendants,[Windows.Automation.PropertyCondition]::new($ae::ControlTypeProperty,[Windows.Automation.ControlType]::Edit))
 $candidates=@()
 foreach($edit in $edits){$pattern=$null;if(-not $edit.Current.IsOffscreen -and $edit.Current.IsEnabled -and $edit.TryGetCurrentPattern([Windows.Automation.ValuePattern]::Pattern,[ref]$pattern) -and -not $pattern.Current.IsReadOnly){$candidates+=@{Pattern=$pattern;Top=$edit.Current.BoundingRectangle.Top}}}
 $field=$candidates | Sort-Object Top | Select-Object -Last 1
 if(-not $field){throw 'The owned file dialog has no editable selection field'}
 $field.Pattern.SetValue($selected)
 $invoke=$button.GetCurrentPattern([Windows.Automation.InvokePattern]::Pattern)
 $confirm={ $invoke.Invoke() }
}
if($ownerProcess.HasExited){throw 'The isolated dialog owner exited'}
& $confirm
$deadline=[DateTime]::UtcNow.AddSeconds(2)
while([OwnedDialogState]::IsWindow($handle) -and [DateTime]::UtcNow -lt $deadline){Start-Sleep -Milliseconds 50}
if($Kind -eq 'folder' -and [OwnedDialogState]::IsWindow($handle)){
 & $confirm
 $deadline=[DateTime]::UtcNow.AddSeconds(2)
 while([OwnedDialogState]::IsWindow($handle) -and [DateTime]::UtcNow -lt $deadline){Start-Sleep -Milliseconds 50}
}
if([OwnedDialogState]::IsWindow($handle)){throw 'The owned native dialog did not confirm the fixture selection'}
Write-Output 'Selected the fixture path through the owned native dialog'
