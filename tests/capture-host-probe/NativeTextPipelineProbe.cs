using System.Diagnostics;
using System.IO.Pipes;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Runtime.InteropServices;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

// Real capture canvas -> independent result UI -> shared OCR/AI service. Only
// generated pixels and the explicitly selected loopback test provider are used.
internal static class NativeTextPipelineProbe
{
    [DllImport("user32.dll")]private static extern IntPtr SendMessageW(IntPtr window,uint message,IntPtr w,IntPtr l);
    [DllImport("user32.dll")]private static extern IntPtr GetForegroundWindow();
    private static IntPtr At(int x,int y)=>new((y<<16)|(x&0xffff));
    internal static int Run(string output)
    {
        if(Environment.GetEnvironmentVariable("WINONE_TEXT_PIPELINE_VISIBLE_TEST")!="1")throw new InvalidOperationException("Opt in to the owned visible result test");
        var bundle=Environment.GetEnvironmentVariable("WINONE_TEXT_PIPELINE_BUNDLE")??throw new InvalidOperationException("Set WINONE_TEXT_PIPELINE_BUNDLE");
        var provider=Environment.GetEnvironmentVariable("WINONE_TEXT_PIPELINE_AI_URL")??throw new InvalidOperationException("Set a loopback AI fixture");
        var secondProvider=Environment.GetEnvironmentVariable("WINONE_TEXT_PIPELINE_SECOND_AI_URL")??throw new InvalidOperationException("Set the second loopback AI fixture");
        if(!Uri.TryCreate(provider,UriKind.Absolute,out var url)||!url.IsLoopback||url.Scheme!="http")throw new InvalidOperationException("Only a loopback fixture is allowed");
        if(!Uri.TryCreate(secondProvider,UriKind.Absolute,out var secondUrl)||!secondUrl.IsLoopback||secondUrl.Scheme!="http")throw new InvalidOperationException("Only a loopback fixture is allowed");
        var translate=Environment.GetEnvironmentVariable("WINONE_TEXT_PIPELINE_MODE")!="ocr";
        Directory.CreateDirectory(Path.Combine(output,"data"));
        File.WriteAllText(Path.Combine(output,"data","settings.json"),JsonSerializer.Serialize(new{schemaVersion=1,language="zh-CN",theme=Environment.GetEnvironmentVariable("WINONE_TEXT_PIPELINE_THEME")??"light",captureShortcut="",pinShortcut="",ocrShortcut="",translateShortcut=""}));
        var pipe="WinOne-text-pipeline-"+Guid.NewGuid().ToString("N");
        var info=new ProcessStartInfo(Path.Combine(bundle,"WinOne.Host.exe")){UseShellExecute=false,CreateNoWindow=true,RedirectStandardError=true};
        info.ArgumentList.Add("--standard-session");info.ArgumentList.Add("--pipe");info.ArgumentList.Add(@"\\.\pipe\"+pipe);info.ArgumentList.Add("--data-dir");info.ArgumentList.Add(Path.Combine(output,"data"));
        var clipboard=SnapshotClipboard();
        using var host=Process.Start(info)??throw new Exception("Host did not start");
        host.ErrorDataReceived+=(_,args)=>{if(args.Data is not null){lock(host)File.AppendAllText(Path.Combine(output,"host.log"),args.Data+Environment.NewLine);}};host.BeginErrorReadLine();
        try{
            using var timeout=new CancellationTokenSource(TimeSpan.FromSeconds(Environment.GetEnvironmentVariable("WINONE_RESULT_MANUAL_CHECK")=="1"?300:65));
            _=Call(pipe,"snapshot",new{},timeout.Token).GetAwaiter().GetResult();
            var settings=Call(pipe,"ai.configuration.get",new{},timeout.Token).GetAwaiter().GetResult();
            var setup=JsonSerializer.SerializeToNode(new{configuration=new{revision=settings.GetProperty("revision").GetUInt64(),engine="llm",ocrEngine="ai",target="zh-CN",prompt="",showReasoning=Environment.GetEnvironmentVariable("WINONE_TEXT_PIPELINE_HIDE_REASONING")!="1",providers=new[]{new{id="first",name="服务一",authMode="api-key",protocol="chat",baseUrl=provider,models=new[]{"fixture-vision-model","gemini-2.5-pro","claude-sonnet-4"}},new{id="second",name="服务二",authMode="api-key",protocol="chat",baseUrl=secondProvider,models=new[]{"gpt-5"}}},tasks=new Dictionary<string,object>{{"default",new{provider="first",model="gemini-2.5-pro"}},{"ocr",new{provider="first",model="fixture-vision-model"}},{"translate",new{provider="first",model="gemini-2.5-pro"}},{"chat",new{provider="second",model="gpt-5"}}}},keys=new Dictionary<string,string>{{"first","fixture-first-key"},{"second","fixture-second-key"}}})!.AsObject();
            if(Environment.GetEnvironmentVariable("WINONE_TEXT_PIPELINE_MULTI_KEYS")=="1"){
                // This fixture exercises the real per-key save boundary and
                // shares one pool across OCR, translation and subsequent chat.
                setup["configuration"]!["providers"]![0]!["keys"]=JsonSerializer.SerializeToNode(new[]{new{id="primary",enabled=true},new{id="second",enabled=true},new{id="third",enabled=true}});
                setup["configuration"]!["providers"]![1]!["keys"]=JsonSerializer.SerializeToNode(new[]{new{id="primary",enabled=true}});
                setup.Remove("keys");
                setup["keyUpdates"]=JsonSerializer.SerializeToNode(new Dictionary<string,object>{{"first",new Dictionary<string,string>{{"primary","fixture-first-key"},{"second","fixture-first-key-2"},{"third","fixture-first-key-3"}}},{"second",new Dictionary<string,string>{{"primary","fixture-second-key"}}}});
            }
            _=Call(pipe,"ai.configuration.save",setup,timeout.Token).GetAwaiter().GetResult();
            var configuration=new NativeHostConfiguration(pipe,host);
            using var image=new Bitmap(800,500);using(var graphics=Graphics.FromImage(image)){graphics.Clear(Color.White);using var font=new Font("Segoe UI",24);graphics.DrawString("Hello world",font,Brushes.Black,new PointF(110,110));}
            using var canvas=new WinOneCaptureCanvas(image);
            using var session=new NativeCaptureSession(canvas,_=>throw new Exception("Handoff copied an image"),_=>{},textService:new NativeCaptureTextService(configuration),textMode:translate?"translate":null,copyText:_=>throw new Exception("Handoff copied text before result interaction"));
            var started=false;var captureClosed=false;Exception? failure=null;Task? activate=null;var clock=Stopwatch.StartNew();
            canvas.Window.FormClosed+=(_,_)=>captureClosed=true;
            using var timer=new System.Windows.Forms.Timer{Interval=25};
            timer.Tick+=(_,_)=>{try{
                if(clock.Elapsed.TotalSeconds>15)throw new Exception($"Result handoff timed out: {session.Error}; busy={session.RecognitionBusy}");if(!session.Ready||started)return;
                if(activate is null){var handle=canvas.Window.Handle;activate=Task.Run(()=>ResultWindowAutomationProbe.FocusOwned(handle));return;}
                if(!activate.IsCompleted)return;activate.GetAwaiter().GetResult();
                // UIA SetFocus acknowledges the request before Windows can
                // finish foreground activation. The existing deadline bounds it.
                if(GetForegroundWindow()!=canvas.Window.Handle)return;
                started=true;
                SendMessageW(canvas.Window.Handle,0x0200,IntPtr.Zero,At(80,80));SendMessageW(canvas.Window.Handle,0x0201,new IntPtr(1),At(80,80));SendMessageW(canvas.Window.Handle,0x0200,new IntPtr(1),At(600,260));SendMessageW(canvas.Window.Handle,0x0202,IntPtr.Zero,At(600,260));
                if(!translate)session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row=>row.Items.Cast<ToolStripItem>()).Single(item=>item.Name=="ocr").PerformClick();
            }catch(Exception error){failure=error;canvas.Window.Close();}};
            timer.Start();Application.Run(canvas.Window);timer.Stop();if(failure is not null)throw failure;
            if(!captureClosed||!session.ResultPresented||session.Copied||string.IsNullOrEmpty(session.ResultWindowId))throw new Exception("Result did not end the screenshot independently");
            var state=Call(pipe,"results.status",new{id=session.ResultWindowId},timeout.Token).GetAwaiter().GetResult();
            var resultWindow=new IntPtr(long.Parse(state.GetProperty("window").GetString()!));var resultPid=state.GetProperty("processId").GetInt32();
            using var resultProcess=Process.GetProcessById(resultPid);
            Task.Run(()=>ResultWindowAutomationProbe.Run(resultWindow,resultPid,output,translate)).GetAwaiter().GetResult();
            var after=Call(pipe,"ai.configuration.get",new{},timeout.Token).GetAwaiter().GetResult();
            if(after.GetProperty("tasks").GetProperty("chat").GetProperty("provider").GetString()!="second")throw new Exception("Window selection changed the global chat assignment");
            if(Clipboard.GetText()!="Hello world")throw new Exception("Copy original changed after AI conversation");
            using(var outside=new Form{Text="Owned outside-click target",Size=new Size(180,100),StartPosition=FormStartPosition.Manual,Location=new Point(30,30)}){
                // UIA focus requests and cross-process activation need a live
                // message loop on the target, not a blocked WinForms thread.
                Exception? focusFailure=null;
                outside.Shown+=(_,_)=>{var handle=outside.Handle;Task.Run(()=>{
                    try{ResultWindowAutomationProbe.FocusOutsideAndWaitClosed(handle,resultWindow);}
                    catch(Exception error){focusFailure=error;}
                    finally{outside.BeginInvoke((Action)(()=>outside.Close()));}
                });};
                Application.Run(outside);
                if(focusFailure is not null)throw focusFailure;
            }
            if(!resultProcess.WaitForExit(5000))throw new Exception("Last result window left its host resident");
            if(!TrayExitProbe.RequestExit(host.Id)||!host.WaitForExit(8000)||host.ExitCode!=0)throw new Exception("Host did not exit normally");
            File.WriteAllText(Path.Combine(output,"result.json"),JsonSerializer.Serialize(new{original="Hello world",translation=translate?"你好，世界":"",seconds=clock.Elapsed.TotalSeconds,sharedAI=true,originalCopied=true,conversationTurns=2,resized=true,moved=true,outsideDismissed=true}));
            File.WriteAllText(Path.Combine(output,"lifecycle.json"),JsonSerializer.Serialize(new{captureClosed,resultClosed=true,resultProcessExited=true,hostExited=true,hostExitCode=host.ExitCode}));
            return 0;
        }finally{
            try{if(clipboard is null)Clipboard.Clear();else Clipboard.SetDataObject(clipboard,true,30,50);}
            finally{if(!host.HasExited){TrayExitProbe.RequestExit(host.Id);if(!host.WaitForExit(3000))host.Kill(true);}}
        }
    }
    private static IDataObject? SnapshotClipboard(){
        IDataObject? original=null;
        var deadline=DateTime.UtcNow.AddSeconds(15);
        while(true){
            try{original=Clipboard.GetDataObject();break;}
            catch(ExternalException error)when(error.ErrorCode==unchecked((int)0x800401D0)&&DateTime.UtcNow<deadline){Thread.Sleep(30);}
        }
        if(original is null)return null;
        // Read the formats before changing clipboard ownership. Retaining the
        // live proxy may attempt delayed rendering while restoring the clipboard.
        var snapshot=new DataObject();
        foreach(var format in original.GetFormats(false)){
            var data=original.GetData(format,false)??throw new Exception("Cannot preserve clipboard format: "+format);
            if(data is Image image)data=image.Clone();else if(data is MemoryStream stream)data=new MemoryStream(stream.ToArray());else if(data is byte[] bytes)data=bytes.ToArray();
            snapshot.SetData(format,false,data);
        }
        return snapshot;
    }
    private static async Task<JsonElement> Call(string name,string method,object parameters,CancellationToken cancellation){
        using var pipe=new NamedPipeClientStream(".",name,PipeDirection.InOut,PipeOptions.Asynchronous);await pipe.ConnectAsync(cancellation);
        await PipeProtocol.WriteAsync(pipe,JsonSerializer.SerializeToUtf8Bytes(new{version=1,id=Guid.NewGuid().ToString("N"),method,@params=parameters}),cancellation);
        using var response=JsonDocument.Parse(await PipeProtocol.ReadAsync(pipe,cancellation));if(response.RootElement.TryGetProperty("error",out var error))throw new Exception(error.ToString());return response.RootElement.GetProperty("result").Clone();
    }
}
