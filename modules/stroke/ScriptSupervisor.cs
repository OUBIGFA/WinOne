using System;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;
using Stroke;
using Timer=System.Threading.Timer;

namespace WinOne.StrokeRuntime
{
    internal sealed class ActionJob {public string type;public string id;public string assembly;public string configuration;public string function;public string name;public int mark;public WindowTarget target;public uint sent;public string error;}
    internal sealed class ActionEvent {public string type;public string id;public string job;public InputValue input;public string error;public string name;public int mark;public WindowTarget target;public PenDefinition pen;public int workers;}
    // One lazily-created executor retains upstream Base.Data and keyboard-event
    // subscriptions across its 1–64 STA workers. Only this executor is stopped
    // for an uncooperative script; processes opened by user actions are not killed.
    internal sealed class ScriptSupervisor:IDisposable
    {
        private readonly object gate=new object(),writeGate=new object();private readonly ScriptPlan plan;private readonly InputLedger inputs;private readonly System.Action<PenDefinition> penChanged;
        private readonly BlockingCollection<ActionJob> queue=new BlockingCollection<ActionJob>(64);private readonly Dictionary<string,ActionJob> pending=new Dictionary<string,ActionJob>();
        private readonly Dictionary<string,Timer> deadlines=new Dictionary<string,Timer>();private Task sender;private Task reader;private Process process;private bool closed;private int workers;
        internal string LastError {get;private set;}
        internal int WorkerCount {get{lock(gate)return process==null||process.HasExited?0:1;}}
        internal int ThreadCount {get{lock(gate)return workers;}}
        internal int ProcessId {get{lock(gate)return process==null||process.HasExited?0:process.Id;}}
        internal int RuntimeThreadCount {get{lock(gate)return process==null||process.HasExited?0:process.Threads.Count;}}
        internal int Queued {get{lock(gate)return pending.Count;}}
        internal ScriptSupervisor(ScriptPlan plan,InputLedger inputs,System.Action<PenDefinition> penChanged=null){this.plan=plan;this.inputs=inputs;this.penChanged=penChanged;}
        internal void Enqueue(string name,int mark,WindowTarget target)
        {
            lock(gate){if(closed)return;if(!plan.functions.ContainsKey(name)){LastError="Unknown action: "+name;return;}if(pending.Count>=64){LastError="动作排队已满";return;}var job=new ActionJob{type="run",id=Guid.NewGuid().ToString("N"),function=plan.functions[name],name=name,mark=mark,target=target};if(!queue.TryAdd(job)){LastError="动作排队已满";return;}pending.Add(job.id,job);if(sender==null)sender=Task.Factory.StartNew(SendLoop,TaskCreationOptions.LongRunning);}
        }
        private void StartExecutor()
        {
            Task previous;lock(gate){if(closed)throw new OperationCanceledException();if(process!=null&&!process.HasExited)return;previous=reader;}
            // Drain the old executor before a new generation can claim jobs.
            // Waiting outside gate lets the reader release each job's inputs.
            if(previous!=null&&!previous.Wait(1500))throw new InvalidOperationException("旧动作执行器尚未清理完成");
            lock(gate){if(closed)throw new OperationCanceledException();if(process!=null&&!process.HasExited)return;if(process!=null){process.Dispose();process=null;}
                var info=new ProcessStartInfo(Assembly.GetExecutingAssembly().Location,"--action-worker --parent "+Process.GetCurrentProcess().Id){UseShellExecute=false,CreateNoWindow=true,RedirectStandardInput=true,RedirectStandardOutput=true,RedirectStandardError=true,StandardOutputEncoding=new UTF8Encoding(false),StandardErrorEncoding=new UTF8Encoding(false)};
                var child=new Process{StartInfo=info};if(!child.Start())throw new InvalidOperationException("Action worker did not start");child.ErrorDataReceived+=(_,eventArgs)=>{if(eventArgs.Data!=null)Console.Error.WriteLine("stroke.executor: "+eventArgs.Data);};child.BeginErrorReadLine();process=child;Write(new ActionJob{type="initialize",assembly=plan.assembly,configuration=Json.Encode(plan.configuration)});reader=Task.Factory.StartNew(()=>ReadLoop(child),TaskCreationOptions.LongRunning);
            }
        }
        private void Write(ActionJob value){lock(writeGate){process.StandardInput.WriteLine(Json.Encode(value));process.StandardInput.Flush();}}
        private void SendLoop()
        {
            foreach(var job in queue.GetConsumingEnumerable()){try{StartExecutor();lock(gate){if(!pending.ContainsKey(job.id))continue;}Write(job);if(plan.configuration.scriptTimeoutSeconds>0){lock(gate){if(pending.ContainsKey(job.id))deadlines[job.id]=new Timer(_=>TimeoutJob(job.id),null,plan.configuration.scriptTimeoutSeconds*1000,Timeout.Infinite);}}}catch(Exception error){Finish(job.id,error.Message);}}
        }
        private void TimeoutJob(string id){lock(gate){if(!pending.ContainsKey(id)||closed)return;LastError="动作超时，执行进程已停止";Kill(process);}}
        private void ReadLoop(Process child)
        {
            try{string line;while((line=child.StandardOutput.ReadLine())!=null){if(line.Length>1024*1024)throw new InvalidDataException("Action response too large");var value=Json.Decode<ActionEvent>(line);
                switch(value.type){
                    case "input":{string error=null;uint sent=0;try{sent=inputs.Send(value.job,value.input);}catch(Exception failure){error=failure.Message;}Write(new ActionJob{type="input.reply",id=value.id,sent=sent,error=error});break;}
                    case "done":Finish(value.id,value.error);break;
                    case "invoke":Enqueue(value.name,value.mark,value.target);break;
                    case "pen":if(penChanged!=null)penChanged(value.pen);break;
                    case "workers":lock(gate)workers=value.workers;break;
                    default:throw new InvalidDataException("Invalid action response");
                }
            }}catch(Exception error){if(!closed){LastError=error.Message;Console.Error.WriteLine("stroke.action: "+error.GetType().Name);}}
            finally{string[] ids;lock(gate)ids=pending.Keys.ToArray();foreach(string id in ids)Finish(id,closed?null:"动作执行器已结束");try{inputs.ReleaseAll();}catch(Exception failure){LastError=failure.Message;}}
        }
        private void Finish(string id,string error)
        {
            lock(gate){Timer timer;if(deadlines.TryGetValue(id,out timer)){timer.Dispose();deadlines.Remove(id);}bool existed=pending.Remove(id);if(error!=null){LastError=error;Console.Error.WriteLine("stroke.action: "+error.Split(':')[0]);}else if(existed)LastError=null;}
            try{if(error!=null)inputs.Release(id);else inputs.Retire(id);}catch(Exception failure){LastError=failure.Message;}
        }
        private static void Kill(Process child){if(child==null)return;try{if(!child.HasExited)child.Kill();}catch(InvalidOperationException){}}
        public void Dispose()
        {
            Process child;lock(gate){if(closed)return;closed=true;queue.CompleteAdding();ActionJob ignored;while(queue.TryTake(out ignored)){};child=process;}
            if(sender!=null&&!sender.Wait(1500))throw new InvalidOperationException("动作发送线程未停止");
            if(child!=null){try{child.StandardInput.Close();}catch(InvalidOperationException){}if(!child.WaitForExit(300))Kill(child);if(!child.WaitForExit(1500))throw new InvalidOperationException("动作执行进程未退出");if(reader!=null&&!reader.Wait(1000))throw new InvalidOperationException("动作响应线程未停止");child.Dispose();}
            string[] ids;lock(gate)ids=pending.Keys.ToArray();foreach(string id in ids)Finish(id,null);inputs.ReleaseAll();queue.Dispose();
        }
        private sealed class StaWorker {internal Control target;internal Thread thread;internal int pending;}
        internal static int RunActionWorker(int parent)
        {
            using(var owner=Process.GetProcessById(parent)){
                owner.EnableRaisingEvents=true;owner.Exited+=(_,__)=>Environment.Exit(0);if(owner.HasExited)return 0;
                TextWriter output=new StreamWriter(Console.OpenStandardOutput(),new UTF8Encoding(false)){AutoFlush=true};TextReader input=new StreamReader(Console.OpenStandardInput(),new UTF8Encoding(false));Console.SetOut(TextWriter.Null);
                object outputGate=new object(),workerGate=new object();var workers=new List<StaWorker>();var replies=new ConcurrentDictionary<string,TaskCompletionSource<ActionJob>>();var current=new AsyncLocal<ActionJob>();
                System.Action<ActionEvent> emit=value=>{lock(outputGate)output.WriteLine(Json.Encode(value));};
                Configuration configuration=null;Assembly scripts=null;
                InputRouting.Sink=value=>{string id=Guid.NewGuid().ToString("N");var completion=new TaskCompletionSource<ActionJob>();replies[id]=completion;emit(new ActionEvent{type="input",id=id,job=current.Value==null?"keyboard-hook":current.Value.id,input=InputValue.From(value)});var reply=completion.Task.GetAwaiter().GetResult();if(reply.error!=null)throw new InvalidOperationException(reply.error);return reply.sent;};
                global::Stroke.Script.Dispatch=(name,mark)=>emit(new ActionEvent{type="invoke",name=name,mark=mark,target=current.Value==null?null:current.Value.target});
                try{string request;while((request=input.ReadLine())!=null){var job=Json.Decode<ActionJob>(request);
                    if(job.type=="input.reply"){TaskCompletionSource<ActionJob> completion;if(replies.TryRemove(job.id,out completion))completion.SetResult(job);continue;}
                    if(job.type=="initialize"){configuration=Configuration.Parse(job.configuration);configuration.ApplyBase();var explicitReferences=configuration.assemblies.Where(Path.IsPathRooted).ToDictionary(path=>AssemblyName.GetAssemblyName(path).Name,path=>path,StringComparer.OrdinalIgnoreCase);AppDomain.CurrentDomain.AssemblyResolve+=(_,requestArgs)=>{string path;return explicitReferences.TryGetValue(new AssemblyName(requestArgs.Name).Name,out path)?Assembly.LoadFrom(path):null;};scripts=Assembly.LoadFrom(job.assembly);global::Stroke.Settings.Pen.PenChanged+=()=>emit(new ActionEvent{type="pen",pen=new PenDefinition{color=System.Drawing.ColorTranslator.ToHtml(global::Stroke.Settings.Pen.Color),opacity=global::Stroke.Settings.Pen.Opacity,thickness=global::Stroke.Settings.Pen.Thickness}});continue;}
                    if(job.type!="run"||scripts==null)throw new InvalidDataException("Executor is not initialized");
                    StaWorker worker;
                    lock(workerGate){worker=workers.OrderBy(w=>w.pending).FirstOrDefault();if(worker==null||worker.pending>0&&workers.Count<configuration.scriptThreadCount){worker=new StaWorker();var created=worker;using(var ready=new ManualResetEvent(false)){created.thread=new Thread(()=>{created.target=new Control();var handle=created.target.Handle;ready.Set();Application.Run();created.target.Dispose();}){IsBackground=true};created.thread.SetApartmentState(ApartmentState.STA);created.thread.Start();ready.WaitOne();}workers.Add(worker);emit(new ActionEvent{type="workers",workers=workers.Count});}worker.pending++;}
                    var selected=worker;var action=job;
                    selected.target.BeginInvoke(new System.Action(()=>{string error=null;current.Value=action;try{var targetNow=NativeWindowIdentity.FromWindow(new IntPtr(action.target.window));if(targetNow==null||targetNow.processId!=action.target.processId||!String.Equals(targetNow.processPath,action.target.processPath,StringComparison.OrdinalIgnoreCase)||targetNow.windowClass!=action.target.windowClass)throw new InvalidOperationException("目标窗口已变化，动作已取消");global::Stroke.Stroke.SetContext(new IntPtr(action.target.window),action.target.processPath,action.target.point.ToPoint());scripts.GetType("Stroke.Scripts",true).GetMethod(action.function).Invoke(null,new object[]{action.mark});}catch(Exception exception){var actual=exception is TargetInvocationException&&exception.InnerException!=null?exception.InnerException:exception;error=actual.GetType().Name+": "+actual.Message;}finally{lock(workerGate)selected.pending--;emit(new ActionEvent{type="done",id=action.id,error=error});}}));
                }}finally{foreach(var pending in replies.Values)pending.TrySetException(new EndOfStreamException());global::Stroke.Base.KeyboardHook.StopHook();foreach(var worker in workers){try{worker.target.BeginInvoke(new System.Action(Application.ExitThread));}catch(InvalidOperationException){}}InputRouting.Sink=null;}
                return 0;
            }
        }
    }
}
