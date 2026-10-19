using System.Diagnostics;
using System.IO.Pipes;
using System.Text.Json;
using System.Text.Json.Nodes;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

// Host startup -> eager workers -> native PNG/mapping bridge -> real OCR.
// Only generated text and an explicitly selected repository bundle are used.
internal static class OcrHostLatencyProbe
{
    internal static int Run(string output) => RunAsync(Path.GetFullPath(output)).GetAwaiter().GetResult();

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
        using var deadline=new CancellationTokenSource(TimeSpan.FromSeconds(idleSeconds+60));
        Process? ocr=null,capture=null,restarted=null;
        async Task<JsonElement> Call(string method,object? parameters=null){
            using var client=new NamedPipeClientStream(".",pipe,PipeDirection.InOut,PipeOptions.Asynchronous);
            await client.ConnectAsync(deadline.Token).ConfigureAwait(false);
            NativeHostConfiguration.VerifyServer(client,host.Id);
            await PipeProtocol.WriteAsync(client,JsonSerializer.SerializeToUtf8Bytes(new {version=1,id=Guid.NewGuid().ToString("N"),method,@params=parameters??new {}}),deadline.Token).ConfigureAwait(false);
            using var response=JsonDocument.Parse(await PipeProtocol.ReadAsync(client,deadline.Token).ConfigureAwait(false));
            if(response.RootElement.TryGetProperty("error",out var error)&&error.ValueKind!=JsonValueKind.Null)throw new InvalidOperationException(error.ToString());
            return response.RootElement.GetProperty("result").Clone();
        }
        static object Resources(Process process){process.Refresh();return new {pid=process.Id,privateBytes=process.PrivateMemorySize64,workingSetBytes=process.WorkingSet64,handles=process.HandleCount,cpuSeconds=process.TotalProcessorTime.TotalSeconds};}
        async Task<Process> WaitOCR(){
            while(true){
                var state=await Call("ai.ocr.status");
                if(state.GetProperty("ready").GetBoolean()&&state.GetProperty("processId").GetInt32() is var pid&&pid>0){var process=Process.GetProcessById(pid);_=process.Handle;return process;}
                await Task.Delay(20,deadline.Token);
            }
        }
        try{
            ocr=await WaitOCR();var ocrReadyMs=clock.Elapsed.TotalMilliseconds;
            JsonElement captureState;
            do{captureState=await Call("capture.snapshot");if(!captureState.GetProperty("prepared").GetBoolean())await Task.Delay(20,deadline.Token);}while(!captureState.GetProperty("prepared").GetBoolean());
            capture=Process.GetProcessById(captureState.GetProperty("processId").GetInt32());_=capture.Handle;
            var allReadyMs=clock.Elapsed.TotalMilliseconds;
            using var image=new Bitmap(260,65);
            using(var graphics=Graphics.FromImage(image)){graphics.Clear(Color.White);using var font=new Font("Segoe UI",24,GraphicsUnit.Pixel);graphics.DrawString("Hello 123",font,Brushes.Black,new PointF(10,8));}
            var client=new NativeHostConfiguration(pipe,host);var samples=new List<object>();
            async Task Recognize(string phase){var watch=Stopwatch.StartNew();var text=await client.RecognizeAsync(image,deadline.Token);if(text!="Hello 123")throw new InvalidOperationException("Unexpected generated OCR text: "+text);samples.Add(new {phase,milliseconds=watch.Elapsed.TotalMilliseconds});}
            await Recognize("first-after-startup-preparation");
            for(var i=0;i<4;i++)await Recognize("warm");
            await Task.Delay(500,deadline.Token);
            host.Refresh();ocr.Refresh();capture.Refresh();var cpu=host.TotalProcessorTime+ocr.TotalProcessorTime+capture.TotalProcessorTime;
            File.WriteAllText(Path.Combine(output,"idle-started.json"),JsonSerializer.Serialize(new {startedUtc=DateTime.UtcNow,idleSeconds,host=host.Id,capture=capture.Id,ocr=ocr.Id}));
            var idle=Stopwatch.StartNew();await Task.Delay(TimeSpan.FromSeconds(idleSeconds),deadline.Token);idle.Stop();
            host.Refresh();ocr.Refresh();capture.Refresh();var idleCpu=(host.TotalProcessorTime+ocr.TotalProcessorTime+capture.TotalProcessorTime-cpu).TotalSeconds;
            var idleResources=new {host=Resources(host),capture=Resources(capture),ocr=Resources(ocr)};
            await Recognize("after-idle");
            var status=await Call("ai.ocr.status");if(status.GetProperty("processId").GetInt32()!=ocr.Id)throw new InvalidOperationException("Idle OCR restarted");
            var configuration=JsonNode.Parse((await Call("ai.configuration.get")).GetRawText())!.AsObject();configuration["ocrEngine"]="ai";
            var saved=await Call("ai.configuration.save",new {configuration});
            if(!ocr.WaitForExit(5000))throw new InvalidOperationException("Selecting AI OCR retained the local worker");
            configuration=JsonNode.Parse(saved.GetRawText())!.AsObject();configuration["ocrEngine"]="local";
            await Call("ai.configuration.save",new {configuration});restarted=await WaitOCR();
            await Recognize("after-local-engine-reenabled");
            File.WriteAllText(Path.Combine(output,"host-latency.json"),JsonSerializer.Serialize(new {privateDesktop,ocrReadyMs,allReadyMs,idleSeconds=idle.Elapsed.TotalSeconds,idleCpuSeconds=idleCpu,idleResources,samples,engineSwitchReleased=true,allChildrenExited=false},new JsonSerializerOptions{WriteIndented=true}));
            var shut=Stopwatch.StartNew();
            if(!TrayExitProbe.RequestExit(host.Id)||!host.WaitForExit(8000)||host.ExitCode!=0)throw new InvalidOperationException("Host did not exit normally");
            if(!capture.WaitForExit(5000)||!restarted.WaitForExit(5000))throw new InvalidOperationException("OCR/capture survived Host exit");
            File.WriteAllText(Path.Combine(output,"host-latency.json"),JsonSerializer.Serialize(new {scope="Actual Host/capture/Python group; generated text through the production PNG mapping and job bridge; no clipboard, external models or real suspend",privateDesktop,ocrReadyMs,allReadyMs,idleSeconds=idle.Elapsed.TotalSeconds,idleCpuSeconds=idleCpu,idleResources,samples,engineSwitchReleased=true,normalExitMs=shut.Elapsed.TotalMilliseconds,allChildrenExited=true},new JsonSerializerOptions{WriteIndented=true}));
            return 0;
        }catch(Exception error){File.WriteAllText(Path.Combine(output,"failure.txt"),error.ToString());return 1;}
        finally{
            if(!host.HasExited){TrayExitProbe.RequestExit(host.Id);if(!host.WaitForExit(3000)){host.Kill();host.WaitForExit();}}
            foreach(var process in new[]{ocr,capture,restarted})process?.Dispose();
            File.WriteAllText(Path.Combine(output,"host-stderr.txt"),await diagnostics);
        }
    }
}
