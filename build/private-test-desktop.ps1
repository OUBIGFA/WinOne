# Shared test desktop launcher: never switches the user's desktop or sends input.
if(-not ('WinOnePrivateTestDesktop' -as [type])){
 Add-Type -TypeDefinition @'
using System;using System.Text;using System.Diagnostics;using System.Runtime.InteropServices;
public sealed class WinOnePrivateTestDesktop:IDisposable {
 [StructLayout(LayoutKind.Sequential,CharSet=CharSet.Unicode)]struct Startup {public int size;public string reserved,desktop,title;public int x,y,width,height,charsX,charsY,fill,flags;public short show,reservedSize;public IntPtr reservedBytes,input,output,error;}
 [StructLayout(LayoutKind.Sequential)]struct Info {public IntPtr process,thread;public int pid,tid;}
 delegate bool EnumWindow(IntPtr window,IntPtr data);
 [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)]static extern IntPtr CreateDesktopW(string name,IntPtr device,IntPtr mode,uint flags,uint access,IntPtr security);
 [DllImport("user32.dll")]static extern bool CloseDesktop(IntPtr desktop);
 [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)]static extern bool CreateProcessW(string exe,StringBuilder command,IntPtr pa,IntPtr ta,bool inherit,uint flags,IntPtr environment,string cwd,ref Startup startup,out Info info);
 [DllImport("kernel32.dll")]static extern bool CloseHandle(IntPtr handle);
 [DllImport("kernel32.dll")]static extern uint GetCurrentThreadId();
 [DllImport("user32.dll")]static extern IntPtr GetThreadDesktop(uint thread);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern bool GetUserObjectInformationW(IntPtr handle,int index,StringBuilder value,int size,out int needed);
 [DllImport("user32.dll")]static extern bool EnumDesktopWindows(IntPtr desktop,EnumWindow callback,IntPtr data);
 [DllImport("user32.dll")]static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern int GetClassNameW(IntPtr window,StringBuilder value,int size);
 [DllImport("user32.dll")]static extern bool PostMessageW(IntPtr window,uint message,UIntPtr w,IntPtr l);
 readonly IntPtr desktop;public string Name {get;private set;}
 public WinOnePrivateTestDesktop(){Name="WinOne-Test-"+Guid.NewGuid().ToString("N");desktop=CreateDesktopW(Name,IntPtr.Zero,IntPtr.Zero,0,0xc7,IntPtr.Zero);if(desktop==IntPtr.Zero)throw new System.ComponentModel.Win32Exception();}
 public Process Start(string exe,string args,string cwd){var startup=new Startup{size=Marshal.SizeOf(typeof(Startup)),desktop=Name};Info info;if(!CreateProcessW(exe,new StringBuilder("\""+exe+"\" "+args),IntPtr.Zero,IntPtr.Zero,false,0x08000000,IntPtr.Zero,cwd,ref startup,out info))throw new System.ComponentModel.Win32Exception();try {var process=Process.GetProcessById(info.pid);var handle=process.Handle;return process;}finally{CloseHandle(info.thread);CloseHandle(info.process);}}
 public static string CurrentName(){var text=new StringBuilder(256);int needed;if(!GetUserObjectInformationW(GetThreadDesktop(GetCurrentThreadId()),2,text,512,out needed))throw new System.ComponentModel.Win32Exception();return text.ToString();}
 public void RequestHostExit(uint pid){IntPtr found=IntPtr.Zero;EnumDesktopWindows(desktop,delegate(IntPtr window,IntPtr ignored){uint owner;GetWindowThreadProcessId(window,out owner);var name=new StringBuilder(64);GetClassNameW(window,name,64);if(owner==pid&&name.ToString()=="SystrayClass")found=window;return true;},IntPtr.Zero);if(found==IntPtr.Zero||!PostMessageW(found,0x111,new UIntPtr(3),IntPtr.Zero))throw new InvalidOperationException("Owned Host tray exit was not available");}
 public void Dispose(){if(!CloseDesktop(desktop))throw new System.ComponentModel.Win32Exception();}
}
'@
}

