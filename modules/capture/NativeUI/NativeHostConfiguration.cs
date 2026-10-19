using System.Diagnostics;
using System.IO.Pipes;
using System.IO.MemoryMappedFiles;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Nodes;
using Microsoft.Win32.SafeHandles;

namespace WinOne.Capture.NativeUI;

// Native module UI uses the same Host transaction as the settings shell.
// A save is sent once; only its read-only configuration job is polled.
internal sealed class NativeHostConfiguration(string pipeName, Process parent)
{
    private readonly string name = pipeName.StartsWith(@"\\.\pipe\", StringComparison.OrdinalIgnoreCase) ? pipeName[9..] : pipeName;
    private readonly object lifecycle = new();
    private int saving;
    private bool closing;
    public bool IsSaving => Volatile.Read(ref saving) != 0;
    public event Action? BecameIdle;

    public bool TryBeginShutdown()
    {
        lock (lifecycle) {
            if (saving != 0) return false;
            closing = true; return true;
        }
    }
    public void StopAccepting() { lock (lifecycle) closing = true; }
    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool GetNamedPipeServerProcessId(SafePipeHandle pipe, out uint processId);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window,out uint processId);
    [DllImport("user32.dll")] private static extern bool SetForegroundWindow(IntPtr window);

    internal static void VerifyServer(NamedPipeClientStream pipe, int expectedParent)
    {
        if (!GetNamedPipeServerProcessId(pipe.SafePipeHandle, out var actual))
            throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
        if (expectedParent <= 0 || actual != (uint)expectedParent) throw new InvalidOperationException("配置服务身份不匹配");
    }

    public async Task<string[]?> SaveToolbarAsync(string[]? tools, CancellationToken cancellation)
    {
        lock (lifecycle) {
            if (closing) throw new InvalidOperationException("WinOne 正在退出");
            if (saving != 0) throw new InvalidOperationException("配置正在保存，请等待完成");
            saving = 1;
        }
        try { return await SaveToolbarCoreAsync(tools, cancellation).ConfigureAwait(false); }
        finally {
            lock (lifecycle) saving = 0;
            BecameIdle?.Invoke();
        }
    }

    private async Task<string[]?> SaveToolbarCoreAsync(string[]? tools, CancellationToken cancellation)
    {
        var allowed = new HashSet<string>(new[] { "select", "crop", "rectangle", "ellipse", "arrow", "pen", "cover", "text", "mosaic", "blur", "ocr", "translate" }, StringComparer.Ordinal);
        if (tools is not null && tools.Any(tool => !allowed.Remove(tool))) throw new ArgumentException("无效的工具栏配置", nameof(tools));
        var requested = tools?.ToArray();
        using var deadline = CancellationTokenSource.CreateLinkedTokenSource(cancellation);
        deadline.CancelAfter(TimeSpan.FromSeconds(30));
        var token = deadline.Token;
        var current = await Call("capture.settings.get", new { }, Guid.NewGuid().ToString("N"), token).ConfigureAwait(false);
        var settings = JsonNode.Parse(current.GetRawText()) as JsonObject ?? throw new InvalidDataException("配置结果无效");
        if (settings["revision"] is not JsonValue revision || !revision.TryGetValue<long>(out var expected) || expected < 0)
            throw new InvalidDataException("配置修订号无效");
        settings["toolbarTools"] = JsonSerializer.SerializeToNode(requested);
        var writeId = Guid.NewGuid().ToString("N");
        var result = await Call("capture.settings.save", settings, writeId, token).ConfigureAwait(false);
        if (result.TryGetProperty("configurationJob", out var ticket)) {
            var jobId = ticket.GetString();
            if (string.IsNullOrEmpty(jobId)) throw new InvalidDataException("配置任务标识无效");
            while (true) {
                var job = await Call("configuration.jobs.get", new { id = jobId }, Guid.NewGuid().ToString("N"), token).ConfigureAwait(false);
                if (job.GetProperty("id").GetString() != jobId) throw new InvalidDataException("配置任务标识不匹配");
                var state = job.GetProperty("state").GetString();
                if (state is "completed" or "failed") {
                    var response = job.GetProperty("response");
                    if (response.GetProperty("id").GetString() != writeId || (state == "failed") != HasError(response))
                        throw new InvalidDataException("配置任务结果不一致");
                    result = Result(response); break;
                }
                if (state != "running") throw new InvalidDataException("无法确认配置任务状态");
                await Task.Delay(150, token).ConfigureAwait(false);
            }
        }
        if (!result.TryGetProperty("revision", out var savedRevision) || !savedRevision.TryGetInt64(out var saved) || saved <= expected)
            throw new InvalidDataException("无法确认配置已保存");
        if (!result.TryGetProperty("toolbarTools", out var value)) throw new InvalidDataException("工具栏保存结果缺失");
        var confirmed = value.ValueKind != JsonValueKind.Null
            ? value.Deserialize<string[]>() : null;
        if ((requested is null) != (confirmed is null) || (requested is not null && !requested.SequenceEqual(confirmed!)))
            throw new InvalidDataException("工具栏保存结果不一致");
        return confirmed;
    }

    private async Task<JsonElement> Call(string method, object parameters, string id, CancellationToken cancellation, int timeoutSeconds = 5)
    {
        if (parent.HasExited) throw new InvalidOperationException("WinOne 已退出");
        using var deadline = CancellationTokenSource.CreateLinkedTokenSource(cancellation);
        deadline.CancelAfter(TimeSpan.FromSeconds(timeoutSeconds));
        using var pipe = new NamedPipeClientStream(".", name, PipeDirection.InOut, PipeOptions.Asynchronous);
        await pipe.ConnectAsync(deadline.Token).ConfigureAwait(false);
        VerifyServer(pipe, parent.Id);
        if (parent.HasExited) throw new InvalidOperationException("WinOne 已退出");
        await PipeProtocol.WriteAsync(pipe, JsonSerializer.SerializeToUtf8Bytes(new { version = 1, id, method, @params = parameters }), deadline.Token).ConfigureAwait(false);
        using var document = JsonDocument.Parse(await PipeProtocol.ReadAsync(pipe, deadline.Token).ConfigureAwait(false), new JsonDocumentOptions { MaxDepth = 32 });
        var response = document.RootElement;
        if (response.GetProperty("id").GetString() != id) throw new InvalidDataException("配置响应标识不匹配");
        return Result(response);
    }

    internal async Task<string> TranslateAsync(string text, CancellationToken cancellation)
    {
        var started = await Call("ai.translate.start", new { text }, Guid.NewGuid().ToString("N"), cancellation).ConfigureAwait(false);
        return await CompleteAIJobAsync(started, cancellation).ConfigureAwait(false);
    }

    internal async Task PrepareOCRAsync(CancellationToken cancellation)
    {
        // The Host checks module enablement and the saved OCR engine again.
        // This only queues generated-image preparation; it starts no AI call.
        await Call("ai.ocr.prepare", new { }, Guid.NewGuid().ToString("N"), cancellation).ConfigureAwait(false);
    }

    internal async Task<bool> UsesAIRecognitionAsync(CancellationToken cancellation)
    {
        var settings = await Call("ai.settings.get", new { }, Guid.NewGuid().ToString("N"), cancellation).ConfigureAwait(false);
        var mode = settings.TryGetProperty("ocrEngine", out var value) ? value.GetString() : "local";
        if (mode is null or "" or "local") return false;
        if (mode == "ai") return true;
        throw new InvalidDataException("AI OCR 设置无效");
    }

    internal async Task<string> RecognizeAsync(Bitmap image, CancellationToken cancellation)
    {
        using var png = new MemoryStream();
        image.Save(png, ImageFormat.Png);
        if (png.Length > 32 * 1024 * 1024) throw new InvalidOperationException("AI 图片过大，请缩小选区");
        var mappingName = $@"Local\WinOne.AI.{Environment.ProcessId}.{Guid.NewGuid():N}";
        JsonElement started;
        // Transfer one PNG snapshot without writing selected pixels to disk or
        // repeating Base64 in the settings/job pipe. The Host copies before ACK.
        using (var mapping = MemoryMappedFile.CreateNew(mappingName, png.Length, MemoryMappedFileAccess.ReadWrite)) {
            using (var view = mapping.CreateViewStream()) {
                png.Position = 0;
                await png.CopyToAsync(view, cancellation).ConfigureAwait(false);
            }
            started = await Call("ai.recognize.start", new { image = new { mapping = mappingName, bytes = png.Length } }, Guid.NewGuid().ToString("N"), cancellation).ConfigureAwait(false);
        }
        return await CompleteAIJobAsync(started, cancellation).ConfigureAwait(false);
    }

    internal async Task<string> OpenResultAsync(Bitmap image,bool translate,CancellationToken cancellation)
    {
        using var png=new MemoryStream();image.Save(png,ImageFormat.Png);
        if(png.Length>32*1024*1024)throw new InvalidOperationException("AI 图片过大，请缩小选区");
        var id=Guid.NewGuid().ToString("N");var name=$@"Local\WinOne.AI.{Environment.ProcessId}.{id}";
        var accepted=false;
        try {
            using var mapping=MemoryMappedFile.CreateNew(name,png.Length,MemoryMappedFileAccess.ReadWrite);
            using(var view=mapping.CreateViewStream()){png.Position=0;await png.CopyToAsync(view,cancellation).ConfigureAwait(false);}
            var reply=await Call("results.open",new{id,mode=translate?"translate":"ocr",image=new{mapping=name,bytes=png.Length}},id,cancellation,12).ConfigureAwait(false);
            if(!reply.GetProperty("opened").GetBoolean())throw new InvalidOperationException("resultUnavailable");
            var window=new IntPtr(long.Parse(reply.GetProperty("window").GetString()!,System.Globalization.CultureInfo.InvariantCulture));
            GetWindowThreadProcessId(window,out var owner);
            if(owner!=reply.GetProperty("processId").GetUInt32()||!SetForegroundWindow(window))throw new InvalidOperationException("resultUnavailable");
            accepted=true;
            return id;
        } finally {
            // A cancelled/failed opening must not create a late result window.
            if(!accepted){try{await Call("results.cancel",new{id},Guid.NewGuid().ToString("N"),CancellationToken.None).ConfigureAwait(false);}catch(Exception error){Console.Error.WriteLine("Result opening cancellation: "+error.GetType().Name);}}
        }
    }

    private async Task<string> CompleteAIJobAsync(JsonElement started, CancellationToken cancellation)
    {
        var jobId = started.GetProperty("id").GetString() ?? throw new InvalidDataException("翻译任务无效");
        try {
            while (true) {
                var job = await Call("ai.jobs.get", new { id = jobId, waitMs = 500 }, Guid.NewGuid().ToString("N"), cancellation).ConfigureAwait(false);
                var state = job.GetProperty("state").GetString();
                if (state == "completed") return job.GetProperty("text").GetString() ?? throw new InvalidDataException("翻译结果为空");
                if (state == "failed") throw new InvalidOperationException(job.GetProperty("error").GetString());
                if (state == "cancelled") throw new OperationCanceledException(cancellation);
                if (state != "running") throw new InvalidDataException("翻译任务状态无效");
                // The bounded Host wait returns immediately on completion.
                // Do not add a 100ms client polling floor to a 20ms OCR result.
            }
        } finally {
            // Releasing a result also removes captured text from the Host job
            // table; cancellation never lets an old result overwrite a new one.
            try { await Call("ai.jobs.cancel", new { id = jobId }, Guid.NewGuid().ToString("N"), CancellationToken.None).ConfigureAwait(false); }
            catch (Exception error) { Console.Error.WriteLine("Translation cleanup: " + error.GetType().Name); }
        }
    }
    private static bool HasError(JsonElement response) => response.TryGetProperty("error", out var error) && error.ValueKind != JsonValueKind.Null;
    private static JsonElement Result(JsonElement response)
    {
        if (HasError(response)) throw new InvalidOperationException(response.GetProperty("error").GetProperty("message").GetString() ?? "配置保存失败");
        return response.GetProperty("result").Clone();
    }
}
