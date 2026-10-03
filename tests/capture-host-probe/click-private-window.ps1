param(
 [Parameter(Mandatory=$true)][string]$StateFile,
 [Parameter(Mandatory=$true)][int]$X,
 [Parameter(Mandatory=$true)][int]$Y,
 [ValidateSet('click','double-message','duplicate-confirm','right-message','text-key','escape-key','set-text','drag','move')][string]$Action='click',
 [string]$Text=''
)
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
. (Join-Path $PSScriptRoot 'read-probe-state.ps1')
$state=Read-SharedProbeState $StateFile
$expected=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot 'bin/Release/net10.0-windows/WinOne.NativeCapture.Probe.exe'))
$process=Get-Process -Id $state.nativeProcess
if(-not [string]::Equals($process.Path,$expected,[StringComparison]::OrdinalIgnoreCase)){throw 'Target is not this prototype executable'}
Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class PrivateTextClick {
 [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern IntPtr OpenDesktopW(string name,uint flags,bool inherit,uint access);
 [DllImport("user32.dll")] static extern bool SetThreadDesktop(IntPtr desktop);
 [DllImport("user32.dll")] static extern bool CloseDesktop(IntPtr desktop);
 [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
 [DllImport("user32.dll")] static extern IntPtr GetThreadDesktop(uint thread);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern bool GetUserObjectInformationW(IntPtr handle,int index,StringBuilder value,uint size,out uint needed);
 [DllImport("user32.dll")] static extern bool PostMessageW(IntPtr window,uint message,IntPtr w,IntPtr l);
 [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern IntPtr SendMessageTimeoutW(IntPtr window,uint message,IntPtr w,string text,uint flags,uint timeout,out IntPtr result);
 [DllImport("user32.dll",EntryPoint="SendMessageTimeoutW",SetLastError=true)] static extern IntPtr SendValue(IntPtr window,uint message,IntPtr w,IntPtr value,uint flags,uint timeout,out IntPtr result);
 public static void Send(long window,uint expected,int x,int y,string desktopName,string action,string text){
  if(!System.Text.RegularExpressions.Regex.IsMatch(desktopName,"^WinOne-Resource-[a-f0-9]{32}$"))throw new InvalidOperationException("Not a test desktop");
  var desktop=OpenDesktopW(desktopName,0,false,0xc7);if(desktop==IntPtr.Zero)throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
  Exception failure=null;
  var worker=new System.Threading.Thread(()=>{try{if(!SetThreadDesktop(desktop))throw new InvalidOperationException("Could not enter test desktop");SendOnDesktop(window,expected,x,y,action,text);}catch(Exception e){failure=e;}});
  worker.Start();worker.Join();CloseDesktop(desktop);if(failure!=null)throw failure;
 }
 static void SendOnDesktop(long window,uint expected,int x,int y,string action,string text){
  if(x<0||y<0||x>32767||y>32767)throw new ArgumentException("Invalid test point");
  uint pid;var hwnd=new IntPtr(window);uint thread=GetWindowThreadProcessId(hwnd,out pid);
  if(thread==0||pid!=expected)throw new InvalidOperationException("Window identity changed");
  var name=new StringBuilder(256);uint needed;
  if(!GetUserObjectInformationW(GetThreadDesktop(thread),2,name,512,out needed)||!name.ToString().StartsWith("WinOne-Resource-",StringComparison.Ordinal))throw new InvalidOperationException("Refusing to send messages to a user desktop");
  var point=new IntPtr((y<<16)|x);
  if(action=="move"){if(!PostMessageW(hwnd,0x200,IntPtr.Zero,point))throw new InvalidOperationException("Pointer message failed");return;}
  if(action=="drag"){
   var end=new IntPtr(((y+20)<<16)|(x+50));
   if(!PostMessageW(hwnd,0x201,new IntPtr(1),point)||!PostMessageW(hwnd,0x200,new IntPtr(1),end)||!PostMessageW(hwnd,0x202,IntPtr.Zero,end))throw new InvalidOperationException("Native drag message failed");
   return;
  }
  if(action=="set-text"){
   IntPtr result;
   // Multiline EDIT does not emit EN_CHANGE for WM_SETTEXT. Replace the
   // selection through the editing API so normal change/undo handling runs.
   if(SendValue(hwnd,0xb1,IntPtr.Zero,new IntPtr(-1),2,3000,out result)==IntPtr.Zero)throw new InvalidOperationException("Native text selection failed");
   if(SendMessageTimeoutW(hwnd,0xc2,new IntPtr(1),text,2,3000,out result)==IntPtr.Zero)throw new InvalidOperationException("Native text update failed");
   return;
  }
  if(action=="text-key"||action=="escape-key"){
   var key=new IntPtr(action=="text-key"?0x54:0x1b);
   if(!PostMessageW(hwnd,0x100,key,IntPtr.Zero)||!PostMessageW(hwnd,0x101,key,IntPtr.Zero))throw new InvalidOperationException("Test key message failed");
   return;
  }
  if(action=="double-message"||action=="duplicate-confirm"){
   if(!PostMessageW(hwnd,0x203,new IntPtr(1),point))throw new InvalidOperationException("Confirmation message failed");
   if(action=="duplicate-confirm")PostMessageW(hwnd,0x203,new IntPtr(1),point);
   return;
  }
  if(action=="right-message"){if(!PostMessageW(hwnd,0x205,IntPtr.Zero,point))throw new InvalidOperationException("Cancellation message failed");return;}
  if(action!="click")throw new ArgumentException("Invalid test action");
  if(!PostMessageW(hwnd,0x201,new IntPtr(1),point)||!PostMessageW(hwnd,0x202,IntPtr.Zero,point))throw new InvalidOperationException("Test message failed");
 }
}
'@
[PrivateTextClick]::Send([long]$state.nativeWindow,[uint32]$state.nativeProcess,$X,$Y,[string]$state.nativeDesktop,$Action,$Text)
