# Shared test desktop launcher. Default use never switches the user's desktop
# or sends input. Foreground checks require explicit constructor opt-in and a
# bounded watchdog; the original input desktop is restored on timeout/dispose.
if(-not ('WinOnePrivateTestDesktop' -as [type])){
 Add-Type -TypeDefinition @'
using System;using System.Text;using System.Diagnostics;using System.Threading;using System.Runtime.InteropServices;
public sealed class WinOnePrivateTestDesktop:IDisposable {
 [StructLayout(LayoutKind.Sequential,CharSet=CharSet.Unicode)]struct Startup {public int size;public string reserved,desktop,title;public int x,y,width,height,charsX,charsY,fill,flags;public short show,reservedSize;public IntPtr reservedBytes,input,output,error;}
 [StructLayout(LayoutKind.Sequential)]struct Info {public IntPtr process,thread;public int pid,tid;}
 delegate bool EnumWindow(IntPtr window,IntPtr data);
 [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)]static extern IntPtr CreateDesktopW(string name,IntPtr device,IntPtr mode,uint flags,uint access,IntPtr security);
 [DllImport("user32.dll",SetLastError=true)]static extern bool CloseDesktop(IntPtr desktop);
 [DllImport("user32.dll",SetLastError=true)]static extern IntPtr OpenInputDesktop(uint flags,bool inherit,uint access);
 [DllImport("user32.dll",SetLastError=true)]static extern bool SwitchDesktop(IntPtr desktop);
 [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)]static extern bool CreateProcessW(string exe,StringBuilder command,IntPtr pa,IntPtr ta,bool inherit,uint flags,IntPtr environment,string cwd,ref Startup startup,out Info info);
 [DllImport("kernel32.dll")]static extern bool CloseHandle(IntPtr handle);
 [DllImport("kernel32.dll")]static extern uint GetCurrentThreadId();
 [DllImport("user32.dll")]static extern IntPtr GetThreadDesktop(uint thread);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern bool GetUserObjectInformationW(IntPtr handle,int index,StringBuilder value,int size,out int needed);
 [DllImport("user32.dll")]static extern bool EnumDesktopWindows(IntPtr desktop,EnumWindow callback,IntPtr data);
 [DllImport("user32.dll")]static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern int GetClassNameW(IntPtr window,StringBuilder value,int size);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)]static extern int GetWindowTextW(IntPtr window,StringBuilder value,int size);
 [DllImport("user32.dll")]static extern bool IsWindowVisible(IntPtr window);
 [DllImport("user32.dll")]static extern bool PostMessageW(IntPtr window,uint message,UIntPtr w,IntPtr l);
 readonly IntPtr desktop;readonly bool foregroundAllowed;readonly object foregroundLock=new object();IntPtr originalInput;Timer foregroundTimer;public string Name {get;private set;}
 public WinOnePrivateTestDesktop():this(false){}
 public WinOnePrivateTestDesktop(bool allowForeground){foregroundAllowed=allowForeground;Name="WinOne-Test-"+Guid.NewGuid().ToString("N");desktop=CreateDesktopW(Name,IntPtr.Zero,IntPtr.Zero,0,0xc7u|(allowForeground?0x100u:0u),IntPtr.Zero);if(desktop==IntPtr.Zero)throw new System.ComponentModel.Win32Exception();}
 // Call only for an explicitly selected foreground test. The timeout is real
 // wall clock, independent of the tested UI thread or process exiting cleanly.
 public void BeginForegroundTest(int seconds){if(!foregroundAllowed)throw new InvalidOperationException("Foreground test was not explicitly enabled");if(seconds<1||seconds>30)throw new ArgumentOutOfRangeException("seconds");lock(foregroundLock){if(originalInput!=IntPtr.Zero)throw new InvalidOperationException("Foreground test already active");originalInput=OpenInputDesktop(0,false,0x101);if(originalInput==IntPtr.Zero)throw new System.ComponentModel.Win32Exception();foregroundTimer=new Timer(delegate(object ignored){RestoreInputDesktop();},null,seconds*1000,Timeout.Infinite);if(!SwitchDesktop(desktop)){int error=Marshal.GetLastWin32Error();RestoreInputDesktop();throw new System.ComponentModel.Win32Exception(error);}}}
 public bool RestoreInputDesktop(){lock(foregroundLock){if(originalInput==IntPtr.Zero)return true;if(!SwitchDesktop(originalInput))return false;foregroundTimer.Dispose();foregroundTimer=null;CloseDesktop(originalInput);originalInput=IntPtr.Zero;return true;}}
 public static string InputName(){IntPtr input=OpenInputDesktop(0,false,1);if(input==IntPtr.Zero)throw new System.ComponentModel.Win32Exception();try{var text=new StringBuilder(256);int needed;if(!GetUserObjectInformationW(input,2,text,512,out needed))throw new System.ComponentModel.Win32Exception();return text.ToString();}finally{CloseDesktop(input);}}
 public Process Start(string exe,string args,string cwd){var startup=new Startup{size=Marshal.SizeOf(typeof(Startup)),desktop=Name};Info info;if(!CreateProcessW(exe,new StringBuilder("\""+exe+"\" "+args),IntPtr.Zero,IntPtr.Zero,false,0x08000000,IntPtr.Zero,cwd,ref startup,out info))throw new System.ComponentModel.Win32Exception();try {var process=Process.GetProcessById(info.pid);var handle=process.Handle;return process;}finally{CloseHandle(info.thread);CloseHandle(info.process);}}
 public static string CurrentName(){var text=new StringBuilder(256);int needed;if(!GetUserObjectInformationW(GetThreadDesktop(GetCurrentThreadId()),2,text,512,out needed))throw new System.ComponentModel.Win32Exception();return text.ToString();}
 // Enumerate only this owned desktop. Process.MainWindowHandle enumerates
 // the caller's desktop and cannot identify a private-desktop test window.
 public IntPtr WindowHandle(uint pid){IntPtr found=IntPtr.Zero;int count=0;EnumDesktopWindows(desktop,delegate(IntPtr window,IntPtr ignored){uint owner;GetWindowThreadProcessId(window,out owner);var title=new StringBuilder(128);GetWindowTextW(window,title,128);if(owner==pid&&IsWindowVisible(window)&&title.ToString()=="WinOne"){found=window;count++;}return true;},IntPtr.Zero);if(count>1)throw new InvalidOperationException("Owned settings window is ambiguous");return found;}
 public static IntPtr CurrentWindowHandle(uint pid){IntPtr found=IntPtr.Zero;int count=0;EnumDesktopWindows(GetThreadDesktop(GetCurrentThreadId()),delegate(IntPtr window,IntPtr ignored){uint owner;GetWindowThreadProcessId(window,out owner);var title=new StringBuilder(128);GetWindowTextW(window,title,128);if(owner==pid&&IsWindowVisible(window)&&title.ToString()=="WinOne"){found=window;count++;}return true;},IntPtr.Zero);if(count>1)throw new InvalidOperationException("Owned settings window is ambiguous");return found;}
 public static void CloseCurrentWindow(uint pid){IntPtr window=CurrentWindowHandle(pid);if(window==IntPtr.Zero||!PostMessageW(window,0x10,UIntPtr.Zero,IntPtr.Zero))throw new InvalidOperationException("Owned settings window did not accept WM_CLOSE");}
 public void CloseWindow(uint pid){IntPtr window=WindowHandle(pid);if(window==IntPtr.Zero||!PostMessageW(window,0x10,UIntPtr.Zero,IntPtr.Zero))throw new InvalidOperationException("Owned settings window did not accept WM_CLOSE");}
 public void RequestHostExit(uint pid){IntPtr found=IntPtr.Zero;EnumDesktopWindows(desktop,delegate(IntPtr window,IntPtr ignored){uint owner;GetWindowThreadProcessId(window,out owner);var name=new StringBuilder(64);GetClassNameW(window,name,64);if(owner==pid&&name.ToString()=="SystrayClass")found=window;return true;},IntPtr.Zero);if(found==IntPtr.Zero||!PostMessageW(found,0x111,new UIntPtr(3),IntPtr.Zero))throw new InvalidOperationException("Owned Host tray exit was not available");}
 public void Dispose(){if(!RestoreInputDesktop())throw new System.ComponentModel.Win32Exception();if(!CloseDesktop(desktop))throw new System.ComponentModel.Win32Exception();}
}
'@
}

