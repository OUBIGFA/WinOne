using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Diagnostics;
using System.Threading;
using System.Windows.Forms;
using Stroke;

namespace WinOne.StrokeRuntime
{
    internal static class GestureInputTests
    {
        private static void Check(bool value,string message){if(!value)throw new Exception(message);}
        private static MouseHook.MouseActionArgs Event(int x,int y,MouseHook.MouseButtonStates state,MouseButtons button=MouseButtons.None,short wheel=0){return new MouseHook.MouseActionArgs(new Point(x,y),button,state,wheel);}
        internal static void Run()
        {
            var config=Configuration.Parse(File.ReadAllText(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"defaults.json")));
            foreach(var pack in config.actionPackages)foreach(var action in pack.actions)action.enabled=action.gesture=="left"||action.gesture=="right";
            var target=new WindowTarget{window=101,processId=7,processPath=@"C:\Windows\explorer.exe",windowClass="CabinetWClass"};
            var replay=new List<MouseHook.MouseActionArgs>();var actions=new List<WindowTarget>();long clock=0;bool modified=false;int lookups=0;
            var engine=new GestureEngine(config,p=>{lookups++;return target;},(a,b)=>{},()=>{},(a,m,t)=>actions.Add(t),events=>replay.AddRange(events),new ScopePolicy(config,@"C:\Windows"),()=>clock,()=>modified);
            var down=Event(100,100,MouseHook.MouseButtonStates.Down,MouseButtons.Right);
            var up=Event(100,100,MouseHook.MouseButtonStates.Up,MouseButtons.Right);
            engine.Process(down);engine.Process(Event(100,120,MouseHook.MouseButtonStates.Move));
            Check(!engine.Tracking&&replay.Count==2&&replay[0]==down&&replay[1].Location.Y==120,"Vertical drag did not restore its source down and path");
            Check(!engine.Process(up)&&actions.Count==0,"Native drag resumed gesture recognition");replay.Clear();
            engine.Process(down);engine.Process(Event(80,100,MouseHook.MouseButtonStates.Move));engine.Process(Event(80,120,MouseHook.MouseButtonStates.Move));
            Check(!engine.Tracking&&replay.Count==3,"Rejected continuation did not restore the full drag");replay.Clear();
            engine.Process(down);engine.Process(Event(80,100,MouseHook.MouseButtonStates.Move));target.window=202;engine.Process(up);
            Check(actions.Count==1&&actions[0].window==101&&actions[0].point.x==100,"Action was redirected from its origin window");target.window=101;
            engine.Process(down);clock=249;engine.Tick();Check(engine.Tracking,"Hold released too early");clock=250;engine.Tick();
            Check(!engine.Tracking&&replay.Count==1&&replay[0]==down,"Stationary hold did not become native down");replay.Clear();
            modified=true;int before=lookups;Check(!engine.Process(down)&&lookups==before,"Keyboard modifier did not bypass capture");modified=false;
            foreach(var button in new[]{MouseButtons.Left,MouseButtons.Middle,MouseButtons.XButton1,MouseButtons.XButton2}){
                engine.Process(down);engine.Process(Event(100,100,MouseHook.MouseButtonStates.Down,button));
                Check(!engine.Tracking&&replay.Count==2&&replay[1].MouseButton==button,"Unassigned second button was swallowed");
                Check(!engine.Process(Event(100,100,MouseHook.MouseButtonStates.Up,button)),"Native button release was swallowed");replay.Clear();
            }
            engine.Process(down);engine.Process(up);Check(replay.SequenceEqual(new[]{down,up}),"Ordinary click replay changed order");replay.Clear();
            foreach(var pack in config.actionPackages)foreach(var action in pack.actions)action.enabled=false;
            Check(!engine.Process(down),"Scene without enabled actions captured the trigger");
            var encoded=NativeGestureInput.Encode(new[]{down,Event(100,120,MouseHook.MouseButtonStates.Move),up},new Rectangle(-100,0,301,301));
            Check(encoded.Length==5&&(uint)encoded[1].mi.dwFlags==8&&(uint)encoded[4].mi.dwFlags==16&&encoded[0].mi.dx==43690,"Native drag packet lost down, up or multi-monitor position");
            Check(encoded.All(i=>i.mi.dwExtraInfo==(UIntPtr)0x7fu),"Replayed input can re-enter recognition");
			var selected=config.actionPackages[0].actions[0];selected.enabled=true;
			foreach(var gesture in config.gestures){
				selected.gesture=gesture.id;int actionCount=actions.Count;replay.Clear();
				var first=gesture.points[0];engine.Process(Event(first.x,first.y,MouseHook.MouseButtonStates.Down,MouseButtons.Right));
				foreach(var point in gesture.points.Skip(1))engine.Process(Event(point.x,point.y,MouseHook.MouseButtonStates.Move));
				var lastPoint=gesture.points.Last();engine.Process(Event(lastPoint.x,lastPoint.y,MouseHook.MouseButtonStates.Up,MouseButtons.Right));
				Check(actions.Count==actionCount+1&&replay.Count==0,"Enabled direction path did not execute: "+gesture.id);
			}
			selected.gesture="left-then-up";int previousActions=actions.Count;replay.Clear();engine.Process(down);engine.Process(Event(80,100,MouseHook.MouseButtonStates.Move));engine.Process(up);
			Check(actions.Count==previousActions&&replay.Count==3,"Incomplete enabled prefix did not return native input");
			selected.gesture="#1";engine.Process(down);Check(engine.Process(Event(100,100,MouseHook.MouseButtonStates.Down,MouseButtons.Left)),"Enabled second button escaped");engine.Process(up);
			Check(engine.Process(Event(100,100,MouseHook.MouseButtonStates.Up,MouseButtons.Left)),"Late second-button release escaped after its down was consumed");
			selected.gesture="#5";previousActions=actions.Count;engine.Process(down);for(int i=0;i<3;i++)engine.Process(Event(100,100,MouseHook.MouseButtonStates.Wheel,MouseButtons.None,120));engine.Process(up);
			Check(actions.Count==previousActions+3,"Continuous enabled wheel did not execute every step");
			if(Environment.GetEnvironmentVariable("WINONE_STROKE_INPUT_TEST")=="1")NativeRoundTrip();
        }

        [DllImport("user32.dll",EntryPoint="SendInput",SetLastError=true)]private static extern uint Send(uint count,[In] API.INPUT[] inputs,int size);
        private static void Pump(){var until=Stopwatch.StartNew();while(until.ElapsedMilliseconds<60){Application.DoEvents();Thread.Sleep(2);}}
        private static void Inject(MouseHook.MouseActionArgs e){var packets=NativeGestureInput.Encode(new[]{e},SystemInformation.VirtualScreen);for(int i=0;i<packets.Length;i++)packets[i].mi.dwExtraInfo=(UIntPtr)0x66u;Check(Send((uint)packets.Length,packets,Marshal.SizeOf(typeof(API.INPUT)))==packets.Length,"Fixture SendInput failed");Pump();}

        // Opt-in: a real low-level hook and SendInput round trip, targeting
        // only this disposable window. Default tests never inject OS input.
        private static void NativeRoundTrip(){
            Point cursor=Cursor.Position;
            using(var form=new Form{Text="WinOne gesture input fixture",StartPosition=FormStartPosition.CenterScreen,Size=new Size(420,300),TopMost=true}){
                form.Show();form.Activate();Pump();
                var config=Configuration.Parse(File.ReadAllText(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"defaults.json")));
                foreach(var pack in config.actionPackages)foreach(var a in pack.actions)a.enabled=a.gesture=="left"||a.gesture=="right";
                int downs=0,ups=0,drags=0,wheels=0,actions=0;Point origin=Point.Empty;
                form.MouseDown+=(_,e)=>{if(e.Button==MouseButtons.Right){downs++;origin=e.Location;}};
                form.MouseUp+=(_,e)=>{if(e.Button==MouseButtons.Right)ups++;};
                form.MouseMove+=(_,e)=>{if(e.Button==MouseButtons.Right)drags++;};form.MouseWheel+=(_,e)=>wheels+=e.Delta;
                var engine=new GestureEngine(config,p=>new WindowTarget{processId=Process.GetCurrentProcess().Id,window=form.Bounds.Contains(p)?form.Handle.ToInt64():0,processPath=Path.Combine(@"C:/Windows","explorer.exe"),windowClass=form.Bounds.Contains(p)?"CabinetWClass":"Progman"},(a,b)=>{},()=>{},(a,m,t)=>actions++,NativeGestureInput.Replay,new ScopePolicy(config,@"C:/Windows"),modified:()=>false);
                Point start=form.PointToScreen(new Point(120,80)),end=form.PointToScreen(new Point(120,155));
                MouseHook.MouseAction+=engine.Process;MouseHook.StartHook();
                try{
                    Inject(Event(start.X,start.Y,MouseHook.MouseButtonStates.Down,MouseButtons.Right));
                    Inject(Event(end.X,end.Y,MouseHook.MouseButtonStates.Move));
                    Inject(Event(end.X,end.Y,MouseHook.MouseButtonStates.Up,MouseButtons.Right));
                    Check(downs==1&&ups==1&&drags>0&&origin==new Point(120,80)&&actions==0,"Native target did not receive the rejected vertical drag from its original point");
                    Inject(Event(start.X,start.Y,MouseHook.MouseButtonStates.Down,MouseButtons.Right));
                    Inject(Event(start.X-60,start.Y,MouseHook.MouseButtonStates.Move));
                    Inject(Event(start.X-60,start.Y,MouseHook.MouseButtonStates.Up,MouseButtons.Right));
                    Check(actions==1&&downs==1&&ups==1,"Enabled horizontal gesture leaked a native click");
                    Inject(Event(start.X,start.Y,MouseHook.MouseButtonStates.Down,MouseButtons.Right));
                    Inject(Event(start.X,start.Y,MouseHook.MouseButtonStates.Wheel,MouseButtons.None,120));
                    Inject(Event(start.X,start.Y,MouseHook.MouseButtonStates.Up,MouseButtons.Right));
                    Check(downs==2&&ups==2&&wheels==120,"Unassigned wheel did not reach the native target");
                    Console.WriteLine("PASS: real hook -> SendInput -> owned native window: vertical right-drag, enabled horizontal gesture, unassigned wheel");
                }finally{MouseHook.StopHook();MouseHook.MouseAction-=engine.Process;engine.Cancel();NativeGestureInput.Replay(new[]{Event(start.X,start.Y,MouseHook.MouseButtonStates.Up,MouseButtons.Right)});Cursor.Position=cursor;}
            }
        }
    }
}
