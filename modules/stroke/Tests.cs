using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Windows.Forms;
using Stroke;

namespace WinOne.StrokeRuntime
{
    internal static class StrokeTests
    {
        [DllImport("user32.dll")]private static extern uint GetGuiResources(IntPtr process,uint kind);
        [DllImport("user32.dll")]private static extern IntPtr GetThreadDpiAwarenessContext();
        [DllImport("user32.dll")]private static extern bool AreDpiAwarenessContextsEqual(IntPtr a,IntPtr b);
        [DllImport("gdi32.dll")]private static extern uint GetPixel(IntPtr canvas,int x,int y);
        private static void Check(bool condition,string message){if(!condition)throw new Exception(message);}
        private static Color TrailPixel(TrailWindow trail,int x,int y)
        {
            // Read only the owned window's painted surface. WM_PRINT and
            // Refresh would request a new paint and hide a missing repaint.
            IntPtr canvas=API.GetDC(trail.Handle);Check(canvas!=IntPtr.Zero,"Trail surface is unavailable");
            try{uint pixel=GetPixel(canvas,x,y);Check(pixel!=UInt32.MaxValue,"Trail pixel is unavailable");return Color.FromArgb((int)(pixel&255),(int)((pixel>>8)&255),(int)((pixel>>16)&255));}
            finally{API.ReleaseDC(trail.Handle,canvas);}
        }
        [STAThread] public static int Main(string[] args)
        {
            try {
                Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
                var hookHealth=new HookRecovery();
                Check(!hookHealth.NeedsRestart(120000,100,100),"Idle time was mistaken for a lost hook");
                Check(!hookHealth.NeedsRestart(120000,120000,119999),"Healthy input caused a hook restart");
                Check(hookHealth.NeedsRestart(120000,120000,100),"Fresh input did not repair a missing low-level hook");
                Check(!hookHealth.NeedsRestart(120500,120000,100),"One input repeatedly restarted the hook");
                Check(hookHealth.NeedsRestart(123000,123000,120000),"A later hook failure could not recover");
                var config=Configuration.Parse(File.ReadAllText(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"defaults.json")));
                // Behavior tests exercise the engine's gesture→button mapping for every action, independent of which actions the production default ships enabled. Enable all actions in this controlled fixture.
                foreach(var pack in config.actionPackages)foreach(var action in pack.actions)action.enabled=true;
                Check(Marshal.SizeOf(typeof(API.INPUT))==40,"x64 INPUT must be 40 bytes");
                Check(Marshal.OffsetOf(typeof(API.INPUT),"mi").ToInt32()==8,"x64 INPUT union must begin at byte 8");
                Check(AreDpiAwarenessContextsEqual(GetThreadDpiAwarenessContext(),new IntPtr(-4)),"Trail and hook coordinates require per-monitor-v2 DPI awareness");
                var policy=new ScopePolicy(config,@"C:\Windows");
                var folder=new WindowTarget{window=1,processId=1,processPath=@"C:\Windows\explorer.exe",windowClass="CabinetWClass"};
                Check(policy.Match(folder).Any(p=>p.id=="explorer"),"Explorer folder not enabled by default");
                foreach(string name in new[]{"Progman","WorkerW","Shell_TrayWnd","Unknown"}){folder.windowClass=name;Check(policy.Match(folder).Count==0,"Desktop or taskbar was captured");}
                folder.windowClass="CabinetWClass";folder.processPath=@"C:\Other\explorer.exe";Check(policy.Match(folder).Count==0,"An explorer.exe name bypassed identity");
                folder.processPath=@"C:\Browser\chrome.exe";Check(policy.Match(folder).Count==0,"Browser captured by default");
                folder.processPath=@"C:\Windows\explorer.exe";
                // Disabled wheel/button actions must never be eaten merely
                // because the scene contains an enabled spatial gesture.
                var onlyHorizontal=Configuration.Parse(File.ReadAllText(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"defaults.json")));
                foreach(var package in onlyHorizontal.actionPackages)foreach(var a in package.actions)a.enabled=a.gesture=="left"||a.gesture=="right";
                var replayed=new List<MouseHook.MouseActionArgs>();
                var horizontal=new GestureEngine(onlyHorizontal,p=>folder,(a,b)=>{},()=>{},(a,m,t)=>{},events=>replayed.AddRange(events),new ScopePolicy(onlyHorizontal,@"C:\Windows"));
                horizontal.Process(new MouseHook.MouseActionArgs(new Point(100,100),MouseButtons.Right,MouseHook.MouseButtonStates.Down));
                horizontal.Process(new MouseHook.MouseActionArgs(new Point(100,100),MouseButtons.None,MouseHook.MouseButtonStates.Wheel,120));
                Check(replayed.Count==2&&replayed[1].WheelDelta==120&&!horizontal.Tracking,"Disabled wheel action swallowed native wheel");
                horizontal.Cancel();
                int lookups=0,clicks=0;var actions=new List<string>();var marks=new List<int>();
                var engine=new GestureEngine(config,p=>{lookups++;return folder;},(a,b)=>{},()=>{},(a,m,t)=>{actions.Add(a);marks.Add(m);},events=>clicks++,policy);
                var down=new MouseHook.MouseActionArgs(new Point(100,100),MouseButtons.Right,MouseHook.MouseButtonStates.Down);
                var up=new MouseHook.MouseActionArgs(new Point(100,100),MouseButtons.Right,MouseHook.MouseButtonStates.Up);
                folder.windowClass="Progman";Check(!engine.Process(down)&&!engine.Process(up)&&!engine.Tracking,"Excluded input was swallowed");
                folder.windowClass="CabinetWClass";Check(engine.Process(down),"Enabled trigger not owned");Check(engine.Process(up)&&clicks==1,"Ordinary right click not replayed once");
                engine.Process(down);engine.Process(new MouseHook.MouseActionArgs(new Point(25,100),MouseButtons.None,MouseHook.MouseButtonStates.Move));engine.Process(up);
                Check(actions.SequenceEqual(new[]{"global.back"}),"Actual Stroke vectors did not match left/back");
                Check(clicks==1,"Recognized gesture also replayed right click");
                engine.Process(down);Check(engine.Process(new MouseHook.MouseActionArgs(new Point(100,100),MouseButtons.None,MouseHook.MouseButtonStates.Wheel,120)),"Enabled wheel gesture escaped");engine.Process(up);Check(actions.Last()=="media.volume-up"&&marks.Last()==0,"Enabled wheel action did not run");
                engine.Process(down);engine.Cancel();Check(!engine.Tracking&&!engine.Process(up),"Shutdown kept a gesture active");
                GestureInputTests.Run();
                var exact=new Gesture("left",new List<Point>{new Point(100,100),new Point(0,100)});
                Check(exact.Vectors.Length==128&&exact.Similarity(Configuration.ToGesture(config.gestures.First(g=>g.id=="left")))==128,"Stroke resampling changed");
                // A real owned fixture window is resolved by its handle. Mouse
                // events remain synthetic; this suite installs no global hook.
                using(var fixture=new Form()){var target=NativeWindowIdentity.FromWindow(fixture.Handle);Check(target!=null&&target.processId==System.Diagnostics.Process.GetCurrentProcess().Id,"Owned window identity failed");Check(policy.Match(target).Count==0,"Fixture application enabled by default");
                    System.Action draw=()=>{using(var trail=new global::Stroke.Draw(fixture.Handle,API.CreatePen(API.PS.SOLID,3,new API.COLORREF(20,40,60)))){trail.DrawPath(new Point(0,0),new Point(10,10));trail.DrawPath(new Point(10,10),new Point(20,0));}};draw();using(var process=System.Diagnostics.Process.GetCurrentProcess()){uint before=GetGuiResources(process.Handle,0);for(int i=0;i<50;i++)draw();Check(GetGuiResources(process.Handle,0)==before,"Repeated strokes leaked GDI pens or DCs");}
                    // Render through WM_PRINT/WM_PAINT on an owned window. This
                    // catches drawing through a stale DC and runs on a private
                    // desktop without reading or capturing the user's screen.
                    using(TrailWindow visible=new TrailWindow(new PenDefinition{enabled=true,color="#1f7fff",opacity=.8,thickness=5}),hidden=new TrailWindow(new PenDefinition{enabled=false,color="#1f7fff",opacity=.8,thickness=5})){
                        int renderedActions=0;
                        var renderedEngine=new GestureEngine(config,p=>folder,(a,b)=>{visible.Segment(a,b);hidden.Segment(a,b);},()=>{visible.ClearTrail();hidden.ClearTrail();},(name,m,t)=>{Check(name=="global.back","Rendered gesture did not match its action");renderedActions++;},events=>{throw new Exception("Rendered gesture replayed a click");},policy);
                        for(int stroke=0;stroke<3;stroke++){
                            var from=new Point(visible.Left+160,visible.Top+50+stroke*20);var to=new Point(visible.Left+40,from.Y);
                            renderedEngine.Process(new MouseHook.MouseActionArgs(from,MouseButtons.Right,MouseHook.MouseButtonStates.Down));renderedEngine.Process(new MouseHook.MouseActionArgs(to,MouseButtons.None,MouseHook.MouseButtonStates.Move));
                            Check(visible.Visible,"Enabled pen did not show its trail window");Check(!hidden.Visible,"Disabled pen showed its trail window");
                            Check(TrailPixel(visible,80,50+stroke*20).ToArgb()==Color.FromArgb(31,127,255).ToArgb(),"Gesture did not paint its native surface before returning");
                            to=new Point(visible.Left+20,from.Y);renderedEngine.Process(new MouseHook.MouseActionArgs(to,MouseButtons.None,MouseHook.MouseButtonStates.Move));
                            Check(TrailPixel(visible,30,50+stroke*20).ToArgb()==Color.FromArgb(31,127,255).ToArgb(),"Continuing gesture did not repaint its new segment");
                            if(stroke>0)Check(TrailPixel(visible,80,50).ToArgb()==visible.BackColor.ToArgb(),"A previous gesture remained on the native surface");
                            visible.Refresh();Application.DoEvents();
                            using(var pixels=new Bitmap(visible.Width,visible.Height)){
                                visible.DrawToBitmap(pixels,new Rectangle(Point.Empty,pixels.Size));Color actual=pixels.GetPixel(80,50+stroke*20);Check(actual.B>actual.R+60,"Trail missing from repainted surface: "+actual);
                                if(stroke>0)Check(pixels.GetPixel(80,50).ToArgb()==visible.BackColor.ToArgb(),"A previous gesture left a trail behind");
                            }
                            renderedEngine.Process(new MouseHook.MouseActionArgs(to,MouseButtons.Right,MouseHook.MouseButtonStates.Up));Check(!visible.Visible,"Released gesture left its overlay visible");Check(renderedActions==stroke+1,"Repeated gesture did not execute after clearing its trail");
                        }
                    }
                }
                var sent=new List<API.INPUT>();var ledger=new InputLedger(value=>{sent.Add(value);return 1;},key=>false);var downInput=new InputValue{type=1,key=(ushort)Keys.ControlKey};ledger.Send("owned",downInput);ledger.Release("owned");Check(sent.Count==2&&((uint)sent[1].ki.dwFlags&2)!=0,"Held simulated key was not returned");sent.Clear();var preserved=new InputLedger(value=>{sent.Add(value);return 1;},key=>true);preserved.Send("owned",downInput);preserved.Send("owned",new InputValue{type=1,key=(ushort)Keys.ControlKey,flags=2});preserved.ReleaseAll();Check(sent.Count==0,"Script released a physically held user key");
                sent.Clear();ledger.Send("first",downInput);ledger.Retire("first");Check(sent.Count==1,"Successful action prematurely released its key");ledger.Send("next",new InputValue{type=1,key=(ushort)Keys.ControlKey,flags=2});ledger.ReleaseAll();Check(sent.Count==2,"Cross-action KeyUp left stale ownership");
                Console.WriteLine("PASS: enabled paths; native drag and button replay; timeout/modifier bypass; origin identity; special gestures; x64 INPUT/DPI; native trail paint/repaint; GDI disposal; owned-input recovery");return 0;
            }catch(Exception error){Console.Error.WriteLine(error);return 1;}
        }
    }
}
