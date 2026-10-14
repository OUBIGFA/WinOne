using System;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Text;
using System.Windows.Forms;
using Stroke;

namespace WinOne.StrokeRuntime
{
    internal sealed class RecordInput {public string id{get;set;}public string gestureId{get;set;}public bool blend{get;set;}public PointValue[] points{get;set;}public int[][] priorVectors{get;set;}}
    internal sealed class StrokeApplication:IDisposable
    {
        private readonly Control ui=new Control();private readonly string directory;private readonly bool fixture;
        private Configuration config;private ScriptPlan plan;private GestureEngine engine;private TrailWindow trail;private ScriptSupervisor scripts;private readonly InputLedger input;
        private string recording;private bool running,stopped,shutdownConfirmed;private int fixtureInputs;
        internal LocalServer Server;
        private static int RuntimeThreads(){using(var process=Process.GetCurrentProcess())return process.Threads.Count;}
        internal StrokeApplication(string directory,bool fixture){this.directory=directory;this.fixture=fixture;var handle=ui.Handle;input=fixture?new InputLedger(value=>{System.Threading.Interlocked.Increment(ref fixtureInputs);return 1;},key=>false):new InputLedger();}
        private T OnUI<T>(Func<T> run){if(ui.InvokeRequired)return (T)ui.Invoke(run);return run();}
        private Configuration ReadConfiguration(){string path=Path.Combine(directory,"managed-settings.json");var file=new FileInfo(path);if(!file.Exists||file.Length>1024*1024)throw new InvalidDataException("Host 管理的鼠标手势配置缺失或过大");return Configuration.Parse(File.ReadAllText(path,Encoding.UTF8));}
        internal object Handle(Request request)
        {
            switch(request.method){
                case "snapshot":return OnUI(()=>new{processId=Process.GetCurrentProcess().Id,configurationManaged=true,runtimeThreads=RuntimeThreads(),actionRuntimeThreads=scripts==null?0:scripts.RuntimeThreadCount,running=running,busy=engine!=null&&engine.Tracking,mouseHook=MouseHook.Enable,scriptWorkers=scripts==null?0:scripts.WorkerCount,scriptThreads=scripts==null?0:scripts.ThreadCount,actionProcessId=scripts==null?0:scripts.ProcessId,queued=scripts==null?0:scripts.Queued,diagnostic=engine!=null&&engine.Diagnostic!=null?engine.Diagnostic:scripts==null?null:scripts.LastError,fixtureInputs=fixture?fixtureInputs:0});
                case "fixture.execute":if(!fixture)throw new InvalidOperationException("Fixture mode is disabled");var execution=Json.Params<ActionJob>(request.@params);return OnUI<object>(()=>{if(scripts==null)throw new InvalidOperationException("Not activated");var target=NativeWindowIdentity.FromWindow(ui.Handle);target.point=new PointValue{x=0,y=0};scripts.Enqueue(execution.name,execution.mark,target);return new{queued=true};});
                case "activate":case "settings.reload":{
                    Configuration next=ReadConfiguration();ScriptPlan compiled=ScriptCompiler.Compile(next,Path.Combine(directory,"cache"));bool enable=request.method=="activate"||OnUI(()=>running);
                    return OnUI<object>(()=>{StopResources();config=next;plan=compiled;stopped=false;if(enable)StartResources();return new{applied=true,running=running,revision=config.revision};});
                }
                case "record.start":{
                    // Recording is local to WinOne's settings canvas. It never
                    // temporarily opens the global hook to an excluded app.
                    recording=Guid.NewGuid().ToString("N");return new{id=recording,state="recording"};
                }
                case "record.read":{
                    var value=Json.Params<RecordInput>(request.@params);if(value==null||value.id!=recording)throw new InvalidOperationException("录制会话已结束，请重新录制");
                    var definition=new GestureDefinition{id="recording",name="recording",points=value.points};var gesture=Configuration.ToGesture(definition);if(gesture.Vectors==null)throw new InvalidDataException("请绘制有移动的手势轨迹");Configuration current=config??ReadConfiguration();
                    if(value.blend&&!String.IsNullOrEmpty(value.gestureId)){var prior=value.priorVectors!=null?new GestureDefinition{id=value.gestureId,vectors=value.priorVectors}:current.gestures.FirstOrDefault(g=>g.id==value.gestureId);if(prior==null)throw new InvalidDataException("原手势不存在");var old=Configuration.ToGesture(prior);if(old.Vectors!=null)for(int i=0;i<128;i++){double x=old.Vectors[i].X*.9+gesture.Vectors[i].X*.1,y=old.Vectors[i].Y*.9+gesture.Vectors[i].Y*.1;double distance=Math.Sqrt(x*x+y*y);if(distance>0)gesture.Vectors[i]=new Gesture.Vector((sbyte)(x*127/distance),(sbyte)(y*127/distance));}}
                    var closest=current.gestures.Select(g=>new{definition=g,gesture=Configuration.ToGesture(g)}).Where(g=>g.gesture.Vectors!=null).Select(g=>new{id=g.definition.id,name=g.definition.name,score=gesture.Similarity(g.gesture)}).OrderByDescending(g=>g.score).FirstOrDefault();
                    return new{id=recording,state="done",points=value.points,vectors=gesture.Vectors.Select(v=>new[]{(int)v.X,(int)v.Y}).ToArray(),match=closest};
                }
                case "record.cancel":var cancellation=Json.Params<RecordInput>(request.@params);bool canceled=cancellation!=null&&cancellation.id==recording;if(canceled)recording=null;return new{canceled=canceled};
                case "shutdown":return OnUI<object>(()=>{ScriptCompiler.RequestStop();StopResources();recording=null;shutdownConfirmed=true;return new{restored=true,processId=Process.GetCurrentProcess().Id,fixtureInputs=fixture?fixtureInputs:0};});
                default:throw new InvalidOperationException("Unsupported Stroke method");
            }
        }
        private void StartResources()
        {
            config.ApplyBase();scripts=new ScriptSupervisor(plan,input,definition=>{if(ui.IsDisposed)return;ui.BeginInvoke(new System.Action(()=>{if(!running)return;definition.enabled=config.pen.enabled;if(trail!=null){trail.Dispose();trail=null;}trail=new TrailWindow(definition);}));});
            trail=new TrailWindow(config.pen);
            engine=new GestureEngine(config,NativeWindowIdentity.At,(a,b)=>{if(trail!=null)trail.Segment(a,b);},()=>{if(trail!=null)trail.ClearTrail();},(name,mark,target)=>scripts.Enqueue(name,mark,target),()=>global::Stroke.Base.MouseClick(config.Button));
            MouseHook.MouseAction+=engine.Process;
            try{if(!fixture)MouseHook.StartHook();running=true;}
            catch{MouseHook.MouseAction-=engine.Process;engine=null;if(trail!=null){trail.Dispose();trail=null;}scripts.Dispose();scripts=null;throw;}
        }
        private void StopResources()
        {
            ScriptCompiler.CancelActive();
            // Input ownership ends before waiting for any user script.
            if(MouseHook.Enable)MouseHook.StopHook();
            if(engine!=null){MouseHook.MouseAction-=engine.Process;engine.Cancel();engine=null;}
            if(trail!=null){trail.Dispose();trail=null;}
            running=false;
            if(scripts!=null){scripts.Dispose();scripts=null;}
            input.ReleaseAll();stopped=true;
        }
        internal void AfterReply(string method){if(method=="shutdown"&&shutdownConfirmed)ui.BeginInvoke(new System.Action(Application.ExitThread));}
        internal void ParentGone(){ScriptCompiler.RequestStop();if(ui.IsDisposed)return;ui.BeginInvoke(new System.Action(()=>{try{StopResources();}catch(Exception error){Console.Error.WriteLine("stroke.shutdown: "+error.Message);Environment.ExitCode=1;}Application.ExitThread();}));}
        public void Dispose(){ScriptCompiler.RequestStop();if(!stopped)StopResources();if(Server!=null)Server.Dispose();ui.Dispose();}
    }
    internal static class Program
    {
        [STAThread]internal static int Main(string[] args)
        {
            string pipe=null,directory=null;int parent=0;bool action=false,fixture=false;
            try{
                // WinExe has no console code page. Explicit UTF-8 streams work
                // for Host-owned pipes without SetConsoleOutputCP or a window.
                Console.SetOut(new StreamWriter(Console.OpenStandardOutput(),new UTF8Encoding(false)){AutoFlush=true});
                Console.SetError(new StreamWriter(Console.OpenStandardError(),new UTF8Encoding(false)){AutoFlush=true});
                AppDomain.CurrentDomain.UnhandledException+=(_,eventArgs)=>{Console.Error.WriteLine("stroke.unhandled: "+eventArgs.ExceptionObject.GetType().Name);Environment.Exit(1);};
                for(int i=0;i<args.Length;i++){switch(args[i]){case "--pipe":pipe=args[++i];break;case "--data-dir":directory=args[++i];break;case "--parent":parent=Int32.Parse(args[++i]);break;case "--action-worker":action=true;break;case "--fixture":fixture=true;break;default:throw new ArgumentException("Unknown Stroke argument");}}
                if(parent<=0)throw new ArgumentException("Stroke needs an owning parent process");
                if(action)return ScriptSupervisor.RunActionWorker(parent);
                if(String.IsNullOrWhiteSpace(pipe)||String.IsNullOrWhiteSpace(directory)||!Path.IsPathRooted(directory))throw new ArgumentException("Missing pipe or absolute data directory");
                Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);Application.SetUnhandledExceptionMode(UnhandledExceptionMode.ThrowException);
                using(var owner=Process.GetProcessById(parent))using(var app=new StrokeApplication(directory,fixture)){
                    app.Server=new LocalServer(pipe,parent,app.Handle);app.Server.AfterReply=app.AfterReply;app.Server.Faulted=app.ParentGone;app.Server.Start();
                    owner.EnableRaisingEvents=true;owner.Exited+=(_,__)=>app.ParentGone();if(owner.HasExited)app.ParentGone();Application.Run();return Environment.ExitCode;
                }
            }catch(Exception error){Console.Error.WriteLine("stroke: "+error);return 1;}
        }
    }
}
