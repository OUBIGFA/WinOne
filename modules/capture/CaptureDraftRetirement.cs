using System.Text.Json;
using System.Text.Json.Serialization;

namespace WinOne.Capture;

internal sealed partial class CaptureDraftStore
{
    private sealed record RetiredDraft(string Id, string? DesktopPreviewId,
        [property: JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)] long? CompletedRevision = null);
    private string RetirementPath => Path.Combine(Path.GetDirectoryName(path)!, "retired-drafts.json");

    private RetiredDraft[] ReadRetired()
    {
        if (!File.Exists(RetirementPath)) return [];
        using var stream = File.OpenRead(RetirementPath);
        if (stream.Length > 64 * 1024) throw new InvalidDataException("旧草稿清理记录过大");
        var entries = JsonSerializer.Deserialize<RetiredDraft[]>(stream, Json) ?? throw new InvalidDataException("旧草稿清理记录无效");
        if (entries.Length > 64) throw new InvalidDataException("旧草稿清理记录过多");
        foreach (var entry in entries)
        {
            if (entry is null) throw new InvalidDataException("旧草稿清理记录无效");
            if (entry.CompletedRevision is < 0) throw new InvalidDataException("完成记录版本无效");
            _ = ImagePath(entry.Id);
            if (entry.DesktopPreviewId is not null) _ = ImagePath(entry.DesktopPreviewId);
        }
        return entries;
    }

    private void Retire(params CaptureDraft[] drafts)
    {
        var entries = ReadRetired().Concat(drafts.Select(draft => new RetiredDraft(draft.Id, draft.DesktopPreviewId))).DistinctBy(entry => entry.Id).ToArray();
        if (entries.Length > 64) throw new InvalidDataException("旧草稿尚未清理，请检查截图诊断日志");
        SettingsStore.AtomicWrite(RetirementPath, JsonSerializer.SerializeToUtf8Bytes(entries, Json));
    }

    public void Complete(string id, long revision)
    {
        lock (gate)
        {
            var draft = Current(id, revision);
            // The exported PNG already exists. Persist that this exact revision
            // is finished before attempting cleanup, so restart cannot reopen it.
            var entries = ReadRetired().Where(entry => entry.Id != id)
                .Append(new RetiredDraft(id, draft.DesktopPreviewId, revision)).ToArray();
            if (entries.Length > 64) throw new InvalidDataException("旧草稿尚未清理，请检查截图诊断日志");
            SettingsStore.AtomicWrite(RetirementPath, JsonSerializer.SerializeToUtf8Bytes(entries, Json));
            TryCleanupCompleted();
        }
    }

    private void TryCleanupCompleted()
    {
        try { File.Delete(path); }
        catch (Exception error) when (error is IOException or UnauthorizedAccessException)
        {
            // Keep both the completion marker and its image until the metadata
            // can be removed. Otherwise a later read could resurrect a bad draft.
            Console.Error.WriteLine($"Completed capture cleanup pending: {error.Message}");
            return;
        }
        TryCleanupRetired(null);
    }

    // Post-commit maintenance is retryable and durable. A locked obsolete file
    // must not turn a successfully committed replacement into a false failure.
    private void TryCleanupRetired(string? activeId, string? activePreviewId = null)
    {
        try
        {
            var entries = ReadRetired();
            if (entries.Length == 0) return;
            var remaining = new List<RetiredDraft>();
            foreach (var entry in entries)
            {
                if (activeId is not null && (entry.Id == activeId || entry.DesktopPreviewId == activeId || activePreviewId is not null && (entry.Id == activePreviewId || entry.DesktopPreviewId == activePreviewId))) { remaining.Add(entry); continue; }
                try
                {
                    File.Delete(ImagePath(entry.Id));
                    previews.Remove(entry.Id);
                    if (entry.DesktopPreviewId is not null) previews.Remove(entry.DesktopPreviewId);
                }
                catch (Exception error) when (error is IOException or UnauthorizedAccessException)
                {
                    remaining.Add(entry);
                    Console.Error.WriteLine($"Retired capture cleanup pending: {error.Message}");
                }
            }
            if (remaining.Count == 0) File.Delete(RetirementPath);
            else if (remaining.Count != entries.Length)
                SettingsStore.AtomicWrite(RetirementPath, JsonSerializer.SerializeToUtf8Bytes(remaining, Json));
        }
        catch (Exception error) when (error is IOException or UnauthorizedAccessException or JsonException or InvalidDataException)
        {
            Console.Error.WriteLine($"Retired capture cleanup pending: {error.Message}");
        }
    }
}
