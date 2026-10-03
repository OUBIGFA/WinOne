using System.Text.Json;

namespace WinOne.Capture.NativeUI;

internal sealed record NativeDraftCleanup(bool Pending, IReadOnlyList<string> Errors);

internal sealed partial class NativeCaptureDraftStore
{
    private sealed record Retired(string Id, long Revision);
    private string RetirementPath => Path.Combine(root, "retired.json");
    private Retired[] ReadRetired()
    {
        FileStream stream;
        try { stream = new FileStream(RetirementPath, FileMode.Open, FileAccess.Read, FileShare.ReadWrite | FileShare.Delete); }
        catch (FileNotFoundException) { return []; } catch (DirectoryNotFoundException) { return []; }
        using (stream) {
            if (stream.Length > 64 * 1024) throw new InvalidDataException("原生草稿清理记录过大");
            using var parsed = JsonDocument.Parse(stream, new JsonDocumentOptions { MaxDepth = 8 });
            NativeCaptureDocumentCodec.CheckNames(parsed.RootElement);
            var entries = parsed.RootElement.Deserialize<Retired[]>(Json) ?? throw new InvalidDataException("原生草稿清理记录无效");
            if (entries.Length > 64) throw new InvalidDataException("原生草稿清理记录过多");
            var seen = new HashSet<string>(StringComparer.Ordinal);
            foreach (var entry in entries) {
                if (entry is null || entry.Revision < 0 || !seen.Add(entry.Id)) throw new InvalidDataException("原生草稿清理标识无效");
                _ = ImagePath(entry.Id);
            }
            return entries;
        }
    }
    public NativeDraftCleanup Retire(string id, long revision)
    {
        using var lease = Lock();
        _ = ImagePath(id);
        var entries = ReadRetired();
        if (!entries.Any(entry => entry.Id == id && entry.Revision == revision)) {
            var current = ReadEnvelope(inspectImage: false, includeRetired: true) ?? throw new InvalidOperationException("原生草稿不存在");
            if (current.Id != id || current.Revision != revision) throw new InvalidOperationException("原生草稿已变化，请重新加载");
            // Persist the exact accepted retirement before deleting either file.
            // A crash or read-only file must not resurrect a cancelled capture.
            RecordRetirement(id, revision);
        }
        return CleanupRetiredCore();
    }
    private void RecordRetirement(string id, long revision)
    {
        var next = ReadRetired().Where(entry => entry.Id != id).Append(new Retired(id, revision)).ToArray();
        if (next.Length > 64) throw new InvalidDataException("旧原生草稿尚未清理，请检查诊断日志");
        SettingsStore.AtomicWrite(RetirementPath, JsonSerializer.SerializeToUtf8Bytes(next, Json));
    }
    public NativeDraftCleanup RetryCleanup()
    {
        using var lease = Lock();
        return CleanupRetiredCore();
    }
    private NativeDraftCleanup CleanupRetiredCore()
    {
        var errors = new List<string>();
        try {
            var entries = ReadRetired();
            if (entries.Length == 0) return new(false, errors);
            var current = ReadEnvelope(inspectImage: false, includeRetired: true);
            var pending = ReadPending(includeRetired: true);
            var remaining = new List<Retired>();
            foreach (var entry in entries) {
                try {
                    FileStream encoding;
                    try { encoding = EncodingLease(entry.Id); }
                    catch (IOException error) when ((error.HResult & 0xffff) == 32) { remaining.Add(entry); continue; }
                    using var encodingGuard = encoding;
                    if (pending?.Id == entry.Id) { File.Delete(PendingPath); pending = null; }
                    if (current?.Id == entry.Id) {
                        File.Delete(metadata);
                        current = null;
                    }
                    DeleteImageFiles(entry.Id);
                } catch (Exception error) when (error is IOException or UnauthorizedAccessException) {
                    remaining.Add(entry); errors.Add(error.Message);
                }
            }
            if (remaining.Count == 0) File.Delete(RetirementPath);
            else if (remaining.Count != entries.Length) SettingsStore.AtomicWrite(RetirementPath, JsonSerializer.SerializeToUtf8Bytes(remaining, Json));
            foreach (var error in errors) Console.Error.WriteLine("Native draft cleanup pending: " + error);
            return new(remaining.Count != 0, errors);
        } catch (Exception error) when (error is IOException or UnauthorizedAccessException or JsonException or InvalidDataException) {
            Console.Error.WriteLine("Native draft cleanup pending: " + error.Message);
            errors.Add(error.Message); return new(true, errors);
        }
    }
}
