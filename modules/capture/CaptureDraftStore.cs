using System.Text.Json;
using System.Text.Json.Serialization;

namespace WinOne.Capture;

internal record CaptureReplacement(string Id, long Revision);
internal record CaptureDraft(int SchemaVersion, string Id, long Revision, int Width, int Height,
    CaptureSettings Settings, AnnotationDocument Document, bool PreviewReady, CapturePlacement? Placement = null, string? DesktopPreviewId = null, bool SelectionRequired = false, string? Notice = null, string? SourceKind = null, CaptureReplacement? Replaces = null, bool SelectionPending = false);

// One recoverable editing session per profile. File locations are derived from
// validated IDs; cancellation never trusts a path stored in the document.
internal sealed partial class CaptureDraftStore(string directory)
{
    private const int MaximumBytes = 768 * 1024;
    private static readonly JsonSerializerOptions Json = new(SettingsStore.Json)
    { WriteIndented = false, UnmappedMemberHandling = JsonUnmappedMemberHandling.Disallow };
    private readonly object gate = new();
    private readonly string path = Path.Combine(directory, "capture-draft.json");
    private readonly CapturePreviews previews = new(directory);

    private string ImagePath(string id)
    {
        if (!Guid.TryParseExact(id, "N", out var value) || value.ToString("N") != id) throw new InvalidDataException("草稿标识无效");
        return Path.Combine(directory, "drafts", id + ".png");
    }

    public CaptureDraft? Read()
    {
        lock (gate)
        {
            FileStream stream;
            try { stream = File.OpenRead(path); }
            catch (FileNotFoundException) { TryCleanupRetired(null); return null; }
            catch (DirectoryNotFoundException) { TryCleanupRetired(null); return null; }
            CaptureDraft draft;
            using (stream)
            {
                if (stream.Length > MaximumBytes) throw new InvalidDataException("截图草稿过大");
                draft = JsonSerializer.Deserialize<CaptureDraft>(stream, Json) ?? throw new InvalidDataException("截图草稿为空");
                _ = ImagePath(draft.Id);
                if (draft.SchemaVersion != 1 || draft.Revision < 0 || draft.Settings is null || draft.Document is null) throw new InvalidDataException("截图草稿格式无效");
                AnnotationRenderer.ValidateDocument(new Size(draft.Width, draft.Height), draft.Document);
                if (draft.SelectionRequired && draft.Document.Crop is null) throw new InvalidDataException("截图选区丢失，请重新截图");
                if (draft.Notice is not (null or "scrollStopped" or "scrollPartial" or "scrollTargetChanged")) throw new InvalidDataException("截图提示无效");
                if (draft.SourceKind is not (null or "scroll" or "window" or "region" or "fullscreen")) throw new InvalidDataException("截图类型无效");
                if (draft.SelectionPending && (draft.SourceKind is not ("window" or "region") || !draft.SelectionRequired || draft.Document.Marks.Count != 0)) throw new InvalidDataException("截图选区状态无效");
                if (draft.Replaces is { } replaced)
                {
                    _ = ImagePath(replaced.Id);
                    if (replaced.Id == draft.Id || replaced.Revision < 0) throw new InvalidDataException("重截来源无效");
                }
                draft.Placement?.Validate(new Size(draft.Width, draft.Height));
                if (draft.DesktopPreviewId is { } desktopId && (draft.Placement is null || desktopId == draft.Id || !Guid.TryParseExact(desktopId, "N", out var desktopGuid) || desktopGuid.ToString("N") != desktopId)) throw new InvalidDataException("桌面预览标识无效");
            }
            if (ReadRetired().Any(entry => entry.Id == draft.Id && entry.CompletedRevision == draft.Revision))
            {
                TryCleanupCompleted();
                return null;
            }
            TryCleanupRetired(draft.Id, draft.DesktopPreviewId);
            return draft;
        }
    }

    private CaptureDraft Current(string id, long? revision = null)
    {
        var draft = Read() ?? throw new InvalidOperationException("没有待编辑的截图");
        if (draft.Id != id || revision.HasValue && draft.Revision != revision.Value) throw new InvalidOperationException("截图草稿已变化，请重新加载");
        return draft;
    }

    private static byte[] Encode(CaptureDraft draft)
    {
        var bytes = JsonSerializer.SerializeToUtf8Bytes(draft, Json);
        // Leave room for RPC envelopes within the one-MiB wire limit.
        if (bytes.Length > MaximumBytes) throw new InvalidDataException("截图草稿过大，请减少标注");
        return bytes;
    }

    public CaptureDraft Begin(Bitmap image, CaptureSettings settings, CapturePlacement? placement = null, Bitmap? desktopPreview = null, AnnotationCrop? selection = null, string? notice = null, string? sourceKind = null, string? replaceId = null, long? replaceRevision = null, bool selectionPending = false)
    {
        lock (gate)
        {
            var previous = Read();
            if (replaceId is null)
            {
                if (previous is not null) throw new InvalidOperationException("请先完成或放弃当前截图草稿");
            }
            else if (previous is null || previous.Id != replaceId || !replaceRevision.HasValue || previous.Revision != replaceRevision.Value)
                throw new InvalidOperationException("截图草稿已变化，请重新加载");
            if (notice is not (null or "scrollStopped" or "scrollPartial" or "scrollTargetChanged")) throw new InvalidDataException("截图提示无效");
            if (sourceKind is not (null or "scroll" or "window" or "region" or "fullscreen")) throw new InvalidDataException("截图类型无效");
            if (selectionPending && (sourceKind is not ("window" or "region") || selection is null)) throw new InvalidDataException("截图选区状态无效");
            placement?.Validate(image.Size);
            if (desktopPreview is not null && (placement is null || Math.Max(desktopPreview.Width, desktopPreview.Height) > 1600)) throw new InvalidDataException("桌面预览范围无效");
            var document = new AnnotationDocument { Version = selection is null ? 1 : 2, Width = image.Width, Height = image.Height, Crop = selection };
            AnnotationRenderer.ValidateDocument(image.Size, document);
            var draft = new CaptureDraft(1, Guid.NewGuid().ToString("N"), 0, image.Width, image.Height, settings,
                document, true, placement, desktopPreview is null ? null : Guid.NewGuid().ToString("N"), selection is not null, notice, sourceKind, previous is null ? null : new CaptureReplacement(previous.Id, previous.Revision), selectionPending);
            var bytes = Encode(draft);
            var source = ImagePath(draft.Id);
            try
            {
                // Track both sides before writing pixels. After a crash, the
                // metadata identity decides which side must be preserved.
                if (previous is not null) Retire(previous, draft);
                PngFiles.Write(image, source);
                previews.Write(image, draft.Id);
                if (desktopPreview is not null) previews.Write(desktopPreview, draft.DesktopPreviewId!);
                SettingsStore.AtomicWrite(path, bytes);
            }
            catch
            {
                File.Delete(source);
                previews.Remove(draft.Id);
                if (draft.DesktopPreviewId is not null) previews.Remove(draft.DesktopPreviewId);
                if (previous is not null) TryCleanupRetired(previous.Id, previous.DesktopPreviewId);
                throw;
            }
            TryCleanupRetired(draft.Id, draft.DesktopPreviewId);
            return draft;
        }
    }

    public CaptureDraft Update(string id, long revision, AnnotationDocument document)
    {
        lock (gate)
        {
            var draft = Current(id, revision);
            if (draft.SelectionPending) throw new InvalidOperationException("请先选择截图区域");
            if (draft.SelectionRequired && document.Crop is null) throw new InvalidDataException("请保留截图选区");
            AnnotationRenderer.ValidateDocument(new Size(draft.Width, draft.Height), document);
            var next = draft with { Revision = checked(draft.Revision + 1), Document = document };
            SettingsStore.AtomicWrite(path, Encode(next));
            return Read()!;
        }
    }

    public CaptureDraft Select(string id, long revision, AnnotationCrop selection)
    {
        lock (gate)
        {
            var draft = Current(id, revision);
            if (!draft.SelectionPending) throw new InvalidOperationException("截图区域已确定");
            var document = draft.Document with { Crop = selection };
            AnnotationRenderer.ValidateDocument(new Size(draft.Width, draft.Height), document);
            var next = draft with { Document = document, SelectionPending = false, Revision = checked(draft.Revision + 1) };
            SettingsStore.AtomicWrite(path, Encode(next));
            return Read()!;
        }
    }

    public Bitmap Load(string id)
    {
        lock (gate)
        {
            var draft = Current(id);
            using var source = new Bitmap(ImagePath(id));
            if (source.Width != draft.Width || source.Height != draft.Height) throw new InvalidDataException("截图草稿尺寸已变化");
            return new Bitmap(source);
        }
    }

    public void Cancel(string id, long revision)
    {
        lock (gate)
        {
            var draft = Current(id, revision);
            // Commit logical cancellation before deleting pixels. If metadata
            // is locked, the source remains recoverable. After commit, the
            // existing retirement journal retries any locked obsolete files.
            Retire(draft);
            File.Delete(path);
            TryCleanupRetired(null);
        }
    }
}
