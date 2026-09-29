using System.Collections.Concurrent;
using System.IO.Pipes;
using System.Text.Json;
using System.Text.Json.Serialization;
using ShareX;

namespace WinOne.Capture;

internal record ApiError(string Code, string Message);
internal record Response(string Id, object? Result = null, ApiError? Error = null);
internal record Request(int Version, string Id, string Method, [property: JsonPropertyName("params")] JsonElement Parameters);
internal record Job(string Id, string State, object? Result = null, string? Error = null);

// One STA owns native capture forms; pipe clients only enqueue bounded commands.
internal sealed class CaptureApplication : ApplicationContext
{
    private readonly Form dispatcher = new() { ShowInTaskbar = false };
    private readonly CancellationTokenSource stopping = new();
    private readonly SettingsStore store;
    private readonly CaptureOperations operations;
    private readonly ConcurrentDictionary<string, Job> jobs = new();
    private readonly ConcurrentQueue<string> completed = new();
    private readonly System.Windows.Forms.Timer idle = new() { Interval = 5000 };
    private long activity = Environment.TickCount64;
    private int busy;
    private int closing;
    private int disposed;
    private int exitCode;
    public int ExitCode => Volatile.Read(ref exitCode);
    public CaptureApplication(string pipe, string directory)
    {
        Directory.CreateDirectory(directory);
        store = new(directory); operations = new(store, directory);
        _ = dispatcher.Handle;
        idle.Tick += (_, _) => { if (Volatile.Read(ref busy) == 0 && PinToScreenForm.ActiveCount == 0 && Environment.TickCount64 - Interlocked.Read(ref activity) > 60_000) RequestStop(); };
        idle.Start();
        _ = Listen(pipe.StartsWith(@"\\.\pipe\", StringComparison.OrdinalIgnoreCase) ? pipe[9..] : pipe);
    }
    private async Task Listen(string name)
    {
        using var slots = new SemaphoreSlim(16);
        var first = true;
        try
        {
            while (!stopping.IsCancellationRequested)
            {
                await slots.WaitAsync(stopping.Token).ConfigureAwait(false);
                var pipe = LocalPipe.Create(name, first);
                first = false;
                try { await pipe.WaitForConnectionAsync(stopping.Token).ConfigureAwait(false); }
                catch { pipe.Dispose(); slots.Release(); throw; }
                _ = Serve(pipe).ContinueWith(_ => { try { slots.Release(); } catch (ObjectDisposedException) { } }, TaskScheduler.Default);
            }
        }
        catch (OperationCanceledException) { }
        catch (Exception error) { Interlocked.Exchange(ref exitCode, 1); Console.Error.WriteLine(error); RequestStop(); }
    }
    private async Task Serve(NamedPipeServerStream pipe)
    {
        using (pipe)
        using (var timeout = CancellationTokenSource.CreateLinkedTokenSource(stopping.Token))
        {
            timeout.CancelAfter(TimeSpan.FromSeconds(5));
            try
            {
                var bytes = await PipeProtocol.ReadAsync(pipe, timeout.Token).ConfigureAwait(false);
                var request = JsonSerializer.Deserialize<Request>(bytes, SettingsStore.Json) ?? throw new InvalidDataException("请求为空");
                Interlocked.Exchange(ref activity, Environment.TickCount64);
                Response response;
                try { response = new(request.Id, Handle(request)); }
                catch (Exception error) { response = new(request.Id, Error: new("capture", error.Message)); }
                await PipeProtocol.WriteAsync(pipe, JsonSerializer.SerializeToUtf8Bytes(response, SettingsStore.Json), timeout.Token).ConfigureAwait(false);
            }
            catch (OperationCanceledException) { }
            catch (IOException) { }
            catch (JsonException error) { Console.Error.WriteLine("Invalid capture request: " + error.Message); }
        }
    }
    private object Handle(Request request)
    {
        if (request.Version != 1 || string.IsNullOrEmpty(request.Id)) throw new InvalidDataException("无效的通信版本或请求标识");
        switch (request.Method)
        {
            case "snapshot": return new { settings = store.Snapshot(), lastCapture = operations.Last, busy = Volatile.Read(ref busy) != 0, pins = PinToScreenForm.ActiveCount };
            case "settings.save": return store.Save(request.Parameters.Deserialize<CaptureSettings>(SettingsStore.Json) ?? throw new InvalidDataException("设置为空"));
            case "capture.start":
                var mode = request.Parameters.GetProperty("mode").GetString() ?? "";
                if (mode is not ("region" or "window" or "fullscreen" or "scroll")) throw new InvalidDataException("不支持的截图方式");
                return StartJob(() => operations.Capture(mode, stopping.Token));
            case "edit.last": return StartJob(() => operations.EditLast(stopping.Token));
            case "copy.last": return StartJob(operations.CopyLast);
            case "pin.last": return StartJob(operations.PinLast);
            case "pins.close": return StartJob(() => { PinToScreenForm.CloseAll(); return new { closed = true }; });
            case "jobs.get":
                var id = request.Parameters.GetProperty("id").GetString() ?? "";
                return jobs.TryGetValue(id, out var job) ? job : throw new InvalidOperationException("任务不存在或已过期");
            case "shutdown":
                if (Volatile.Read(ref busy) != 0) throw new InvalidOperationException("请先完成或取消当前截图，再退出 WinOne");
                _ = Task.Delay(150).ContinueWith(_ => RequestStop(), TaskScheduler.Default);
                return new { stopping = true };
            default: throw new InvalidDataException("不支持的截图操作");
        }
    }
    private Job StartJob(Func<object> action)
    {
        if (Volatile.Read(ref closing) != 0) throw new InvalidOperationException("截图模块正在退出");
        if (Interlocked.CompareExchange(ref busy, 1, 0) != 0) throw new InvalidOperationException("请先完成当前截图操作");
        var job = new Job(Guid.NewGuid().ToString("N"), "running"); jobs[job.Id] = job;
        try
        {
            dispatcher.BeginInvoke(() =>
            {
                try { stopping.Token.ThrowIfCancellationRequested(); jobs[job.Id] = job with { State = "completed", Result = action() }; }
                catch (OperationCanceledException) { jobs[job.Id] = job with { State = "cancelled" }; }
                catch (Exception error) { jobs[job.Id] = job with { State = "failed", Error = error.Message }; Console.Error.WriteLine(error); }
                finally
                {
                    Interlocked.Exchange(ref busy, 0); Interlocked.Exchange(ref activity, Environment.TickCount64);
                    completed.Enqueue(job.Id);
                    while (jobs.Count > 64 && completed.TryDequeue(out var old)) jobs.TryRemove(old, out _);
                }
            });
        }
        catch { Interlocked.Exchange(ref busy, 0); jobs.TryRemove(job.Id, out _); throw; }
        return job;
    }
    public void RequestStop()
    {
        if (Interlocked.Exchange(ref closing, 1) != 0) return;
        stopping.Cancel();
        try
        {
            dispatcher.BeginInvoke(async () =>
            {
                idle.Stop();
                foreach (var form in Application.OpenForms.Cast<Form>().Where(f => f != dispatcher).ToArray()) form.Close();
                PinToScreenForm.CloseAll();
                var deadline = Environment.TickCount64 + 8000;
                while (Volatile.Read(ref busy) != 0 && Environment.TickCount64 < deadline) await Task.Delay(25);
                ExitThread();
            });
        }
        catch (InvalidOperationException) { ExitThread(); }
    }
    protected override void Dispose(bool disposing)
    {
        if (disposing && Interlocked.Exchange(ref disposed, 1) == 0) { stopping.Cancel(); idle.Dispose(); dispatcher.Dispose(); stopping.Dispose(); }
        base.Dispose(disposing);
    }
}
