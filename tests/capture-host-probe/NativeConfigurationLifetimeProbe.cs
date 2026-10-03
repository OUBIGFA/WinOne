using System.Diagnostics;
using System.IO.Pipes;
using System.Text.Json;
using System.Text.Json.Nodes;
using WinOne.Capture;

internal static class NativeConfigurationLifetimeProbe
{
    public static int Run(string output) { RunAsync(output).GetAwaiter().GetResult(); return 0; }
    private static async Task RunAsync(string output)
    {
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output);
        using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(10));
        var hostPipe = "WinOneConfigOwner-" + Guid.NewGuid().ToString("N");
        var workerPipe = "WinOneConfigWorker-" + Guid.NewGuid().ToString("N");
        var entered = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        var release = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        var host = Task.Run(async () => {
            for (var i = 0; i < 2; i++) {
                using var pipe = new NamedPipeServerStream(hostPipe, PipeDirection.InOut, 1, PipeTransmissionMode.Byte, PipeOptions.Asynchronous | PipeOptions.CurrentUserOnly);
                await pipe.WaitForConnectionAsync(timeout.Token);
                using var request = JsonDocument.Parse(await PipeProtocol.ReadAsync(pipe, timeout.Token));
                var value = request.RootElement; object result;
                if (i == 0) result = new { revision = 1, toolbarTools = new[] { "text" }, outputDirectory = output };
                else {
                    entered.SetResult(); await release.Task.WaitAsync(timeout.Token);
                    var saved = JsonNode.Parse(value.GetProperty("params").GetRawText())!.AsObject(); saved["revision"] = 2; result = saved;
                }
                await PipeProtocol.WriteAsync(pipe, JsonSerializer.SerializeToUtf8Bytes(new { id = value.GetProperty("id").GetString(), result }), timeout.Token);
            }
        });
        using var parent = Process.GetCurrentProcess();
        var configuration = new NativeHostConfiguration(hostPipe, parent);
        var ready = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        Exception? workerFailure = null; CaptureApplication? application = null;
        var thread = new Thread(() => {
            try {
                Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
                using var app = new CaptureApplication(workerPipe, Path.Combine(output, "data"), nativeConfiguration: configuration);
                application = app; ready.SetResult(); Application.Run(app);
                if (app.ExitCode != 0) throw new Exception("Configuration worker exited with an error");
            } catch (Exception error) { workerFailure = error; ready.TrySetException(error); }
        });
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        async Task<JsonElement> Call(string method) {
            using var pipe = new NamedPipeClientStream(".", workerPipe, PipeDirection.InOut, PipeOptions.Asynchronous);
            await pipe.ConnectAsync(timeout.Token);
            var id = Guid.NewGuid().ToString("N");
            await PipeProtocol.WriteAsync(pipe, JsonSerializer.SerializeToUtf8Bytes(new { version = 1, id, method, @params = new { } }), timeout.Token);
            using var response = JsonDocument.Parse(await PipeProtocol.ReadAsync(pipe, timeout.Token)); return response.RootElement.Clone();
        }
        try {
            await ready.Task.WaitAsync(timeout.Token);
            var pending = configuration.SaveToolbarAsync(new[] { "arrow" }, timeout.Token);
            await entered.Task.WaitAsync(timeout.Token);
            var busy = (await Call("snapshot")).GetProperty("result");
            if (!busy.GetProperty("busy").GetBoolean() || !busy.GetProperty("configurationBusy").GetBoolean()) throw new Exception("Worker hid an active configuration save");
            var refused = await Call("shutdown");
            if (!refused.TryGetProperty("error", out var error) || error.ValueKind == JsonValueKind.Null) throw new Exception("Worker shut down during a configuration save");
            release.SetResult(); await pending; await host;
            var idle = (await Call("snapshot")).GetProperty("result");
            if (idle.GetProperty("busy").GetBoolean() || idle.GetProperty("configurationBusy").GetBoolean()) throw new Exception("Worker kept the completed save active");
            var stopped = await Call("shutdown");
            if (!stopped.GetProperty("result").GetProperty("stopping").GetBoolean()) throw new Exception("Worker did not accept shutdown after saving");
            if (!thread.Join(TimeSpan.FromSeconds(3))) throw new Exception("Worker did not exit after saving");
            if (workerFailure is not null) throw workerFailure;
            File.WriteAllText(Path.Combine(output, "report.json"), JsonSerializer.Serialize(new { busyDuringSave = true, shutdownBlocked = true, idleAfterSave = true, exited = true }));
            Console.WriteLine("PASS: real worker retains pending configuration, rejects early shutdown and exits after save");
        } finally {
            release.TrySetResult(); timeout.Cancel();
            if (thread.IsAlive) { application?.RequestStop(); thread.Join(TimeSpan.FromSeconds(3)); }
        }
    }
}
