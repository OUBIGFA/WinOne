using System.Diagnostics;
using System.IO.Pipes;
using System.Text.Json;
using WinOne.Capture;

// Regression: a hung worker UI thread once made every standby `prepare` wait
// forever, until all 16 pipe instances were held and the Host saw only
// "context deadline exceeded". The pipe must stay usable, each request must
// end within its budget, and the stall must be reported for replacement.
internal static class CaptureUiStallTests
{
    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        var pipeName = "WinOne-ui-stall-" + Guid.NewGuid().ToString("N");
        var ready = new ManualResetEventSlim();
        var release = new ManualResetEventSlim();
        var blocked = new ManualResetEventSlim();
        CaptureApplication? application = null; Control? marshal = null; Exception? failure = null;
        var thread = new Thread(() => {
            try {
                using var app = new CaptureApplication(pipeName, directory, uiStallLimit: TimeSpan.FromSeconds(1));
                using var control = new Control();
                _ = control.Handle;
                application = app; marshal = control; ready.Set();
                Application.Run(app);
            } catch (Exception error) { failure = error; ready.Set(); }
        }) { IsBackground = true };
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        try {
            if (!ready.Wait(10_000) || failure is not null) throw new Exception("UI stall worker did not start", failure);
            // Hang the worker's own UI thread, as an unresponsive input-method
            // window or native call did in production.
            marshal!.BeginInvoke(() => { blocked.Set(); release.Wait(); });
            if (!blocked.Wait(5_000)) throw new Exception("UI thread was not blocked");
            var clock = Stopwatch.StartNew();
            // More concurrent maintenance requests than pipe instances.
            var prepares = Enumerable.Range(0, 24).Select(_ => Task.Run(() => Call(pipeName, "prepare", new { refresh = true }))).ToArray();
            if (!Task.WaitAll(prepares, 12_000)) throw new Exception("prepare requests waited on a hung UI thread");
            if (clock.Elapsed > TimeSpan.FromSeconds(10)) throw new Exception($"prepare requests took {clock.Elapsed}");
            foreach (var prepare in prepares)
                if (prepare.Result.TryGetProperty("error", out var error) && error.ValueKind != JsonValueKind.Null && error.GetProperty("code").GetString() != "unresponsive")
                    throw new Exception("prepare failed unexpectedly: " + error);
            Thread.Sleep(1_200);
            var probe = Call(pipeName, "snapshot", new { });
            if (!probe.TryGetProperty("error", out var stalled) || stalled.ValueKind == JsonValueKind.Null || stalled.GetProperty("code").GetString() != "unresponsive")
                throw new Exception("hung UI thread was not reported: " + probe);
        } finally {
            application?.RequestStop();
            release.Set();
            if (!thread.Join(15_000)) throw new Exception("UI stall worker did not exit");
        }
        if (failure is not null) throw failure;
        Console.WriteLine("PASS: hung capture UI thread keeps the pipe available and is reported for restart");
    }

    private static JsonElement Call(string pipeName, string method, object parameters)
    {
        using var deadline = new CancellationTokenSource(TimeSpan.FromSeconds(8));
        using var pipe = new NamedPipeClientStream(".", pipeName, PipeDirection.InOut, PipeOptions.Asynchronous);
        pipe.ConnectAsync(deadline.Token).GetAwaiter().GetResult();
        var id = Guid.NewGuid().ToString("N");
        PipeProtocol.WriteAsync(pipe, JsonSerializer.SerializeToUtf8Bytes(new { version = 1, id, method, @params = parameters }), deadline.Token).GetAwaiter().GetResult();
        using var response = JsonDocument.Parse(PipeProtocol.ReadAsync(pipe, deadline.Token).GetAwaiter().GetResult());
        return response.RootElement.Clone();
    }
}
