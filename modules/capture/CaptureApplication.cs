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
    // Reuse immutable serializer metadata on the hot editing path. Copying
    // JsonSerializerOptions per request does not copy its metadata cache.
    private static readonly JsonSerializerOptions AnnotationJson = new(SettingsStore.Json)
    { UnmappedMemberHandling = JsonUnmappedMemberHandling.Disallow };
    private static readonly JsonSerializerOptions WireJson = new(SettingsStore.Json) { WriteIndented = false };
    private readonly Form dispatcher = new() { ShowInTaskbar = false };
    private readonly CancellationTokenSource stopping = new();
    private readonly object lifecycle = new();
    private readonly SettingsStore store;
    private readonly CaptureOperations operations;
    private readonly NativeCaptureHandlers? native;
    private readonly ConcurrentDictionary<string, Job> jobs = new();
    private readonly ConcurrentQueue<string> completed = new();
    private readonly System.Windows.Forms.Timer idle = new() { Interval = 5000 };
    private long activity = Environment.TickCount64;
    private int busy;
    private int closing;
    private int stopRequested;
    private int disposed;
    private int exitCode;
    private string? activeNativeJob;
    private (string Draft, long Revision, string Job)? retake;
    public int ExitCode => Volatile.Read(ref exitCode);
    public CaptureApplication(string pipe, string directory, NativeCaptureHandlers? native = null, Action<Bitmap>? copyImage = null, NativeUI.NativeHostConfiguration? nativeConfiguration = null)
    {
        Directory.CreateDirectory(directory);
        store = new(directory); operations = new(store, directory, copyImage) { NativeConfiguration = nativeConfiguration }; this.native = native;
        if (nativeConfiguration is not null) nativeConfiguration.BecameIdle += ConfigurationBecameIdle;
        _ = dispatcher.Handle;
        idle.Tick += (_, _) =>
        {
            lock (lifecycle)
            {
                if (busy != 0 || closing != 0 || PinToScreenForm.ActiveCount != 0 || Environment.TickCount64 - activity <= 60_000) return;
                if (operations.NativeConfiguration?.TryBeginShutdown() == false) return;
                closing = 1;
            }
            RequestStop();
        };
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
                // Status polling is not user activity and must not keep an idle worker alive.
                if (request.Method is not ("snapshot" or "jobs.get" or "settings.reload")) Interlocked.Exchange(ref activity, Environment.TickCount64);
                Response response;
                try { response = new(request.Id, Handle(request)); }
                catch (Exception error) { response = new(request.Id, Error: new("capture", error.Message)); }
                await PipeProtocol.WriteAsync(pipe, JsonSerializer.SerializeToUtf8Bytes(response, WireJson), timeout.Token).ConfigureAwait(false);
            }
            catch (OperationCanceledException) { }
            catch (IOException) { }
            catch (JsonException error) { Console.Error.WriteLine("Invalid capture request: " + error.Message); }
        }
    }
    private object Handle(Request request)
    {
        if (request.Version != 1 || string.IsNullOrEmpty(request.Id) || request.Id.Length > 128) throw new InvalidDataException("无效的通信版本或请求标识");
        string? expectedId = request.Parameters.ValueKind == JsonValueKind.Object && request.Parameters.TryGetProperty("expectedId", out var identity) ? identity.GetString() : null;
        var language = request.Parameters.ValueKind == JsonValueKind.Object && request.Parameters.TryGetProperty("language", out var requestedLanguage) ? requestedLanguage.GetString() : null;
        if (language is not (null or "zh-CN" or "en-US")) throw new InvalidDataException("不支持的界面语言");
        switch (request.Method)
        {
            case "snapshot": return new { processId = Environment.ProcessId, configurationManaged = store.IsManaged, settings = store.Snapshot(), lastCapture = operations.Last, busy = Volatile.Read(ref busy) != 0 || operations.NativeConfiguration?.IsSaving == true, configurationBusy = operations.NativeConfiguration?.IsSaving == true, pins = PinToScreenForm.ActiveCount };
            case "settings.reload": return store.ReloadManaged();
            case "capture.activate":
                lock (lifecycle) { return new { active = ActivateCurrentNative() is not null }; }
            case "settings.save": return store.Save(request.Parameters.Deserialize<CaptureSettings>(SettingsStore.Json) ?? throw new InvalidDataException("设置为空"));
            case "capture.start":
            case "capture.unified":
                var mode = request.Parameters.GetProperty("mode").GetString() ?? "";
                if (mode is not ("region" or "region-native" or "window" or "fullscreen" or "scroll" or "ocr" or "translate")) throw new InvalidDataException("不支持的截图方式");
                if (mode == "scroll" && request.Method == "capture.unified")
                    return native?.Scrolling is { } scrolling
                        ? StartNativeJobAsync(id => scrolling(store, operations, id, language, stopping.Token))
                        : StartJobAsync(id => operations.CaptureScrollingAsync(id, stopping.Token, language));
                if (request.Method == "capture.unified" && native?.Capture is { } capture)
                    return StartNativeJob(() => {
                        return capture(store, operations, mode == "region-native" ? "region" : mode, language, stopping.Token);
                    });
                return StartJob(() => operations.Capture(mode, stopping.Token, request.Method == "capture.unified", language));
            case "scroll.stop":
                return new { stopping = operations.StopScrolling(request.Parameters.GetProperty("id").GetString() ?? "") };
            case "handoff.release":
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少截图草稿标识");
                dispatcher.BeginInvoke(() => operations.ReleaseCaptureHold(expectedId));
                return new { accepted = true };
            case "scroll.retake":
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少截图草稿标识");
                return StartScrollRetake(expectedId, request.Parameters.GetProperty("revision").GetInt64(), language);
            case "draft.get": return new { draft = operations.GetDraft(expectedId) };
            case "draft.resume":
                if (expectedId is null || !Guid.TryParseExact(expectedId, "N", out var draftGuid) || draftGuid.ToString("N") != expectedId)
                    throw new InvalidDataException("截图草稿标识无效");
                if (native?.Resume is not { } resume || native.DraftMatches is not { } matches) throw new InvalidOperationException("原生草稿恢复不可用");
                // Validate before activation: a stale launch must not focus or
                // replace an unrelated capture already being edited.
                if (!matches(operations,expectedId)) throw new InvalidOperationException("截图草稿已变化，请重新加载");
                return StartNativeJob(() => resume(store, operations, expectedId, language, stopping.Token));
            case "draft.confirm":
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少截图草稿标识");
                var confirmRevision = request.Parameters.GetProperty("revision").GetInt64();
                return StartJob(() => operations.ConfirmDraft(expectedId, confirmRevision));
            case "selection.candidates":
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少截图草稿标识");
                int? selectionX = request.Parameters.TryGetProperty("x", out var sx) ? sx.GetInt32() : null;
                int? selectionY = request.Parameters.TryGetProperty("y", out var sy) ? sy.GetInt32() : null;
                return StartJob(() => operations.ReadSelectionCandidates(expectedId, selectionX, selectionY));
            case "selection.commit":
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少截图草稿标识");
                var selectionRevision = request.Parameters.GetProperty("revision").GetInt64();
                var selected = request.Parameters.GetProperty("crop").Deserialize<AnnotationCrop>(AnnotationJson) ?? throw new InvalidDataException("截图选区无效");
                return StartJob(() => operations.SelectDraft(expectedId, selectionRevision, selected));
            case "draft.begin":
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少待编辑图像标识");
                return StartJob(() => operations.PrepareLastDraft(expectedId));
            case "draft.update":
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少截图草稿标识");
                var draftRevision = request.Parameters.GetProperty("revision").GetInt64();
                var draftDocument = request.Parameters.GetProperty("document").Deserialize<AnnotationDocument>(AnnotationJson) ?? throw new InvalidDataException("标注为空");
                return StartJob(() =>
                {
                    var updated = operations.UpdateDraft(expectedId, draftRevision, draftDocument);
                    // Completed jobs retain only the acknowledgement, not up
                    // to 64 copies of a potentially large annotation scene.
                    return new { draftId = updated.Id, revision = updated.Revision };
                });
            case "draft.cancel":
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少截图草稿标识");
                var cancelRevision = request.Parameters.GetProperty("revision").GetInt64();
                return StartJob(() => operations.CancelDraft(expectedId, cancelRevision));
            case "draft.complete":
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少截图草稿标识");
                var completeRevision = request.Parameters.GetProperty("revision").GetInt64();
                var completePath = request.Parameters.TryGetProperty("path", out var requestedPath) ? requestedPath.GetString() : null;
                var saveCopy = request.Parameters.TryGetProperty("saveCopy", out var saveCopyValue) && saveCopyValue.GetBoolean();
                return StartJob(() => operations.CompleteDraft(expectedId, completeRevision, completePath, stopping.Token, language, saveCopy));
            case "draft.copy":
            case "draft.pin":
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少截图草稿标识");
                var actionRevision = request.Parameters.GetProperty("revision").GetInt64();
                return StartJob(() => request.Method == "draft.copy" ? operations.CopyDraft(expectedId, actionRevision) : operations.PinDraft(expectedId, actionRevision, language));
            case "edit.last":
                if (native?.EditLast is { } editLast) {
                    if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少待编辑图像标识");
                    if (operations.Last?.Id != expectedId) throw new InvalidOperationException("截图已变化，请刷新后重试");
                    return StartNativeJob(() => editLast(store, operations, expectedId, language, stopping.Token));
                }
                return StartJob(() => operations.EditLast(stopping.Token, expectedId));
            case "copy.last": return StartJob(() => operations.CopyLast(expectedId));
            case "save.last":
                var destination = request.Parameters.GetProperty("path").GetString() ?? throw new InvalidDataException("保存位置为空");
                return StartJob(() => operations.SaveLastAs(destination, expectedId));
            case "edit.preview":
            case "edit.apply":
                var document = request.Parameters.GetProperty("document").Deserialize<AnnotationDocument>(AnnotationJson) ?? throw new InvalidDataException("标注为空");
                if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少待编辑图像标识");
                if (request.Method == "edit.preview") return StartJob(() => operations.PreviewAnnotations(expectedId, document));
                var editPath = request.Parameters.GetProperty("path").GetString() ?? throw new InvalidDataException("保存位置为空");
                return StartJob(() => operations.ApplyAnnotations(editPath, expectedId, document));
            case "pin.last": return StartJob(() => operations.PinLast(expectedId, language));
            case "pins.close": return StartJob(() => { PinToScreenForm.CloseAll(); return new { closed = true }; });
            case "jobs.get":
                var id = request.Parameters.GetProperty("id").GetString() ?? "";
                return jobs.TryGetValue(id, out var job) ? job : throw new InvalidOperationException("任务不存在或已过期");
            case "shutdown":
                lock (lifecycle)
                {
                    if (busy != 0) throw new InvalidOperationException("请先完成或取消当前截图操作");
                    if (operations.NativeConfiguration?.TryBeginShutdown() == false) throw new InvalidOperationException("配置正在保存，请等待完成");
                    closing = 1;
                }
                _ = Task.Delay(150).ContinueWith(_ => RequestStop(), TaskScheduler.Default);
                return new { stopping = true };
            default: throw new InvalidDataException("不支持的截图操作");
        }
    }
    private Job StartScrollRetake(string id, long revision, string? language)
    {
        lock (lifecycle)
        {
            // Duplicate requests reuse the active job. After a restart the
            // committed draft's lineage recovers a response lost in transit.
            if (retake is { } pending && pending.Draft == id && pending.Revision == revision && jobs.TryGetValue(pending.Job, out var running) && running.State == "running") return running;
            var current = operations.GetDraft();
            if (current?.Replaces is { } replaced && replaced.Id == id && replaced.Revision == revision)
                return new Job(Guid.NewGuid().ToString("N"), "completed", new CaptureEditorRequest(current.Id));
            var job = StartJobAsync(jobId => operations.CaptureScrollingAsync(jobId, stopping.Token, language, replaceId: id, replaceRevision: revision));
            retake = (id, revision, job.Id);
            return job;
        }
    }
    // All accesses to the active job run under lifecycle. Activation captures
    // its identity so a queued request cannot focus a later, unrelated session.
    private Job? ActivateCurrentNative()
    {
        if (closing != 0 || activeNativeJob is not { } id || !jobs.TryGetValue(id, out var job) || job.State != "running") return null;
        dispatcher.BeginInvoke((Action)(() => {
            lock (lifecycle) { if (activeNativeJob != id) return; }
            operations.ActivateNativeCapture?.Invoke();
        }));
        return job;
    }
    private Job StartNativeJob(Func<Task<object>> action)
    {
        return StartNativeJobAsync(_ => action());
    }
    private Job StartNativeJobAsync(Func<string, Task<object>> action)
    {
        lock (lifecycle) { return ActivateCurrentNative() ?? StartJobAsync(action, native: true); }
    }
    private Job StartJob(Func<object> action) => StartJobAsync(_ => Task.FromResult(action()));
    private Job StartJobAsync(Func<string, Task<object>> action, bool native = false)
    {
        var job = new Job(Guid.NewGuid().ToString("N"), "running");
        lock (lifecycle)
        {
            if (closing != 0) throw new InvalidOperationException("截图模块正在退出");
            if (busy != 0) throw new InvalidOperationException("请先完成当前截图操作");
            busy = 1;
            jobs[job.Id] = job;
            if (native) activeNativeJob = job.Id;
        }
        try
        {
            dispatcher.BeginInvoke(async () =>
            {
                var finished = job;
                try
                {
                    stopping.Token.ThrowIfCancellationRequested();
                    using var interactive = new InteractiveWorkScope();
                    if (native) operations.NativeJobId = job.Id;
                    finished = job with { State = "completed", Result = await action(job.Id) };
                }
                catch (OperationCanceledException) { finished = job with { State = "cancelled" }; }
                catch (Exception error) { finished = job with { State = "failed", Error = error.Message }; Console.Error.WriteLine(error); }
                finally
                {
                    // A terminal job and its released busy slot become visible before shutdown can pass.
                    lock (lifecycle)
                    {
                        activity = Environment.TickCount64; busy = 0; jobs[job.Id] = finished;
                        if (operations.NativeJobId == job.Id) operations.NativeJobId = null;
                        if (activeNativeJob == job.Id) activeNativeJob = null;
                    }
                    completed.Enqueue(job.Id);
                    while (jobs.Count > 64 && completed.TryDequeue(out var old)) jobs.TryRemove(old, out _);
                    if (Volatile.Read(ref closing) != 0) RequestStop();
                }
            });
        }
        catch {
            lock (lifecycle) { busy = 0; if (activeNativeJob == job.Id) activeNativeJob = null; jobs.TryRemove(job.Id, out _); }
            throw;
        }
        return job;
    }
    private void ConfigurationBecameIdle()
    {
        Interlocked.Exchange(ref activity, Environment.TickCount64);
        if (Volatile.Read(ref closing) != 0 && Volatile.Read(ref busy) == 0) RequestStop();
    }
    public void RequestStop(bool preserveActive = false)
    {
        lock (lifecycle)
        {
            closing = 1;
            operations.NativeConfiguration?.StopAccepting();
            // A dead Host must not discard edits. Stop accepting work and let
            // the active native operation finish before cancelling the worker.
            if (preserveActive && (busy != 0 || operations.NativeConfiguration?.IsSaving == true)) return;
        }
        if (Interlocked.Exchange(ref stopRequested, 1) != 0) return;
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
        if (disposing && Interlocked.Exchange(ref disposed, 1) == 0) {
            if (operations.NativeConfiguration is { } configuration) { configuration.BecameIdle -= ConfigurationBecameIdle; configuration.StopAccepting(); }
            stopping.Cancel(); idle.Dispose(); dispatcher.Dispose(); stopping.Dispose();
        }
        base.Dispose(disposing);
    }
}
