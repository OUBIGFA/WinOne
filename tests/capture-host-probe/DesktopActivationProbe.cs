using System.Diagnostics;
using System.IO.Pipes;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;

internal static class DesktopActivationProbe
{
    private delegate bool EnumProc(IntPtr window,IntPtr state);
    [DllImport("user32.dll")]private static extern bool EnumWindows(EnumProc callback,IntPtr state);
    [DllImport("user32.dll")]private static extern uint GetWindowThreadProcessId(IntPtr window,out int pid);
    [DllImport("user32.dll",CharSet=CharSet.Unicode)]private static extern int GetClassNameW(IntPtr window,StringBuilder name,int length);
    [DllImport("user32.dll")]private static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")]private static extern bool SetForegroundWindow(IntPtr window);
    [DllImport("user32.dll")]private static extern bool AttachThreadInput(uint first,uint second,bool attach);
    [DllImport("kernel32.dll")]private static extern uint GetCurrentThreadId();
    [DllImport("user32.dll")]private static extern bool IsIconic(IntPtr window);
    [DllImport("user32.dll")]private static extern bool IsWindowVisible(IntPtr window);
    [DllImport("user32.dll")]private static extern bool PostMessageW(IntPtr window,uint message,IntPtr w,IntPtr l);
    [DllImport("kernel32.dll",SetLastError=true)]private static extern bool GetNamedPipeServerProcessId(Microsoft.Win32.SafeHandles.SafePipeHandle pipe,out uint pid);
    [DllImport("kernel32.dll",SetLastError=true)]private static extern IntPtr OpenProcess(uint access,bool inherit,int pid);
    [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)]private static extern bool QueryFullProcessImageNameW(IntPtr process,uint flags,StringBuilder path,ref int size);
    [DllImport("kernel32.dll")]private static extern bool CloseHandle(IntPtr handle);
    private static IntPtr Find(int pid){var result=IntPtr.Zero;EnumWindows((window,_)=>{GetWindowThreadProcessId(window,out var owner);if(owner!=pid)return true;var name=new StringBuilder(128);GetClassNameW(window,name,name.Capacity);if(name.ToString()=="WinOne.Desktop")result=window;return true;},IntPtr.Zero);return result;}
    private static IntPtr FindProduct(string executable){var result=IntPtr.Zero;EnumWindows((window,_)=>{var name=new StringBuilder(128);GetClassNameW(window,name,name.Capacity);if(name.ToString()!="WinOne.Desktop")return true;GetWindowThreadProcessId(window,out var pid);var process=OpenProcess(0x1000,false,pid);if(process==IntPtr.Zero)return true;try{var path=new StringBuilder(32768);var size=path.Capacity;if(QueryFullProcessImageNameW(process,0,path,ref size)&&string.Equals(path.ToString(),executable,StringComparison.OrdinalIgnoreCase))result=window;}finally{CloseHandle(process);}return true;},IntPtr.Zero);return result;}
    private static async Task Wait(Func<bool> condition,string error){var deadline=DateTime.UtcNow.AddSeconds(15);while(!condition()){if(DateTime.UtcNow>=deadline)throw new Exception(error);await Task.Delay(25);}}
    private static void OwnForeground(Form launcher){
        var current=GetCurrentThreadId();var other=GetWindowThreadProcessId(GetForegroundWindow(),out _);
        var attached=other!=0&&other!=current&&AttachThreadInput(current,other,true);
        try{launcher.Activate();SetForegroundWindow(launcher.Handle);}finally{if(attached)AttachThreadInput(current,other,false);}
        if(GetForegroundWindow()!=launcher.Handle)throw new Exception("Activation fixture did not own foreground input");
    }
    internal static int Run(string output){
        var bundle=Environment.GetEnvironmentVariable("WINONE_ACTIVATION_BUNDLE")??throw new InvalidOperationException("Set WINONE_ACTIVATION_BUNDLE");
        var defaultProfile=Environment.GetEnvironmentVariable("WINONE_ACTIVATION_DEFAULT_PROFILE")=="1";
        output=Path.GetFullPath(output);Directory.CreateDirectory(output);var profile=Path.Combine(output,"profile");Directory.CreateDirectory(profile);
        File.WriteAllText(Path.Combine(profile,"settings.json"),"{\"schemaVersion\":1,\"theme\":\"light\",\"captureShortcut\":\"\",\"pinShortcut\":\"\",\"ocrShortcut\":\"\",\"translateShortcut\":\"\"}");
        File.WriteAllText(Path.Combine(profile,"module-state.json"),"{\"schemaVersion\":1,\"revision\":0,\"modules\":{\"capture\":{\"enabled\":false},\"always-on-top\":{\"enabled\":false},\"wintray\":{\"enabled\":false}}}");
        var pipe="WinOne-foreground-"+Guid.NewGuid().ToString("N");Process? desktop=null;Process? host=null;Exception? failed=null;
        Process Start(){var info=new ProcessStartInfo(Path.Combine(bundle,"WinOne.exe")){UseShellExecute=false,CreateNoWindow=true,RedirectStandardError=true};info.Environment["WINONE_DESKTOP_STARTUP_TRACE"]="1";foreach(var arg in defaultProfile?new[]{"--open-settings"}:new[]{"--standard-session","--open-settings","--pipe",@"\\.\pipe\"+pipe,"--data-dir",profile})info.ArgumentList.Add(arg);var child=Process.Start(info)??throw new Exception("Desktop did not start");child.ErrorDataReceived+=(_,line)=>{if(line.Data is not null)File.AppendAllText(Path.Combine(output,"desktop.log"),line.Data+Environment.NewLine);};child.BeginErrorReadLine();return child;}
        using var launcher=new Form{Text="WinOne 窗口激活测试",Size=new Size(400,100),StartPosition=FormStartPosition.CenterScreen};
        launcher.Shown+=async(_,_)=>{
            try{
                OwnForeground(launcher);desktop=Start();IntPtr window=IntPtr.Zero;
                if(defaultProfile){
                    await Wait(()=>{window=FindProduct(Path.Combine(bundle,"WinOne.exe"));return window!=IntPtr.Zero&&IsWindowVisible(window)&&GetForegroundWindow()==window;},"Installed WinOne did not activate");
                    var sid=System.Security.Principal.WindowsIdentity.GetCurrent().User!.Value;
                    using var client=new NamedPipeClientStream(".",$"WinOne-{sid}-{Process.GetCurrentProcess().SessionId}",PipeDirection.InOut,PipeOptions.Asynchronous);
                    using var deadline=new CancellationTokenSource(TimeSpan.FromSeconds(5));await client.ConnectAsync(deadline.Token);
                    await WinOne.Capture.PipeProtocol.WriteAsync(client,JsonSerializer.SerializeToUtf8Bytes(new{version=1,id="activation-check",method="snapshot"}),deadline.Token);
                    using var response=JsonDocument.Parse(await WinOne.Capture.PipeProtocol.ReadAsync(client,deadline.Token));var snapshot=response.RootElement.GetProperty("result");var elevated=snapshot.GetProperty("administrator").GetBoolean();
                    if(snapshot.GetProperty("settings").TryGetProperty("startup",out var startup)&&startup.TryGetProperty("preferAdministrator",out var prefer)&&prefer.GetBoolean()&&!elevated)throw new Exception("Remembered administrator mode was not preserved");
                    GetWindowThreadProcessId(window,out var actualPid);File.WriteAllText(Path.Combine(output,"result.json"),JsonSerializer.Serialize(new{foreground=true,administrator=elevated,defaultProfile=true,desktopPid=actualPid}));
                    return;
                }
                await Wait(()=>{if(desktop.HasExited)throw new Exception("Desktop exited: "+desktop.ExitCode);window=Find(desktop.Id);return window!=IntPtr.Zero&&IsWindowVisible(window);},"Initial open did not show WinOne");
                var foreground=GetForegroundWindow();GetWindowThreadProcessId(foreground,out var foregroundPid);
                File.WriteAllText(Path.Combine(output,"initial.json"),JsonSerializer.Serialize(new{window=window.ToInt64(),foreground=foreground.ToInt64(),foregroundPid,expectedPid=desktop.Id,launcherPid=Environment.ProcessId}));
                await Wait(()=>GetForegroundWindow()==window,"Visible WinOne did not activate");
                using(var client=new NamedPipeClientStream(".",pipe,PipeDirection.InOut,PipeOptions.Asynchronous)){using var deadline=new CancellationTokenSource(TimeSpan.FromSeconds(5));await client.ConnectAsync(deadline.Token);if(!GetNamedPipeServerProcessId(client.SafePipeHandle,out var pid))throw new Exception("Host identity unavailable");host=Process.GetProcessById((int)pid);}
                PostMessageW(window,0x112,new IntPtr(0xf020),IntPtr.Zero);await Wait(()=>IsIconic(window),"Window did not minimize");
                OwnForeground(launcher);using var wake=Start();await wake.WaitForExitAsync();if(wake.ExitCode!=0)throw new Exception("Window wake failed");
                await Wait(()=>!IsIconic(window)&&GetForegroundWindow()==window,"Reopening did not restore and activate the same window");
                if(Find(desktop.Id)!=window)throw new Exception("Reopening replaced the window");
                PostMessageW(window,0x10,IntPtr.Zero,IntPtr.Zero);await Wait(()=>desktop.HasExited,"Desktop did not close");
                TrayExitProbe.RequestExit(host.Id);await Wait(()=>host.HasExited,"Host did not exit");
                File.WriteAllText(Path.Combine(output,"result.json"),JsonSerializer.Serialize(new{initialForeground=true,reopenedForeground=true,reusedWindow=true,restoredFromMinimized=true,desktopExited=true,hostExited=true}));
            }catch(Exception error){failed=error;}finally{foreach(var process in new[]{desktop,host})if(process is not null){if(!defaultProfile&&!process.HasExited)process.Kill(true);process.Dispose();}launcher.Close();}
        };
        Application.Run(launcher);if(failed is not null)throw failed;return 0;
    }
}
