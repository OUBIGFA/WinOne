using System.Diagnostics;
using System.IO.Pipes;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Nodes;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

// Host startup -> eager workers -> native PNG/mapping bridge -> real OCR.
// Only generated text and an explicitly selected repository bundle are used.
internal static class OcrHostLatencyProbe
{
    [DllImport("psapi.dll", SetLastError=true)] private static extern bool EmptyWorkingSet(IntPtr process);
    internal static int Run(string output)
    {
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        Application.EnableVisualStyles();
        var code = 1;
        // One persistent STA loop, like the production capture worker. Modal
        // OCR sessions can await their real service without thread-pool UI or
        // a new WinForms synchronization context for each measurement.
        using var dispatcher = new Form { ShowInTaskbar = false };
        _ = dispatcher.Handle;
        dispatcher.BeginInvoke(async () => {
            try { code = await RunAsync(Path.GetFullPath(output)); }
            catch (Exception error) { Directory.CreateDirectory(output); File.WriteAllText(Path.Combine(output,"failure.txt"),error.ToString()); }
            finally { Application.ExitThread(); }
        });
        Application.Run();
        return code;
    }

    private static async Task<int> RunAsync(string output)
    {
        var privateDesktop = HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal);
        if (!privateDesktop && Environment.GetEnvironmentVariable("WINONE_OCR_HOST_VISIBLE_TEST") != "1") throw new InvalidOperationException("Explicitly select the owned current-desktop test");
        var root = Path.GetFullPath(Environment.CurrentDirectory);
        var bundle = Path.GetFullPath(Environment.GetEnvironmentVariable("WINONE_OCR_HOST_BUNDLE") ?? throw new InvalidOperationException("Set WINONE_OCR_HOST_BUNDLE"));
        if (!bundle.StartsWith(Path.Combine(root, "_temp") + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase)) throw new InvalidOperationException("Use an isolated repository bundle");
        Directory.CreateDirectory(output);
        var profile = Path.Combine(output, "profile");Directory.CreateDirectory(profile);
        File.WriteAllText(Path.Combine(profile,"settings.json"), "{\"schemaVersion\":1,\"language\":\"zh-CN\",\"theme\":\"light\",\"captureShortcut\":\"\",\"pinShortcut\":\"\",\"ocrShortcut\":\"\",\"translateShortcut\":\"\",\"selectionShortcut\":\"\",\"quickAssistantShortcut\":\"\",\"openShortcut\":\"\",\"startup\":{\"runAtLogon\":false,\"keepUIRunning\":false}}");
        using var catalog = JsonDocument.Parse(File.ReadAllBytes(Path.Combine(root,"internal/catalog/builtin.json")));
        var modules = catalog.RootElement.EnumerateArray().ToDictionary(item=>item.GetProperty("id").GetString()!,item=>(object)new {enabled=item.GetProperty("id").GetString()=="capture",pendingStop=false});
        File.WriteAllText(Path.Combine(profile,"module-state.json"),JsonSerializer.Serialize(new {schemaVersion=4,revision=0,modules}));
        var pipe = "WinOne-OCR-Latency-" + Guid.NewGuid().ToString("N");
        var start = new ProcessStartInfo(Path.Combine(bundle,"WinOne.Host.exe")) {UseShellExecute=false,CreateNoWindow=true,RedirectStandardError=true};
        foreach(var arg in new[]{"--standard-session","--pipe",@"\\.\pipe\"+pipe,"--data-dir",profile})start.ArgumentList.Add(arg);
        var clock=Stopwatch.StartNew();
        using var host=Process.Start(start)??throw new InvalidOperationException("Host did not start");
        var diagnostics=host.StandardError.ReadToEndAsync();
        var idleSeconds = int.Parse(Environment.GetEnvironmentVariable("WINONE_OCR_HOST_IDLE_SECONDS") ?? "65");
        if (idleSeconds is < 0 or > 3600) throw new ArgumentOutOfRangeException("OCR host idle measurement");
        using var deadline=new CancellationTokenSource(TimeSpan.FromSeconds(idleSeconds+210));
        Process? ocr=null,capture=null,restarted=null,recovered=null,reenabledOCR=null,reenabledCapture=null;
        async Task<JsonElement> Call(string method,object? parameters=null){
            var id=Guid.NewGuid().ToString("N");
            using var client=new NamedPipeClientStream(".",pipe,PipeDirection.InOut,PipeOptions.Asynchronous);
            await client.ConnectAsync(deadline.Token).ConfigureAwait(false);
            NativeHostConfiguration.VerifyServer(client,host.Id);
            await PipeProtocol.WriteAsync(client,JsonSerializer.SerializeToUtf8Bytes(new {version=1,id,method,@params=parameters??new {}}),deadline.Token).ConfigureAwait(false);
            using var response=JsonDocument.Parse(await PipeProtocol.ReadAsync(client,deadline.Token).ConfigureAwait(false));
            JsonElement Result(JsonElement envelope){
                if(envelope.GetProperty("id").GetString()!=id)throw new InvalidDataException("Host response identity changed");
                if(envelope.TryGetProperty("error",out var error)&&error.ValueKind!=JsonValueKind.Null)throw new InvalidOperationException(error.ToString());
                return envelope.GetProperty("result").Clone();
            }
            var result=Result(response.RootElement);
            if(!result.TryGetProperty("configurationJob",out var ticket))return result;
            var jobId=ticket.GetString();
            if(string.IsNullOrEmpty(jobId))throw new InvalidDataException("Configuration job has no identity");
            // A configuration write is sent once. Poll only the accepted job,
            // matching the production native settings client's completion path.
            while(true){
                var job=await Call("configuration.jobs.get",new {id=jobId}).ConfigureAwait(false);
                if(job.GetProperty("id").GetString()!=jobId)throw new InvalidDataException("Configuration job identity changed");
                var state=job.GetProperty("state").GetString();
                if(state is "completed" or "failed")return Result(job.GetProperty("response"));
                if(state!="running")throw new InvalidDataException("Unexpected configuration job state: "+state);
                await Task.Delay(20,deadline.Token).ConfigureAwait(false);
            }
        }
        static object Resources(Process process){process.Refresh();return new {pid=process.Id,privateBytes=process.PrivateMemorySize64,workingSetBytes=process.WorkingSet64,handles=process.HandleCount,cpuSeconds=process.TotalProcessorTime.TotalSeconds};}
        async Task<Process> WaitOCR(int excluded=0){
            while(true){
                var state=await Call("ai.ocr.status");
                if(state.GetProperty("ready").GetBoolean()&&state.GetProperty("processId").GetInt32() is var pid&&pid>0&&pid!=excluded){var process=Process.GetProcessById(pid);_=process.Handle;return process;}
                await Task.Delay(20,deadline.Token);
            }
        }
        async Task SetCaptureEnabled(bool enabled){
            var snapshot=await Call("modules.snapshot");
            var job=await Call("modules.setEnabled",new {id="capture",enabled,revision=snapshot.GetProperty("revision").GetUInt64()});
            while(job.GetProperty("state").GetString()=="running"){
                await Task.Delay(20,deadline.Token);
                job=await Call("modules.jobs.get",new {id=job.GetProperty("id").GetString()});
            }
            if(job.GetProperty("state").GetString()!="completed")throw new InvalidOperationException("Capture enablement did not finish: "+job);
        }
        try{
            ocr=await WaitOCR();var ocrReadyMs=clock.Elapsed.TotalMilliseconds;
            JsonElement captureState;
            do{captureState=await Call("capture.snapshot");if(!captureState.GetProperty("prepared").GetBoolean())await Task.Delay(20,deadline.Token);}while(!captureState.GetProperty("prepared").GetBoolean());
            capture=Process.GetProcessById(captureState.GetProperty("processId").GetInt32());_=capture.Handle;
            var preparationsBefore=captureState.GetProperty("preparations").GetInt32();
            var allReadyMs=clock.Elapsed.TotalMilliseconds;
            using var image=new Bitmap(260,65);
            using(var graphics=Graphics.FromImage(image)){graphics.Clear(Color.White);using var font=new Font("Segoe UI",24,GraphicsUnit.Pixel);graphics.DrawString("Hello 123",font,Brushes.Black,new PointF(10,8));}
            var client=new NativeHostConfiguration(pipe,host);var samples=new List<object>();
            var copySamples=new List<object>();
            async Task Recognize(string phase){var watch=Stopwatch.StartNew();var text=await client.RecognizeAsync(image,deadline.Token);if(text!="Hello 123")throw new InvalidOperationException("Unexpected generated OCR text: "+text);samples.Add(new {phase,milliseconds=watch.Elapsed.TotalMilliseconds});}
            await Recognize("first-after-startup-preparation");
            for(var i=0;i<4;i++)await Recognize("warm");
            if(privateDesktop) {
                copySamples.Add(await NativeOcrCopyLatencyProbe.Sample("first-session-in-probe",client));
                for(var i=0;i<4;i++)copySamples.Add(await NativeOcrCopyLatencyProbe.Sample("warm",client));
            }
            await Task.Delay(500,deadline.Token);
            var beforeIdleResources=new {host=Resources(host),capture=Resources(capture),ocr=Resources(ocr)};
            host.Refresh();ocr.Refresh();capture.Refresh();var cpu=host.TotalProcessorTime+ocr.TotalProcessorTime+capture.TotalProcessorTime;
            File.WriteAllText(Path.Combine(output,"idle-started.json"),JsonSerializer.Serialize(new {startedUtc=DateTime.UtcNow,idleSeconds,host=host.Id,capture=capture.Id,ocr=ocr.Id}));
            var idle=Stopwatch.StartNew();await Task.Delay(TimeSpan.FromSeconds(idleSeconds),deadline.Token);idle.Stop();
            host.Refresh();ocr.Refresh();capture.Refresh();var idleCpu=(host.TotalProcessorTime+ocr.TotalProcessorTime+capture.TotalProcessorTime-cpu).TotalSeconds;
            var idleResources=new {host=Resources(host),capture=Resources(capture),ocr=Resources(ocr)};
            // The OCR shortcut must be the first request after the idle period;
            // a direct-recognition sample would otherwise warm it first.
            if(privateDesktop)copySamples.Add(await NativeOcrCopyLatencyProbe.Sample("first-after-idle",client));
            await Recognize("after-idle");
            var afterIdle=await Call("capture.snapshot");
            var preparationsAfter=afterIdle.GetProperty("preparations").GetInt32();
            if(idleSeconds>=65&&preparationsAfter<=preparationsBefore)throw new InvalidOperationException("Idle capture never refreshed its prepared path");
            if(afterIdle.GetProperty("busy").GetBoolean()||afterIdle.GetProperty("pins").GetInt32()!=0)throw new InvalidOperationException("Idle preparation retained an active capture");
            var status=await Call("ai.ocr.status");if(status.GetProperty("processId").GetInt32()!=ocr.Id)throw new InvalidOperationException("Idle OCR restarted");
            File.WriteAllText(Path.Combine(output,"host-latency.json"),JsonSerializer.Serialize(new {privateDesktop,ocrReadyMs,allReadyMs,idleSeconds=idle.Elapsed.TotalSeconds,idleCpuSeconds=idleCpu,beforeIdleResources,idleResources,preparationsBefore,preparationsAfter,samples,copySamples,allChildrenExited=false},new JsonSerializerOptions{WriteIndented=true}));
            if(privateDesktop) {
                // Explicit pressure on this invocation's processes only. This
                // is a test, never the product's standby or memory policy.
                foreach(var process in new[]{host,capture,ocr})
                    if(!EmptyWorkingSet(process.Handle))throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
                copySamples.Add(await NativeOcrCopyLatencyProbe.Sample("first-after-owned-working-set-trim",client));
            }
            // A prepared interpreter must recover from its exit notification,
            // before any recognition request restarts it on the user's path.
            var recovery=Stopwatch.StartNew();ocr.Kill();await ocr.WaitForExitAsync(deadline.Token);
            recovered=await WaitOCR(ocr.Id);
            if(recovered.Id==ocr.Id)throw new InvalidOperationException("Background OCR recovery retained the dead process");
            var recoveryMs=recovery.Elapsed.TotalMilliseconds;
            if(privateDesktop)copySamples.Add(await NativeOcrCopyLatencyProbe.Sample("after-background-worker-recovery",client));
            await Recognize("after-background-worker-recovery");
            var configuration=JsonNode.Parse((await Call("ai.configuration.get")).GetRawText())!.AsObject();configuration["ocrEngine"]="ai";
            var saved=await Call("ai.configuration.save",new {configuration});
            if(!recovered.WaitForExit(5000))throw new InvalidOperationException("Selecting AI OCR retained the local worker");
            configuration=JsonNode.Parse(saved.GetRawText())!.AsObject();configuration["ocrEngine"]="local";
            await Call("ai.configuration.save",new {configuration});restarted=await WaitOCR();
            if(privateDesktop)copySamples.Add(await NativeOcrCopyLatencyProbe.Sample("after-local-engine-reenabled",client));
            await Recognize("after-local-engine-reenabled");
            await SetCaptureEnabled(false);
            if(!capture.WaitForExit(5000)||!restarted.WaitForExit(5000))throw new InvalidOperationException("Disabling capture retained its workers");
            await Task.Delay(TimeSpan.FromSeconds(35),deadline.Token);
            status=await Call("ai.ocr.status");
            if(status.GetProperty("ready").GetBoolean()||status.GetProperty("processId").GetInt32()!=0)throw new InvalidOperationException("Maintenance resurrected a disabled OCR worker");
            await SetCaptureEnabled(true);reenabledOCR=await WaitOCR();
            do{captureState=await Call("capture.snapshot");if(!captureState.GetProperty("prepared").GetBoolean())await Task.Delay(20,deadline.Token);}while(!captureState.GetProperty("prepared").GetBoolean());
            reenabledCapture=Process.GetProcessById(captureState.GetProperty("processId").GetInt32());_=reenabledCapture.Handle;
            if(privateDesktop)copySamples.Add(await NativeOcrCopyLatencyProbe.Sample("after-capture-module-reenabled",client));
            File.WriteAllText(Path.Combine(output,"host-latency.json"),JsonSerializer.Serialize(new {privateDesktop,ocrReadyMs,allReadyMs,idleSeconds=idle.Elapsed.TotalSeconds,idleCpuSeconds=idleCpu,beforeIdleResources,idleResources,samples,copySamples,engineSwitchReleased=true,moduleDisabledReleased=true,allChildrenExited=false},new JsonSerializerOptions{WriteIndented=true}));
            var shut=Stopwatch.StartNew();
            var closing=privateDesktop?TrayExitProbe.RequestPrivateHostClose(host.Id):TrayExitProbe.RequestExit(host.Id);
            if(!closing||!host.WaitForExit(8000)||host.ExitCode!=0)throw new InvalidOperationException("Host did not exit normally");
            if(!reenabledCapture.WaitForExit(5000)||!reenabledOCR.WaitForExit(5000))throw new InvalidOperationException("OCR/capture survived Host exit");
            File.WriteAllText(Path.Combine(output,"host-latency.json"),JsonSerializer.Serialize(new {scope="Actual Host/capture/Python group; generated text through the production PNG mapping and job bridge. Copy samples run Shift+C's native OCR session with real HWND selection and an in-memory copy receiver on a private desktop. No physical global hotkey, desktop acquisition, system clipboard, external models or real suspend. Private Host closes through WM_CLOSE, not an Explorer tray click.",privateDesktop,ocrReadyMs,allReadyMs,idleSeconds=idle.Elapsed.TotalSeconds,idleCpuSeconds=idleCpu,beforeIdleResources,idleResources,preparationsBefore,preparationsAfter,recoveryMs,samples,copySamples,engineSwitchReleased=true,moduleDisabledReleased=true,normalExitMs=shut.Elapsed.TotalMilliseconds,allChildrenExited=true},new JsonSerializerOptions{WriteIndented=true}));
            return 0;
        }catch(Exception error){File.WriteAllText(Path.Combine(output,"failure.txt"),error.ToString());return 1;}
        finally{
            if(!host.HasExited){if(privateDesktop)TrayExitProbe.RequestPrivateHostClose(host.Id);else TrayExitProbe.RequestExit(host.Id);if(!host.WaitForExit(3000)){host.Kill();host.WaitForExit();}}
            foreach(var process in new[]{ocr,capture,restarted,recovered,reenabledOCR,reenabledCapture})process?.Dispose();
            File.WriteAllText(Path.Combine(output,"host-stderr.txt"),await diagnostics);
        }
    }
}
