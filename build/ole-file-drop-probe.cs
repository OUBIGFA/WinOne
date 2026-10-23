// Test-only WinForms OLE source. Never shipped; all paths and input targets
// must belong to the runner's isolated product instance.
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
 public sealed class Spec { public int targetPid; public string targetExe,taskRoot,resultPath; public string[] paths; public double x,y,scale; public bool expectedTargetElevated; }
 [StructLayout(LayoutKind.Sequential)] public struct Point {public int x,y;public Point(int x,int y){this.x=x;this.y=y;}}
 [StructLayout(LayoutKind.Sequential)] struct Rect {public int left,top,right,bottom;}
 [StructLayout(LayoutKind.Sequential)] struct MouseInput {public int x,y;public uint data,flags,time;public IntPtr extra;}
 [StructLayout(LayoutKind.Explicit)] struct InputUnion {[FieldOffset(0)] public MouseInput mouse;}
 [StructLayout(LayoutKind.Sequential)] struct Input {public uint type;public InputUnion value;}
 [DllImport("user32.dll")] static extern IntPtr SetThreadDpiAwarenessContext(IntPtr value);
 [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr window,out uint pid);
 [DllImport("user32.dll")] static extern IntPtr GetAncestor(IntPtr window,uint flags);
 [DllImport("user32.dll")] static extern IntPtr WindowFromPoint(Point point);
 [DllImport("user32.dll")] static extern bool GetCursorPos(out Point point);
 [DllImport("user32.dll")] static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")] static extern bool ClientToScreen(IntPtr window,ref Point point);
 [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr window);
 [DllImport("user32.dll")] static extern bool ShowWindow(IntPtr window,int command);
 [DllImport("user32.dll")] static extern bool BringWindowToTop(IntPtr window);
 [DllImport("user32.dll")] static extern bool AttachThreadInput(uint source,uint target,bool attach);
 [DllImport("dwmapi.dll")] static extern int DwmFlush();
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
 [DllImport("kernel32.dll",SetLastError=true)] static extern IntPtr OpenProcess(uint access,bool inherit,int pid);
 [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool QueryFullProcessImageName(IntPtr process,uint flags,StringBuilder value,ref int length);
 [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);
 [DllImport("advapi32.dll",SetLastError=true)] static extern bool OpenProcessToken(IntPtr process,uint access,out IntPtr token);
 [DllImport("advapi32.dll",SetLastError=true)] static extern bool GetTokenInformation(IntPtr token,int kind,out int value,int size,out int needed);
 static bool sourceElevated,targetElevated;
 // Verify the actual tokens; a runas flag alone is not cross-integrity evidence.
 static bool IsElevated(int pid){var process=OpenProcess(0x1000,false,pid);if(process==IntPtr.Zero)throw new Win32Exception();IntPtr token=IntPtr.Zero;try{if(!OpenProcessToken(process,8,out token))throw new Win32Exception();int value,needed;if(!GetTokenInformation(token,20,out value,4,out needed))throw new Win32Exception();return value!=0;}finally{if(token!=IntPtr.Zero)CloseHandle(token);CloseHandle(process);}}
 static readonly JavaScriptSerializer Json=new JavaScriptSerializer();
 static Spec spec;static IntPtr target;static long targetStarted;static int sourcePid;static bool pressed,cancelled;static string failure="";static readonly List<string> trace=new List<string>();
 static string DesktopName(IntPtr desktop){int needed;var name=new StringBuilder(256);if(!GetUserObjectInformation(desktop,2,name,512,out needed))throw new Win32Exception();return name.ToString();}
 static uint PID(IntPtr window){uint pid;GetWindowThreadProcessId(window,out pid);return pid;}
 // QUERY_LIMITED_INFORMATION works for the normal source -> elevated target
 // check, unlike .NET Framework MainModule (which also requests VM_READ).
 static string ProcessPath(int pid){var handle=OpenProcess(0x1000,false,pid);if(handle==IntPtr.Zero)throw new Win32Exception();try{var text=new StringBuilder(32768);int length=text.Capacity;if(!QueryFullProcessImageName(handle,0,text,ref length))throw new Win32Exception();return text.ToString();}finally{CloseHandle(handle);}}
 static bool TargetAlive(){try{using(var process=Process.GetProcessById(spec.targetPid)){return process.StartTime.ToUniversalTime().Ticks==targetStarted&&String.Equals(Path.GetFullPath(ProcessPath(spec.targetPid)),Path.GetFullPath(spec.targetExe),StringComparison.OrdinalIgnoreCase);}}catch{return false;}}
 static bool ForegroundOwned(){uint pid=PID(GetForegroundWindow());return TargetAlive()&&(pid==sourcePid||pid==spec.targetPid);}
 static bool PointOwned(Point point){var root=GetAncestor(WindowFromPoint(point),2);return PID(root)==sourcePid||root==target;}
 static void Mouse(uint flags,int x,int y){var input=new Input{type=0,value=new InputUnion{mouse=new MouseInput{x=x,y=y,flags=flags}}};if(SendInput(1,new[]{input},Marshal.SizeOf(typeof(Input)))!=1)throw new Win32Exception(Marshal.GetLastWin32Error());}
 static void Move(Point point){if(!ForegroundOwned()||!PointOwned(point))throw new InvalidOperationException("Input guard stopped: foreground or destination is not an owned test window.");int x0=GetSystemMetrics(76),y0=GetSystemMetrics(77),w=GetSystemMetrics(78),h=GetSystemMetrics(79);Mouse(0x8000|0x4000|0x0001,(int)((long)(point.x-x0)*65535/Math.Max(1,w-1)),(int)((long)(point.y-y0)*65535/Math.Max(1,h-1)));}
 static void Save(string state,string effect){File.WriteAllText(spec.resultPath,Json.Serialize(new{state=state,effect=effect,error=failure,sourcePid=sourcePid,targetPid=spec.targetPid,sourceElevated=sourceElevated,targetElevated=targetElevated,trace=trace,mousePressed=pressed}),new UTF8Encoding(false));}
 [STAThread] public static int Main(string[] args){
  Point original=new Point();bool haveCursor=false;Form form=null;System.Windows.Forms.Timer timer=null;
  try{
   if(args.Length!=1)throw new ArgumentException("Pass one specification path.");
   spec=Json.Deserialize<Spec>(File.ReadAllText(args[0],Encoding.UTF8));
   string root=Path.GetFullPath(spec.taskRoot).TrimEnd(Path.DirectorySeparatorChar)+Path.DirectorySeparatorChar;
   foreach(string path in new[]{ProcessPath(Process.GetCurrentProcess().Id),spec.targetExe,spec.resultPath})if(!Path.GetFullPath(path).StartsWith(root,StringComparison.OrdinalIgnoreCase))throw new InvalidOperationException("Probe, target and output must belong to the isolated task directory.");
   foreach(string path in spec.paths)if(!Path.GetFullPath(path).StartsWith(root,StringComparison.OrdinalIgnoreCase)||(!File.Exists(path)&&!Directory.Exists(path)))throw new InvalidOperationException("Only existing isolated fixture paths may be dragged.");
   IntPtr desktop=OpenInputDesktop(0,false,1);if(desktop==IntPtr.Zero)throw new Win32Exception();
   try{if(DesktopName(desktop)!=DesktopName(GetThreadDesktop(GetCurrentThreadId())))throw new InvalidOperationException("OLE input requires the explicitly selected input desktop.");}finally{CloseDesktop(desktop);}
   SetThreadDpiAwarenessContext(new IntPtr(-4));
   using(var process=Process.GetProcessById(spec.targetPid)){target=process.MainWindowHandle;targetStarted=process.StartTime.ToUniversalTime().Ticks;}
   sourcePid=Process.GetCurrentProcess().Id;if(target==IntPtr.Zero||!TargetAlive())throw new InvalidOperationException("Owned target is unavailable.");
   sourceElevated=IsElevated(sourcePid);targetElevated=IsElevated(spec.targetPid);
   if(sourceElevated||targetElevated!=spec.expectedTargetElevated)throw new InvalidOperationException("Unexpected source/target elevation; refusing an invalid cross-integrity test.");
   if((GetAsyncKeyState(1)&0x8000)!=0)throw new InvalidOperationException("A mouse button is held; no input was sent.");
   haveCursor=GetCursorPos(out original);if(!haveCursor)throw new Win32Exception();
   var origin=new Point();if(!ClientToScreen(target,ref origin))throw new Win32Exception();
   var destination=new Point(origin.x+(int)Math.Round(spec.x*spec.scale),origin.y+(int)Math.Round(spec.y*spec.scale));
   Rect window;GetWindowRect(target,out window);
   form=new Form{Text="WinOne isolated OLE source",StartPosition=FormStartPosition.Manual,Location=new System.Drawing.Point(window.left+16,window.top+50),ClientSize=new Size(248,82),TopMost=true,AutoScaleMode=AutoScaleMode.None};
   var label=new Label{Dock=DockStyle.Fill,Text="Isolated FileDrop fixture",TextAlign=ContentAlignment.MiddleCenter};form.Controls.Add(label);
   form.QueryContinueDrag+=(sender,e)=>{if(cancelled||!ForegroundOwned())e.Action=DragAction.Cancel;};
   label.MouseDown+=(sender,e)=>{
    if(e.Button!=MouseButtons.Left||!pressed)return;
    if(timer!=null){timer.Stop();timer.Dispose();}
    var start=label.PointToScreen(new System.Drawing.Point(label.Width/2,label.Height/2));var from=new Point(start.X,start.Y);
    var data=new DataObject();data.SetData(DataFormats.FileDrop,true,spec.paths);Save("dragging","");
    // OLE runs a nested COM loop which can starve WinForms timers. Drive only
    // guarded mouse input on a bounded thread, like a user's independent
    // mouse, rather than extending timeouts on the blocked source UI thread.
    var done=new System.Threading.ManualResetEvent(false);
    var inputThread=new System.Threading.Thread(()=>{
     var elapsed=Stopwatch.StartNew();
     try{
      for(int step=1;step<=7&&!done.WaitOne(90);step++){
       if(!ForegroundOwned()||elapsed.ElapsedMilliseconds>3000)throw new InvalidOperationException("Input guard cancelled after focus loss or timeout.");
       Move(new Point(from.x+(destination.x-from.x)*Math.Min(step,6)/6,from.y+(destination.y-from.y)*Math.Min(step,6)/6));trace.Add("move "+step);
      }
     }catch(Exception error){failure=error.Message;cancelled=true;}
     finally{if(pressed){Mouse(0x0004,0,0);pressed=false;}}
    });inputThread.IsBackground=true;
    try{inputThread.Start();var effect=form.DoDragDrop(data,DragDropEffects.Copy|DragDropEffects.Link);done.Set();inputThread.Join();Save(cancelled?"cancelled":"completed",effect.ToString());}
    catch(Exception error){failure=error.ToString();cancelled=true;done.Set();inputThread.Join();Save("failed","");}
    finally{done.Dispose();if(pressed){Mouse(0x0004,0,0);pressed=false;}form.Close();}
   };
   form.Shown+=(sender,e)=>form.BeginInvoke(new Action(()=>{
    try{
     if(!ForegroundOwned())throw new InvalidOperationException("Foreground is no longer owned.");
     // Explicitly transfer focus from the owned target. SetForegroundWindow
     // alone may silently fail for a source launched by the CDP driver.
     uint ignored;uint foregroundThread=GetWindowThreadProcessId(GetForegroundWindow(),out ignored),current=GetCurrentThreadId();
     bool attached=foregroundThread!=current&&AttachThreadInput(current,foregroundThread,true);
     try{ShowWindow(form.Handle,5);BringWindowToTop(form.Handle);SetForegroundWindow(form.Handle);}finally{if(attached)AttachThreadInput(current,foregroundThread,false);}
     DwmFlush();
     if(GetForegroundWindow()!=form.Handle)throw new InvalidOperationException("Owned source could not receive focus; no input sent.");
     var start=label.PointToScreen(new System.Drawing.Point(label.Width/2,label.Height/2));Move(new Point(start.X,start.Y));
     timer=new System.Windows.Forms.Timer{Interval=150};
     timer.Tick+=(unused,tick)=>{
      timer.Stop();Point actual;GetCursorPos(out actual);
      if(PID(WindowFromPoint(actual))!=sourcePid){cancelled=true;failure="Source is not under the cursor; no mouse-down sent.";Save("failed","");form.Close();return;}
      timer.Dispose();timer=new System.Windows.Forms.Timer{Interval=3000};
      timer.Tick+=(unused2,tick2)=>{cancelled=true;failure="No source mouse-down arrived.";timer.Stop();if(pressed){Mouse(0x0004,0,0);pressed=false;}Save("failed","");form.Close();};
      timer.Start();pressed=true;Mouse(0x0002,0,0);trace.Add("press in source");
     };timer.Start();
    }catch(Exception error){cancelled=true;failure=error.ToString();Save("failed","");form.Close();}
   }));
   Application.Run(form);return cancelled?1:0;
  }catch(Exception error){failure=error.ToString();if(spec!=null)Save("failed","");return 1;}
  finally{if(timer!=null)timer.Dispose();if(form!=null)form.Dispose();if(pressed){Mouse(0x0004,0,0);pressed=false;}if(haveCursor)SetCursorPos(original.x,original.y);}
 }
}
