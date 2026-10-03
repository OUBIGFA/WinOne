using System.Buffers.Binary;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

internal sealed record NativeCaptureDraft(string Id, long Revision, string Mode, CaptureSettings Settings, CapturePlacement? Placement, WinOneCaptureDocument Document, NativeTextInputState? Text = null, string? Notice = null);

// Original pixels are immutable. Only the small versioned document is replaced
// when editing; a failed update leaves the previous recoverable state intact.
// A background single writer must call these synchronous methods with exclusive
// bitmap access; they do not belong in pointer, paint or toolbar refresh events.
internal sealed partial class NativeCaptureDraftStore
{
    private sealed record Envelope(int Version, string Id, long Revision, string Mode, CaptureSettings Settings, CapturePlacement? Placement, int Width, int Height, string ImageHash, JsonElement Document, NativeTextInputState? Text = null, string? Notice = null);
    private static readonly JsonSerializerOptions Json = new(JsonSerializerDefaults.Web) { PropertyNameCaseInsensitive = false, UnmappedMemberHandling = JsonUnmappedMemberHandling.Disallow, MaxDepth = 32 };
    private readonly string root, metadata, mutexName;
    private readonly Action<Image, string> writeImage;
    public NativeCaptureDraftStore(string directory, Action<Image, string>? writeImage = null)
    {
        this.writeImage = writeImage ?? PngFiles.Write;
        root = Path.GetFullPath(Path.Combine(directory, "native-capture"));
        metadata = Path.Combine(root, "current.json");
        mutexName = "Local\\WinOneNativeDraft-" + Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(root.ToUpperInvariant())));
    }
    private IDisposable Lock()
    {
        var mutex = new Mutex(false, mutexName);
        try {
            try { if (!mutex.WaitOne(TimeSpan.FromSeconds(5))) throw new IOException("原生截图草稿正在更新"); }
            catch (AbandonedMutexException) { /* Atomic metadata decides the committed state after a crash. */ }
            return new Lease(mutex);
        } catch { mutex.Dispose(); throw; }
    }
    private sealed class Lease(Mutex mutex) : IDisposable { public void Dispose() { mutex.ReleaseMutex(); mutex.Dispose(); } }
    private string ImagePath(string id)
    {
        if (!Guid.TryParseExact(id, "N", out var parsed) || parsed.ToString("N") != id) throw new InvalidDataException("原生草稿标识无效");
        return Path.Combine(root, "images", id + ".png");
    }
    public NativeCaptureDraft? Read()
    {
        using var lease = Lock();
        var record = ReadEnvelope();
        return record is null ? null : ToDraft(record);
    }
    private Envelope? ReadEnvelope(bool inspectImage = true, bool includeRetired = false) => ReadRecord(metadata, false, inspectImage, includeRetired);
    private Envelope? ReadRecord(string recordPath, bool pending, bool inspectImage, bool includeRetired)
    {
        FileStream stream;
        try { stream = new FileStream(recordPath, FileMode.Open, FileAccess.Read, FileShare.ReadWrite | FileShare.Delete); }
        catch (FileNotFoundException) { return null; } catch (DirectoryNotFoundException) { return null; }
        using (stream) {
            if (stream.Length > 1024 * 1024) throw new InvalidDataException("原生草稿元数据过大");
            using var parsed = JsonDocument.Parse(stream, new JsonDocumentOptions { MaxDepth = 32 });
            NativeCaptureDocumentCodec.CheckNames(parsed.RootElement);
            var record = parsed.RootElement.Deserialize<Envelope>(Json) ?? throw new InvalidDataException("原生草稿元数据为空");
            _ = ImagePath(record.Id);
            if (record.Version != 1 || record.Revision < 0) throw new InvalidDataException("原生草稿版本无效");
            // Retirement is authoritative even when image cleanup was blocked.
            // Do not inspect a retired image or offer it for recovery again.
            if (!includeRetired && ReadRetired().Any(entry => entry.Id == record.Id)) return null;
            if (record.Settings is null || record.Document.ValueKind != JsonValueKind.Object || record.Mode is not ("window" or "region" or "fullscreen" or "scroll") || (pending ? record.ImageHash != "" || record.Revision != 0 : record.ImageHash is null || record.ImageHash.Length != 64 || !record.ImageHash.All(Uri.IsHexDigit)))
                throw new InvalidDataException("原生草稿元数据无效");
            SettingsStore.Validate(record.Settings);
            ValidateNotice(record.Mode, record.Notice);
            ValidateSize(record.Width, record.Height);
            record.Placement?.Validate(new Size(record.Width, record.Height));
            if (inspectImage) { using var image = OpenImage(record); ValidatePngHeader(image, record.Width, record.Height); }
            return record;
        }
    }
    private static NativeCaptureDraft ToDraft(Envelope record)
    {
        var document = NativeCaptureDocumentCodec.Decode(Encoding.UTF8.GetBytes(record.Document.GetRawText()), new Size(record.Width, record.Height));
        record.Text?.Validate(document);
        return new(record.Id, record.Revision, record.Mode, record.Settings, record.Placement, document, record.Text, record.Notice);
    }
    private static void ValidateNotice(string mode, string? notice)
    {
        if (notice is not null && (mode != "scroll" || notice is not ("scrollStopped" or "scrollPartial" or "scrollTargetChanged")))
            throw new InvalidDataException("截图状态提示无效");
    }
    private static void ValidateSize(int width, int height)
    {
        if (width <= 0 || height <= 0 || width > 65535 || height > 65535 || (long)width * height > 256_000_000)
            throw new InvalidDataException("原生草稿图像尺寸过大或无效");
    }
    private FileStream OpenImage(Envelope record)
    {
        var stream = new FileStream(ImagePath(record.Id), FileMode.Open, FileAccess.Read, FileShare.Read | FileShare.Delete);
        if (stream.Length < 33 || stream.Length > 512L * 1024 * 1024) { stream.Dispose(); throw new InvalidDataException("原生草稿图像文件无效"); }
        return stream;
    }
    private static void ValidatePngHeader(Stream stream, int width, int height)
    {
        Span<byte> header = stackalloc byte[24]; stream.ReadExactly(header); stream.Position = 0;
        if (!header[..8].SequenceEqual(new byte[] { 137, 80, 78, 71, 13, 10, 26, 10 }) || BinaryPrimitives.ReadUInt32BigEndian(header[8..12]) != 13 || !header[12..16].SequenceEqual("IHDR"u8)
            || BinaryPrimitives.ReadUInt32BigEndian(header[16..20]) != (uint)width || BinaryPrimitives.ReadUInt32BigEndian(header[20..24]) != (uint)height)
            throw new InvalidDataException("原生草稿图像与文档不一致");
    }
    public NativeCaptureDraft Begin(Bitmap image, CaptureSettings settings, WinOneCaptureDocument document, string mode, CapturePlacement? placement = null)
    {
        var reservation = Reserve(image.Size, settings, document, mode, placement);
        try { return CompleteReservation(reservation.Id, image); }
        catch {
            try { CancelReservation(reservation.Id); } catch (Exception error) { Console.Error.WriteLine("Native draft cleanup: " + error.Message); }
            throw;
        }
    }
    public NativeCaptureDraft Update(string id, long revision, WinOneCaptureDocument document, NativeTextInputState? text = null)
    {
        using var lease = Lock();
        var current = Current(id, revision);
        text?.Validate(document);
        using var encoded = JsonDocument.Parse(NativeCaptureDocumentCodec.Encode(document, new Size(current.Width, current.Height)));
        var next = current with { Revision = checked(current.Revision + 1), Document = encoded.RootElement.Clone(), Text = text };
        Write(next); return ToDraft(next);
    }
    private Envelope Current(string id, long revision)
    {
        var current = ReadEnvelope() ?? throw new InvalidOperationException("原生草稿不存在");
        if (id != current.Id || revision != current.Revision) throw new InvalidOperationException("原生草稿已变化，请重新加载");
        _ = ToDraft(current);
        return current;
    }
    private void Write(Envelope record)
    {
        var bytes = JsonSerializer.SerializeToUtf8Bytes(record, Json);
        if (bytes.Length > 1024 * 1024) throw new InvalidDataException("原生草稿元数据过大");
        SettingsStore.AtomicWrite(metadata, bytes);
    }
    public Bitmap LoadImage(string id, long revision)
    {
        using var lease = Lock();
        var current = Current(id, revision);
        using var stream = OpenImage(current);
        ValidatePngHeader(stream, current.Width, current.Height);
        if (!Convert.ToHexString(SHA256.HashData(stream)).Equals(current.ImageHash, StringComparison.OrdinalIgnoreCase)) throw new InvalidDataException("原生草稿原图已变化");
        stream.Position = 0;
        using var decoded = Image.FromStream(stream, useEmbeddedColorManagement: false, validateImageData: true);
        if (decoded.Width != current.Width || decoded.Height != current.Height) throw new InvalidDataException("原生草稿原图尺寸不一致");
        return new Bitmap(decoded);
    }
}
