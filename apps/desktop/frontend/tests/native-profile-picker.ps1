param([Parameter(Mandatory)][int]$OwnedProcessId,[Parameter(Mandatory)][string]$Executable,[Parameter(Mandatory)][string]$Path,[ValidateSet('open','save','cancel')][string]$Mode,[ValidateSet('.winone','.csv','.cshare')][string]$Extension='.winone',[switch]$Folder)
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$process=Get-Process -Id $OwnedProcessId
$expected=[IO.Path]::GetFullPath($Executable);$target=[IO.Path]::GetFullPath($Path)
if(-not [string]::Equals($process.Path,$expected,[StringComparison]::OrdinalIgnoreCase)){throw 'Picker owner is not the test executable'}
if(-not $target.StartsWith(([IO.Path]::GetDirectoryName($expected)+[IO.Path]::DirectorySeparatorChar),[StringComparison]::OrdinalIgnoreCase) -or (-not $Folder -and [IO.Path]::GetExtension($target) -ne $Extension)){throw 'Selected path must stay inside this owned test directory and use the requested type'}
if($Folder -and ($Mode-eq'save' -or -not(Test-Path -LiteralPath $target -PathType Container))){throw 'Folder picker requires an existing owned directory'}
if(($Mode -eq 'save') -and (Test-Path -LiteralPath $target)){throw 'Save test requires a new archive'}
if(($Mode -eq 'open') -and -not(Test-Path -LiteralPath $target)){throw 'Open test requires its existing archive'}
# Operate only the classic common dialog owned by the isolated UI. No global
# keyboard input, desktop switching or unrelated dialog dismissal is used.
Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class ProfilePickerProbe {
 delegate bool EnumWindow(IntPtr window,IntPtr data);
 [DllImport("user32.dll")] static extern bool EnumWindows(EnumWindow callback,IntPtr data);
 [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr window,EnumWindow callback,IntPtr data);
 [DllImport("user32.dll")] static extern int GetDlgCtrlID(IntPtr window);
 [DllImport("user32.dll")] static extern IntPtr GetParent(IntPtr window);
 [DllImport("user32.dll")] static extern IntPtr GetDlgItem(IntPtr window,int id);
 [DllImport("user32.dll")] public static extern bool IsWindow(IntPtr window);
 [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr window,StringBuilder name,int count);
 [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern IntPtr SendMessageTimeout(IntPtr window,uint message,UIntPtr w,string text,uint flags,uint timeout,out UIntPtr result);
 [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern IntPtr SendMessageTimeout(IntPtr window,uint message,UIntPtr w,StringBuilder text,uint flags,uint timeout,out UIntPtr result);
 [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern IntPtr SendMessageTimeout(IntPtr window,uint message,UIntPtr w,IntPtr value,uint flags,uint timeout,out UIntPtr result);
 [DllImport("user32.dll",SetLastError=true)] static extern bool PostMessage(IntPtr window,uint message,UIntPtr w,IntPtr l);
 public static IntPtr Find(uint pid){IntPtr found=IntPtr.Zero;EnumWindows(delegate(IntPtr window,IntPtr data){uint owner;GetWindowThreadProcessId(window,out owner);var name=new StringBuilder(128);GetClassName(window,name,128);if(owner==pid&&name.ToString()=="#32770")found=window;return true;},IntPtr.Zero);return found;}
 public static void Cancel(IntPtr dialog){if(!PostMessage(dialog,0x111,new UIntPtr(2),IntPtr.Zero))throw new System.ComponentModel.Win32Exception();}
 public static bool Ready(IntPtr dialog,bool suggestedName){var name=new StringBuilder(32768);UIntPtr value;IntPtr input=IntPtr.Zero;EnumChildWindows(dialog,delegate(IntPtr child,IntPtr ignored){var type=new StringBuilder(64);GetClassName(child,type,64);if(type.ToString()=="Edit"&&(GetDlgCtrlID(child)==1001||GetDlgCtrlID(child)==1152||GetDlgCtrlID(GetParent(child))==1148))input=child;return true;},IntPtr.Zero);return input!=IntPtr.Zero&&SendMessageTimeout(input,0xD,new UIntPtr(32768),name,2,3000,out value)!=IntPtr.Zero&&(!suggestedName||name.Length>0);}
 public static void Choose(IntPtr dialog,string path,bool folder){
  UIntPtr result;
  // WM_SETTEXT/WM_GETTEXT marshal across processes. CDM_* carries raw pointers
  // and cannot be used from this independent test helper.
  IntPtr edit=IntPtr.Zero;int matches=0;var controls=new StringBuilder();
  EnumChildWindows(dialog,delegate(IntPtr child,IntPtr ignored){var name=new StringBuilder(128);GetClassName(child,name,128);if(name.ToString()=="Edit"){
   int id=GetDlgCtrlID(child),parent=GetDlgCtrlID(GetParent(child)),grandparent=GetDlgCtrlID(GetParent(GetParent(child)));
   controls.AppendFormat(" Edit#{0}/parent#{1}/grandparent#{2}",id,parent,grandparent);
   if(id==1001||id==1152||parent==1148||grandparent==1148){edit=child;matches++;}
  }return true;},IntPtr.Zero);
  if(matches!=1){PostMessage(dialog,0x111,new UIntPtr(2),IntPtr.Zero);throw new InvalidOperationException("Cannot identify one filename control:"+controls);}
  // Replace the selected text through the Edit input path. Shell dialogs
  // also cache their filename; changing display text alone can leave that
  // cached selection unchanged even when WM_GETTEXT shows the new value.
  if(SendMessageTimeout(edit,0xB1,UIntPtr.Zero,new IntPtr(-1),2,3000,out result)==IntPtr.Zero)throw new System.ComponentModel.Win32Exception();
  if(SendMessageTimeout(edit,0xC2,new UIntPtr(1),path,2,3000,out result)==IntPtr.Zero)throw new System.ComponentModel.Win32Exception();
  var selected=new StringBuilder(32768);
  if(SendMessageTimeout(edit,0xD,new UIntPtr(32768),selected,2,3000,out result)==IntPtr.Zero)throw new System.ComponentModel.Win32Exception();
  if(!String.Equals(selected.ToString(),path,StringComparison.OrdinalIgnoreCase)){PostMessage(dialog,0x111,new UIntPtr(2),IntPtr.Zero);throw new InvalidOperationException("Picker path did not match the owned archive: "+selected);}
  Console.WriteLine("Verified filename control #{0}: {1}",GetDlgCtrlID(edit),selected);
  var button=folder?GetDlgItem(dialog,1):dialog;
  if(button==IntPtr.Zero)throw new InvalidOperationException("The owned folder dialog has no selection button");
  if(!PostMessage(button,folder?0xF5u:0x111u,folder?UIntPtr.Zero:new UIntPtr(1),IntPtr.Zero))throw new System.ComponentModel.Win32Exception();
 }
}
'@
$deadline=[DateTime]::UtcNow.AddSeconds(15)
do{$window=[ProfilePickerProbe]::Find([uint32]$OwnedProcessId);if($window -ne [IntPtr]::Zero -and ($Mode-eq'cancel' -or [ProfilePickerProbe]::Ready($window,$Mode-eq'save'))){if($Mode-eq'cancel'){[ProfilePickerProbe]::Cancel($window)}else{[ProfilePickerProbe]::Choose($window,$target,$Folder.IsPresent)};exit 0};Start-Sleep -Milliseconds 50}while([DateTime]::UtcNow -lt $deadline)
throw 'The owned profile file dialog did not appear'
