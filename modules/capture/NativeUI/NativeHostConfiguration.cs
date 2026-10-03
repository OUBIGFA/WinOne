using System.Diagnostics;
using System.IO.Pipes;
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
        var allowed = new HashSet<string>(new[] { "select", "crop", "rectangle", "ellipse", "arrow", "pen", "cover", "text", "mosaic", "blur" }, StringComparer.Ordinal);
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

    private async Task<JsonElement> Call(string method, object parameters, string id, CancellationToken cancellation)
    {
        if (parent.HasExited) throw new InvalidOperationException("WinOne 已退出");
        using var deadline = CancellationTokenSource.CreateLinkedTokenSource(cancellation);
        deadline.CancelAfter(TimeSpan.FromSeconds(5));
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
    private static bool HasError(JsonElement response) => response.TryGetProperty("error", out var error) && error.ValueKind != JsonValueKind.Null;
    private static JsonElement Result(JsonElement response)
    {
        if (HasError(response)) throw new InvalidOperationException(response.GetProperty("error").GetProperty("message").GetString() ?? "配置保存失败");
        return response.GetProperty("result").Clone();
    }
}
