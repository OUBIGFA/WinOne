// Test-only WinForms drag source. This is built into an ignored task folder,
// never shipped. It exercises Wry's genuine OLE FileDrop handler.
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using System.Web.Script.Serialization;
using System.Windows.Forms;

public static class OleFileDropProbe {
    public sealed class Spec { public int targetPid; public string targetExe; public string taskRoot; public string[] paths; public double x; public double y; public double scale; public string resultPath; }
    [StructLayout(LayoutKind.Sequential)] public struct Point { public int x,y; public Point(int x,int y){this.x=x;this.y=y;} }
    [StructLayout(LayoutKind.Sequential)] struct Rect { public int left,top,right,bottom; }
    [StructLayout(LayoutKind.Sequential)] struct MouseInput { public int x,y; public uint data,flags,time; public IntPtr extra; }
    [StructLayout(LayoutKind.Explicit)] struct InputUnion { [FieldOffset(0)] public MouseInput mouse; }
    [StructLayout(LayoutKind.Sequential)] struct Input { public uint type; public InputUnion value; }
    [DllImport("user32.dll")] static extern IntPtr SetThreadDpiAwarenessContext(IntPtr value);
    [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
    [DllImport("user32.dll")] static extern IntPtr GetAncestor(IntPtr window,uint flags);
    [DllImport("user32.dll")] static extern IntPtr WindowFromPoint(Point point);
    [DllImport("user32.dll")] static extern bool GetCursorPos(out Point point);
    [DllImport("user32.dll")] static extern bool SetCursorPos(int x,int y);
    [DllImport("user32.dll")] static extern bool ClientToScreen(IntPtr window,ref Point point);
    [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr window);
    [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr window,out Rect rect);
    [DllImport("user32.dll")] static extern short GetAsyncKeyState(int key);
    [DllImport("user32.dll")] static extern int GetSystemMetrics(int index);
    [DllImport("user32.dll",SetLastError=true)] static extern uint SendInput(uint count,Input[] inputs,int size);
    [DllImport("user32.dll")] static extern bool PostMessage(IntPtr window,uint message,IntPtr w,IntPtr l);
    [DllImport("kernel32.dll")] static extern uint GetCurrentThreadId();
    [DllImport("user32.dll")] static extern IntPtr GetThreadDesktop(uint thread);
    [DllImport("user32.dll")] static extern IntPtr OpenInputDesktop(uint flags,bool inherit,uint access);
    [DllImport("user32.dll")] static extern bool CloseDesktop(IntPtr desktop);
    [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern bool GetUserObjectInformation(IntPtr handle,int index,StringBuilder value,int size,out int needed);
    static readonly JavaScriptSerializer Json=new JavaScriptSerializer();
    static Spec spec; static IntPtr target; static long targetStarted; static int sourcePid; static bool pressed,cancelled; static string failure=""; static readonly List<string> trace=new List<string>();
    static string DesktopName(IntPtr desktop){int needed;var name=new StringBuilder(256);if(!GetUserObjectInformation(desktop,2,name,512,out needed))throw new Win32Exception();return name.ToString();}
    static uint PID(IntPtr window){uint pid;GetWindowThreadProcessId(window,out pid);return pid;}
    static bool TargetAlive(){try{using(var process=Process.GetProcessById(spec.targetPid)){return process.StartTime.ToUniversalTime().Ticks==targetStarted&&String.Equals(Path.GetFullPath(process.MainModule.FileName),Path.GetFullPath(spec.targetExe),StringComparison.OrdinalIgnoreCase);}}catch{return false;}}
    static bool ForegroundOwned(){uint pid=PID(GetForegroundWindow());return TargetAlive()&&(pid==sourcePid||pid==spec.targetPid);}
    static bool PointOwned(Point point){var root=GetAncestor(WindowFromPoint(point),2);var pid=PID(root);return pid==sourcePid||root==target;}
    static void Mouse(uint flags,int x,int y){var input=new Input{type=0,value=new InputUnion{mouse=new MouseInput{x=x,y=y,flags=flags}}};if(SendInput(1,new[]{input},Marshal.SizeOf(typeof(Input)))!=1)throw new Win32Exception(Marshal.GetLastWin32Error());}
    static void Move(Point point){if(!ForegroundOwned()||!PointOwned(point))throw new InvalidOperationException("Input guard stopped: foreground or destination is not an owned test window.");int x0=GetSystemMetrics(76),y0=GetSystemMetrics(77),w=GetSystemMetrics(78),h=GetSystemMetrics(79);Mouse(0x8000|0x4000|0x0001,(int)((long)(point.x-x0)*65535/Math.Max(1,w-1)),(int)((long)(point.y-y0)*65535/Math.Max(1,h-1)));}
    static void Save(string state,string effect){File.WriteAllText(spec.resultPath,Json.Serialize(new{state=state,effect=effect,error=failure,sourcePid=sourcePid,targetPid=spec.targetPid,trace=trace,mousePressed=pressed}),new UTF8Encoding(false));}

    [STAThread] public static int Main(string[] args){
        Point original=new Point();bool haveCursor=false;Form form=null;System.Windows.Forms.Timer timer=null;
        try{
            if(args.Length!=1)throw new ArgumentException("Pass one test specification path.");
            spec=Json.Deserialize<Spec>(File.ReadAllText(args[0],Encoding.UTF8));
            string root=Path.GetFullPath(spec.taskRoot).TrimEnd(Path.DirectorySeparatorChar)+Path.DirectorySeparatorChar;
            if(!Path.GetFullPath(Process.GetCurrentProcess().MainModule.FileName).StartsWith(root,StringComparison.OrdinalIgnoreCase))throw new InvalidOperationException("The source probe must be a clone inside this test directory.");
            foreach(string path in spec.paths){if(!Path.GetFullPath(path).StartsWith(root,StringComparison.OrdinalIgnoreCase)||(!File.Exists(path)&&!Directory.Exists(path)))throw new InvalidOperationException("Only existing fixture paths inside this test directory may be dragged.");}
            if(!Path.GetFullPath(spec.targetExe).StartsWith(root,StringComparison.OrdinalIgnoreCase)||!Path.GetFullPath(spec.resultPath).StartsWith(root,StringComparison.OrdinalIgnoreCase))throw new InvalidOperationException("Target and output must belong to this test directory.");
            IntPtr inputDesktop=OpenInputDesktop(0,false,1);if(inputDesktop==IntPtr.Zero)throw new Win32Exception();
            try{if(DesktopName(inputDesktop)!=DesktopName(GetThreadDesktop(GetCurrentThreadId())))throw new InvalidOperationException("Real OLE input requires the explicitly selected current desktop.");}finally{CloseDesktop(inputDesktop);}
            SetThreadDpiAwarenessContext(new IntPtr(-4));
            using(var process=Process.GetProcessById(spec.targetPid)){target=process.MainWindowHandle;targetStarted=process.StartTime.ToUniversalTime().Ticks;}
            sourcePid=Process.GetCurrentProcess().Id;if(target==IntPtr.Zero||!TargetAlive())throw new InvalidOperationException("Owned WinOne target is unavailable.");
            if((GetAsyncKeyState(1)&0x8000)!=0)throw new InvalidOperationException("A mouse button is already held; no input was sent.");
            haveCursor=GetCursorPos(out original);if(!haveCursor)throw new Win32Exception();
            var origin=new Point(0,0);if(!ClientToScreen(target,ref origin))throw new Win32Exception();
            var destination=new Point(origin.x+(int)Math.Round(spec.x*spec.scale),origin.y+(int)Math.Round(spec.y*spec.scale));
            Rect window;GetWindowRect(target,out window);
            form=new Form{Text="WinOne isolated OLE source",StartPosition=FormStartPosition.Manual,Location=new System.Drawing.Point(window.left+16,window.top+50),ClientSize=new Size(248,82),TopMost=true,AutoScaleMode=AutoScaleMode.None};
            var sourceLabel=new Label{Dock=DockStyle.Fill,Text="Isolated FileDrop fixture\r\n"+spec.paths.Length+" file or folder entries",TextAlign=ContentAlignment.MiddleCenter};
            form.Controls.Add(sourceLabel);
            form.QueryContinueDrag+=(sender,eventArgs)=>{if(cancelled||!ForegroundOwned())eventArgs.Action=DragAction.Cancel;};
            // Start on the real mouse-down delivered to this control. Calling
            // DoDragDrop immediately after SendInput can see the old key state.
            sourceLabel.MouseDown+=(sender,eventArgs)=>{
                if(eventArgs.Button!=MouseButtons.Left||!pressed)return;
                if(timer!=null){timer.Stop();timer.Dispose();}
                var start=form.PointToScreen(new System.Drawing.Point(124,41));var from=new Point(start.X,start.Y);int step=0;var elapsed=Stopwatch.StartNew();
                timer=new System.Windows.Forms.Timer{Interval=90};
                timer.Tick+=(unused,tick)=>{
                    try{
                        if(!ForegroundOwned()||elapsed.ElapsedMilliseconds>3000)throw new InvalidOperationException("Input guard cancelled after focus loss or timeout.");
                        step++;var point=new Point(from.x+(destination.x-from.x)*Math.Min(step,6)/6,from.y+(destination.y-from.y)*Math.Min(step,6)/6);
                        Move(point);trace.Add("move "+step);
                        if(step>=7){if(!PointOwned(destination))throw new InvalidOperationException("Drop point is not the owned WinOne window.");Mouse(0x0004,0,0);pressed=false;timer.Stop();trace.Add("release on target");}
                    }catch(Exception error){failure=error.Message;cancelled=true;timer.Stop();PostMessage(form.Handle,0x001f,IntPtr.Zero,IntPtr.Zero);PostMessage(form.Handle,0x0100,new IntPtr(27),new IntPtr(1));}
                };
                try{
                    if(!ForegroundOwned())throw new InvalidOperationException("Test source lost focus before dragging.");
                    Save("dragging","");timer.Start();
                    var data=new DataObject();data.SetData(DataFormats.FileDrop,true,spec.paths);
                    var effect=form.DoDragDrop(data,DragDropEffects.Copy|DragDropEffects.Link);
                    Save(cancelled?"cancelled":"completed",effect.ToString());
                }catch(Exception error){failure=error.ToString();cancelled=true;Save("failed","");}
                finally{timer.Stop();if(pressed){Mouse(0x0004,0,0);pressed=false;}form.Close();}
            };
            form.Shown+=(sender,eventArgs)=>form.BeginInvoke(new Action(()=>{
                try{
                    SetForegroundWindow(form.Handle);if(!ForegroundOwned())throw new InvalidOperationException("Test source could not receive focus.");
                    var start=form.PointToScreen(new System.Drawing.Point(124,41));Move(new Point(start.X,start.Y));System.Threading.Thread.Sleep(150);
                    timer=new System.Windows.Forms.Timer{Interval=3000};
                    timer.Tick+=(unused,tick)=>{cancelled=true;failure="No source mouse-down arrived.";timer.Stop();if(pressed){Mouse(0x0004,0,0);pressed=false;}Save("failed","");form.Close();};
                    timer.Start();Mouse(0x0002,0,0);pressed=true;trace.Add("press in source");
                }catch(Exception error){cancelled=true;failure=error.ToString();Save("failed","");form.Close();}
            }));
            Application.Run(form);return cancelled?1:0;
        }catch(Exception error){failure=error.ToString();if(spec!=null)Save("failed","");return 1;}
        finally{if(timer!=null)timer.Dispose();if(form!=null)form.Dispose();if(pressed){Mouse(0x0004,0,0);pressed=false;}if(haveCursor)SetCursorPos(original.x,original.y);}
    }
}
