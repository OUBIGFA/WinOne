using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;

namespace WinOne.Capture.NativeUI;

// Runtime-only contract: HWNDs and physical selection coordinates are never
// written to transferable preferences or the screenshot recovery document.
internal sealed record RecordingConfig(int X, int Y, int Width, int Height, string Monitor, string Output, string Format)
{
    public string Codec { get; init; } = "h264";
    public string Coordinates { get; init; } = "physicalDesktop";
    public double ScaleFactor { get; init; } = 1;
    public int Fps { get; init; } = 30;
    public int Quality { get; init; } = 85;
    public int GifMaxSeconds { get; init; } = 120;
    public ulong GifMaxFrames { get; init; } = 1800;
    public bool Cursor { get; init; } = true;
    public ulong[] ExcludeWindows { get; init; } = [];

    internal static RecordingConfig FromSelection(Rectangle selection, string monitor, string path, string format, double scaleFactor)
    {
        if (format is not ("mp4" or "gif")) throw new ArgumentException("不支持的录制格式");
        // This canvas is already per-monitor DPI aware and uses physical
        // pixels. Passing its DPI is metadata, not permission to scale twice.
        var width = selection.Width & ~1; var height = selection.Height & ~1;
        if (width < 16 || height < 16) throw new InvalidOperationException("录制选区不能小于 16×16 像素");
        return new(selection.X, selection.Y, width, height, monitor, path, format) {
            ScaleFactor = scaleFactor, Fps = format == "gif" ? 15 : 30,
        };
    }
}
internal sealed record RecordingStatus
{
    public string Phase { get; init; } = "idle";
    public string Format { get; init; } = "mp4";
    public int Width { get; init; }
    public int Height { get; init; }
    public ulong DurationMs { get; init; }
    public ulong EncodedFrames { get; init; }
    public ulong CapturedFrames { get; init; }
    public ulong SkippedStatic { get; init; }
    public ulong DroppedBackpressure { get; init; }
    public ulong PoolBytes { get; init; }
    public ulong MaxQueue { get; init; }
    public double FilterMaxUs { get; init; }
    public ulong Bytes { get; init; }
    public string? Path { get; init; }
    public string? Reason { get; init; }
    public string? Error { get; init; }
    public bool Active => Phase is "starting" or "recording" or "paused" or "stopping";
}
internal static class RecordingWindowProtection
{
    [DllImport("user32.dll", SetLastError = true)] private static extern bool SetWindowDisplayAffinity(IntPtr window, uint affinity);
    internal static void Exclude(IntPtr window)
    {
        // Establish protection before Show starts the DWM entrance animation.
        // Protecting an already-animating window can leave its first captured
        // animation surface black even though subsequent frames are excluded.
        if (!SetWindowDisplayAffinity(window, 0x11)) throw new Win32Exception(Marshal.GetLastWin32Error(), "无法排除录制操作窗口，请检查 Windows 捕获支持。");
    }
}

internal interface IRecordingEngine
{
    Task<RecordingStatus> CallAsync(string method, RecordingConfig? config = null);
}
internal sealed class NativeRecordingEngine : IRecordingEngine
{
    private static readonly object Gate = new();
    private static readonly JsonSerializerOptions Json = new(JsonSerializerDefaults.Web) { MaxDepth = 16 };
    private sealed record Response(bool Ok, RecordingStatus? Status, string? Error);
    [DllImport("WinOne.Recording.dll", CallingConvention = CallingConvention.Cdecl, EntryPoint = "winone_recording_command")]
    [DefaultDllImportSearchPaths(DllImportSearchPath.AssemblyDirectory)]
    private static extern IntPtr Command(byte[] input, nuint length);
    [DllImport("WinOne.Recording.dll", CallingConvention = CallingConvention.Cdecl, EntryPoint = "winone_recording_response_free")]
    [DefaultDllImportSearchPaths(DllImportSearchPath.AssemblyDirectory)]
    private static extern void FreeResponse(IntPtr response);

    public Task<RecordingStatus> CallAsync(string method, RecordingConfig? config = null) => Task.Run(() => {
        // Native stop waits for queue drain and file finalization. Keep every
        // call off the UI thread and serialize lifetime operations with polls.
        lock (Gate) {
            var input = JsonSerializer.SerializeToUtf8Bytes(new { method, config }, Json);
            IntPtr result;
            try { result = Command(input, checked((nuint)input.Length)); }
            catch (DllNotFoundException error) { throw new InvalidOperationException("录制组件缺失，请使用包含 WinOne.Recording.dll 的完整构建。", error); }
            catch (BadImageFormatException error) { throw new InvalidOperationException("录制组件架构不匹配，请使用 64 位完整构建。", error); }
            if (result == IntPtr.Zero) throw new InvalidDataException("录制引擎没有返回结果");
            try {
                var text = Marshal.PtrToStringUTF8(result) ?? throw new InvalidDataException("录制结果为空");
                if (Encoding.UTF8.GetByteCount(text) > 65536) throw new InvalidDataException("录制结果超过大小限制");
                var response = JsonSerializer.Deserialize<Response>(text, Json) ?? throw new InvalidDataException("录制结果无效");
                if (!response.Ok) throw new InvalidOperationException(response.Error ?? "录制操作失败");
                return response.Status ?? throw new InvalidDataException("录制状态缺失");
            } finally { FreeResponse(result); }
        }
    });
}
