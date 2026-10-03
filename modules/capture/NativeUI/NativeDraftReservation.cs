using System.Security.Cryptography;
using System.Text.Json;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

internal sealed record NativeDraftReservation(string Id, int Width, int Height);

internal sealed partial class NativeCaptureDraftStore
{
    private string PendingPath => Path.Combine(root, "pending.json");
    private Envelope? ReadPending(bool includeRetired = false) => ReadRecord(PendingPath, true, false, includeRetired);
    private FileStream EncodingLease(string id)
    {
        _ = ImagePath(id);
        var directory = Path.Combine(root, "encoding"); Directory.CreateDirectory(directory);
        // The OS releases this exclusion after process failure. Cleanup never
        // mistakes a still-running encoder for an abandoned generation.
        return new FileStream(Path.Combine(directory, id + ".lock"), FileMode.OpenOrCreate, FileAccess.ReadWrite, FileShare.None, 1, FileOptions.DeleteOnClose);
    }
    public NativeDraftReservation? ReadReservation()
    {
        using var lease = Lock();
        var record = ReadPending();
        return record is null ? null : new(record.Id, record.Width, record.Height);
    }
    public NativeCaptureDraft? Recover()
    {
        using var lease = Lock();
        var current = ReadEnvelope();
        if (current is not null) return ToDraft(current);
        var pending = ReadPending();
        if (pending is null) return null;
        // An active encoder owns the same lease. Startup must never promote its
        // file or change its marker while that process can still commit it.
        using var encoding = EncodingLease(pending.Id);
        var draft = ToDraft(pending);
        try {
            using var source = OpenImage(pending);
            ValidatePngHeader(source, pending.Width, pending.Height);
            var hash = Convert.ToHexString(SHA256.HashData(source));
            source.Position = 0;
            using var decoded = Image.FromStream(source, useEmbeddedColorManagement: false, validateImageData: true);
            using var pixels = new Bitmap(decoded);
            if (pixels.Size != new Size(pending.Width, pending.Height)) throw new InvalidDataException("截图原图尺寸不一致");
            // PngFiles publishes the completed PNG atomically. A crash between
            // that rename and metadata commit can be recovered without recapture.
            Write(pending with { ImageHash = hash });
        } catch (FileNotFoundException error) {
            throw new NativeDraftIncompleteException(pending.Id, error);
        } catch (DirectoryNotFoundException error) {
            throw new NativeDraftIncompleteException(pending.Id, error);
        }
        try { File.Delete(PendingPath); }
        catch (Exception error) when (error is IOException or UnauthorizedAccessException) { Console.Error.WriteLine("Native pending cleanup: " + error.Message); }
        return draft;
    }
    public NativeDraftReservation Reserve(Size image, CaptureSettings settings, WinOneCaptureDocument document, string mode, CapturePlacement? placement = null, string? notice = null)
    {
        using var lease = Lock();
        _ = ReadRetired();
        if (ReadEnvelope() is not null || ReadPending() is not null) throw new InvalidOperationException("请先完成或放弃当前原生草稿");
        if (mode is not ("window" or "region" or "fullscreen" or "scroll")) throw new InvalidDataException("原生截图类型无效");
        SettingsStore.Validate(settings); ValidateSize(image.Width, image.Height); placement?.Validate(image);
        ValidateNotice(mode, notice);
        using var encoded = JsonDocument.Parse(NativeCaptureDocumentCodec.Encode(document, image));
        var id = Guid.NewGuid().ToString("N");
        var record = new Envelope(1, id, 0, mode, settings with { ToolbarTools = settings.ToolbarTools?.ToArray() }, placement, image.Width, image.Height, "", encoded.RootElement.Clone(), Notice: notice);
        var bytes = JsonSerializer.SerializeToUtf8Bytes(record, Json);
        if (bytes.Length > 1024 * 1024) throw new InvalidDataException("原生草稿元数据过大");
        SettingsStore.AtomicWrite(PendingPath, bytes);
        return new(id, image.Width, image.Height);
    }
    public NativeCaptureDraft CompleteReservation(string id, Bitmap image)
    {
        try {
            using var encoding = EncodingLease(id);
            Envelope pending;
            using (var lease = Lock()) {
                pending = ReadPending() ?? throw new OperationCanceledException("原生草稿保存已取消");
                if (pending.Id != id || ReadEnvelope() is not null) throw new OperationCanceledException("原生草稿保存已失效");
                if (image.Size != new Size(pending.Width, pending.Height)) throw new InvalidDataException("原生草稿图像尺寸不一致");
                _ = ToDraft(pending);
            }
            // Encoding holds only its generation lease, not the profile mutex.
            // Cancellation and the next generation can therefore proceed.
            var path = ImagePath(id); writeImage(image, path);
            string hash;
            using (var file = File.OpenRead(path)) hash = Convert.ToHexString(SHA256.HashData(file));
            using (var lease = Lock()) {
                var current = ReadPending();
                if (current?.Id != id || ReadEnvelope() is not null) throw new OperationCanceledException("原生草稿保存已失效");
                var committed = pending with { ImageHash = hash };
                var result = ToDraft(committed);
                Write(committed);
                // A stale pending marker is harmless once current.json exists.
                // It is also handled by retirement cleanup if deletion fails.
                try { File.Delete(PendingPath); } catch (Exception error) when (error is IOException or UnauthorizedAccessException) { Console.Error.WriteLine("Native pending cleanup: " + error.Message); }
                return result;
            }
        } finally { RetryCleanup(); }
    }
    public NativeDraftCleanup CancelReservation(string id)
    {
        using var lease = Lock();
        _ = ImagePath(id);
        var entries = ReadRetired();
        if (!entries.Any(entry => entry.Id == id)) {
            var pending = ReadPending(includeRetired: true);
            var current = ReadEnvelope(inspectImage: false, includeRetired: true);
            if (current?.Id == id) RecordRetirement(id, current.Revision);
            else if (pending?.Id == id) RecordRetirement(id, 0);
            else throw new InvalidOperationException("原生草稿保存标识已变化");
        }
        return CleanupRetiredCore();
    }
    private void DeleteImageFiles(string id)
    {
        var path = ImagePath(id); var directory = Path.GetDirectoryName(path)!;
        if (!Directory.Exists(directory)) return;
        File.Delete(path);
        var prefix = Path.GetFileName(path) + ".";
        foreach (var temporary in Directory.EnumerateFiles(directory, prefix + "*.tmp")) {
            var name = Path.GetFileName(temporary); var token = name[prefix.Length..^4];
            if (Guid.TryParseExact(token, "N", out var parsed) && parsed.ToString("N") == token) File.Delete(temporary);
        }
    }
}

internal sealed class NativeDraftIncompleteException(string id, Exception inner)
    : IOException("上次截图未保存完整，无法恢复。", inner)
{
    public string DraftId { get; } = id;
}
