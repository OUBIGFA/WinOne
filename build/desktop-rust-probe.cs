// Background lifecycle probe only: never requests desktop-switch permission or
// sends global input. A private desktop is not a visible-experience benchmark.
using System;
using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;

public sealed class RustDesktopProbe : IDisposable
{
    [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)]
    struct StartupInfo {
        public int cb; public string reserved, desktop, title;
        public int x,y,width,height,xChars,yChars,fill,flags;
        public short show,reservedSize; public IntPtr reservedBytes,input,output,error;
    }
    [StructLayout(LayoutKind.Sequential)]
    struct ProcessInfo { public IntPtr process,thread; public int pid,tid; }
    delegate bool EnumWindow(IntPtr window, IntPtr data);
    [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern IntPtr CreateDesktop(string name,IntPtr device,IntPtr mode,int flags,uint access,IntPtr security);
    [DllImport("user32.dll",SetLastError=true)] static extern bool CloseDesktop(IntPtr desktop);
    [DllImport("user32.dll",SetLastError=true)] static extern bool EnumDesktopWindows(IntPtr desktop,EnumWindow callback,IntPtr data);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
    [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr window,StringBuilder name,int count);
    [DllImport("user32.dll")] static extern IntPtr GetDlgItem(IntPtr window,int id);
    [DllImport("user32.dll")] static extern bool IsWindowEnabled(IntPtr window);
    [DllImport("kernel32.dll",SetLastError=true)] static extern bool GetNamedPipeServerProcessId(IntPtr pipe,out uint processId);
    [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern IntPtr FindWindowEx(IntPtr parent,IntPtr after,string className,string title);
    [DllImport("user32.dll",SetLastError=true)] static extern IntPtr SendMessageTimeout(IntPtr window,uint message,UIntPtr wparam,IntPtr lparam,uint flags,uint timeout,out UIntPtr result);
    [DllImport("user32.dll",SetLastError=true)] static extern IntPtr SendMessageTimeout(IntPtr window,uint message,UIntPtr wparam,StringBuilder lparam,uint flags,uint timeout,out UIntPtr result);
    [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool CreateProcess(string executable,StringBuilder command,IntPtr processAttributes,IntPtr threadAttributes,bool inherit,uint flags,IntPtr environment,string directory,ref StartupInfo startup,out ProcessInfo process);
    [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);
    readonly IntPtr desktop;
    public Process Child { get; private set; }
    public IntPtr Window { get; private set; }
    public double ReadyMilliseconds { get; private set; }
    string executablePath, pipeName, profileDirectory, desktopName;
    public RustDesktopProbe(string executable,string pipe,string dataDirectory,bool requireCatalog=true)
    {
        if (!pipe.StartsWith(@"\\.\pipe\WinOne-rust-test-",StringComparison.Ordinal) || pipe.IndexOf('"')>=0) throw new ArgumentException("Use a dedicated test pipe");
        if (!System.IO.Path.IsPathRooted(dataDirectory) || dataDirectory.IndexOf('"')>=0) throw new ArgumentException("Use a dedicated absolute profile directory");
        var name="WinOne-Rust-Probe-"+Guid.NewGuid().ToString("N");
        executablePath=executable; pipeName=pipe; profileDirectory=dataDirectory; desktopName=name;
        // READOBJECTS | CREATEWINDOW | ENUMERATE | WRITEOBJECTS; no SWITCHDESKTOP.
        desktop=CreateDesktop(name,IntPtr.Zero,IntPtr.Zero,0,0xc3,IntPtr.Zero);
        if(desktop==IntPtr.Zero) throw new Win32Exception();
        var timer=Stopwatch.StartNew();
        try {
            var startup=new StartupInfo { cb=Marshal.SizeOf(typeof(StartupInfo)),desktop=name };
            ProcessInfo info;
            if(!CreateProcess(executable,new StringBuilder("\""+executable+"\" --pipe \""+pipe+"\" --data-dir \""+dataDirectory.TrimEnd('\\')+"\""),IntPtr.Zero,IntPtr.Zero,false,0,IntPtr.Zero,System.IO.Path.GetDirectoryName(executable),ref startup,out info)) throw new Win32Exception();
            try { Child=Process.GetProcessById(info.pid); var retained=Child.Handle; }
            finally { CloseHandle(info.thread);CloseHandle(info.process); }
            while(timer.ElapsedMilliseconds<10000) {
                if(Child.HasExited) throw new InvalidOperationException("Rust desktop exited before becoming ready");
                EnumWindow callback=delegate(IntPtr window,IntPtr unused) {
                    uint pid; GetWindowThreadProcessId(window,out pid);
                    var className=new StringBuilder(128); GetClassName(window,className,128);
                    if(pid==(uint)Child.Id && className.ToString()=="WinOne.Desktop") Window=window;
                    return true;
                };
                if(!EnumDesktopWindows(desktop,callback,IntPtr.Zero)) {
                    var error=Marshal.GetLastWin32Error();
                    if(error!=0) throw new Win32Exception(error,"Cannot enumerate the private desktop");
                }
                if(Window!=IntPtr.Zero && GetDlgItem(Window,requireCatalog?104:101)!=IntPtr.Zero) { ReadyMilliseconds=timer.Elapsed.TotalMilliseconds; return; }
                System.Threading.Thread.Sleep(10);
            }
            throw new TimeoutException("Rust desktop did not load the Host catalog");
        } catch { Dispose(); throw; }
    }
    public double Ping() {
        var timer=Stopwatch.StartNew(); UIntPtr result;
        if(SendMessageTimeout(Window,0,UIntPtr.Zero,IntPtr.Zero,2,1000,out result)==IntPtr.Zero) throw new Win32Exception();
        return timer.Elapsed.TotalMilliseconds;
    }
    public int ConnectedHostProcessId() {
        using(var pipe=new System.IO.Pipes.NamedPipeClientStream(".",pipeName.Substring(9),System.IO.Pipes.PipeDirection.InOut)) {
            pipe.Connect(1000); uint pid;
            if(!GetNamedPipeServerProcessId(pipe.SafePipeHandle.DangerousGetHandle(),out pid)) throw new Win32Exception();
            return checked((int)pid);
        }
    }
    public double Reopen() {
        var timer=Stopwatch.StartNew();
        var startup=new StartupInfo { cb=Marshal.SizeOf(typeof(StartupInfo)),desktop=desktopName };
        ProcessInfo info;
        var args="\""+executablePath+"\" --pipe \""+pipeName+"\" --data-dir \""+profileDirectory.TrimEnd('\\')+"\" --open-settings";
        if(!CreateProcess(executablePath,new StringBuilder(args),IntPtr.Zero,IntPtr.Zero,false,0,IntPtr.Zero,System.IO.Path.GetDirectoryName(executablePath),ref startup,out info)) throw new Win32Exception();
        using(var duplicate=Process.GetProcessById(info.pid)) {
            // Retain a handle before releasing CreateProcess's handle; otherwise
            // an already-exited duplicate loses its readable exit status.
            var retained=duplicate.Handle;
            CloseHandle(info.thread);CloseHandle(info.process);
            if(!duplicate.WaitForExit(5000)) { duplicate.Kill(); duplicate.WaitForExit(); throw new TimeoutException("Duplicate settings launch did not exit"); }
            if(duplicate.ExitCode!=0 || Child.HasExited) throw new InvalidOperationException("Duplicate settings launch did not preserve the original process");
        }
        return timer.Elapsed.TotalMilliseconds;
    }
    public double OpenGlobalLogs(string expectedText) {
        var timer=Stopwatch.StartNew();
        Click(GetDlgItem(Window,104)!=IntPtr.Zero?104:101);
        Click(203);
        WaitForLog(expectedText);
        return timer.Elapsed.TotalMilliseconds;
    }
    public double ReadSavedCaptureHistory(string expectedPath) {
        var timer=Stopwatch.StartNew();Click(101);Click(375);
        while(timer.ElapsedMilliseconds<5000) {
            var path=GetDlgItem(Window,376);var text=new StringBuilder(4096);UIntPtr count;
            if(path!=IntPtr.Zero) SendMessageTimeout(path,0x000d,new UIntPtr(4096),text,2,1000,out count);
            if(text.ToString()==expectedPath) return timer.Elapsed.TotalMilliseconds;
            System.Threading.Thread.Sleep(10);
        }
        var last=new StringBuilder(4096);var status=new StringBuilder(4096);UIntPtr ignored;
        SendMessageTimeout(GetDlgItem(Window,376),0x000d,new UIntPtr(4096),last,2,1000,out ignored);
        SendMessageTimeout(GetDlgItem(Window,400),0x000d,new UIntPtr(4096),status,2,1000,out ignored);
        throw new TimeoutException("Local saved-capture history did not become available; path="+last+"; status="+status+"; refreshEnabled="+IsWindowEnabled(GetDlgItem(Window,375)));
    }
    // Only commands to this runner-owned private window are sent. No global
    // keyboard/mouse input, focus changes, or Explorer actions are involved.
    public double OpenCaptureLogs(string expectedText) {
        var timer=Stopwatch.StartNew();
        Click(101);
        Click(203);
        WaitForLog(expectedText);
        return timer.Elapsed.TotalMilliseconds;
    }
    void WaitForLog(string expectedText) {
        var timer=Stopwatch.StartNew();
        // On DBCS systems Rich Edit counts ANSI code-page bytes in its length
        // answers: WM_GETTEXTLENGTH reports 56007 for a 48007-character fixture,
        // and WM_GETTEXT also treats the buffer size as a byte budget and copies
        // only a prefix if it is smaller. Size the buffer for the worst-case
        // byte count and compare the returned text itself.
        var buffer=new StringBuilder(expectedText.Length*2+1);
        while(timer.ElapsedMilliseconds<10000) {
            if(Child.HasExited) throw new InvalidOperationException("Rust desktop exited while loading logs");
            var viewer=FindWindowEx(Window,IntPtr.Zero,"RICHEDIT50W",null);
            if(viewer!=IntPtr.Zero) {
                UIntPtr copied;
                if(SendMessageTimeout(viewer,0x000d,new UIntPtr((uint)(expectedText.Length*2+1)),buffer,2,1000,out copied)==IntPtr.Zero) throw new Win32Exception();
                if(buffer.ToString()==expectedText) return;
            }
            System.Threading.Thread.Sleep(10);
        }
        var status=new StringBuilder(4096); UIntPtr statusCount;
        var statusWindow=GetDlgItem(Window,400);
        SendMessageTimeout(statusWindow,0x000d,new UIntPtr(4096),status,2,1000,out statusCount);
        var actual=new StringBuilder(expectedText.Length*2+1); UIntPtr logCount;
        var logWindow=FindWindowEx(Window,IntPtr.Zero,"RICHEDIT50W",null);
        if(logWindow!=IntPtr.Zero) SendMessageTimeout(logWindow,0x000d,new UIntPtr((uint)(expectedText.Length*2+1)),actual,2,1000,out logCount);
        throw new TimeoutException("The log viewer did not display the complete fixture; status="+status+"; log="+actual+"; settings="+(GetDlgItem(Window,104)!=IntPtr.Zero));
    }
    void Click(int id) {
        // Async page replies can rebuild child HWNDs between observations.
        // Wait for the requested control, then dispatch the action only once.
        var timer=Stopwatch.StartNew();var control=GetDlgItem(Window,id);
        while(control==IntPtr.Zero && timer.ElapsedMilliseconds<5000 && !Child.HasExited) {
            System.Threading.Thread.Sleep(10);control=GetDlgItem(Window,id);
        }
        if(control==IntPtr.Zero) throw new InvalidOperationException("Missing private test control: "+id);
        UIntPtr result;
        if(SendMessageTimeout(Window,0x0111,new UIntPtr((uint)id),control,2,1000,out result)==IntPtr.Zero) throw new Win32Exception();
    }
    public double CloseNormally() {
        var timer=Stopwatch.StartNew(); UIntPtr result;
        if(SendMessageTimeout(Window,0x10,UIntPtr.Zero,IntPtr.Zero,2,1000,out result)==IntPtr.Zero) throw new Win32Exception();
        if(!Child.WaitForExit(5000)) throw new TimeoutException("Rust desktop did not exit after WM_CLOSE");
        if(Child.ExitCode!=0) throw new InvalidOperationException("Rust desktop exited with an error");
        return timer.Elapsed.TotalMilliseconds;
    }
    public void Dispose() {
        if(Child!=null) {
            if(!Child.HasExited) { Child.Kill();Child.WaitForExit(5000); }
            Child.Dispose(); Child=null;
        }
        if(desktop!=IntPtr.Zero) CloseDesktop(desktop);
    }
}
