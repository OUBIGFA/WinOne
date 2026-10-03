using System.Text.Json;
using WinOne.Capture;

internal static class CaptureDraftReplacementTests
{
    public static void Run(string directory)
    {
        var store = new CaptureDraftStore(directory);
        var settings = new CaptureSettings { OutputDirectory = directory };
        using var original = new Bitmap(40, 30);
        using (var graphics = Graphics.FromImage(original)) graphics.Clear(Color.Coral);
        using var replacement = new Bitmap(50, 40);
        using (var graphics = Graphics.FromImage(replacement)) graphics.Clear(Color.SteelBlue);
        var old = store.Begin(original, settings);
        old = store.Update(old.Id, old.Revision, old.Document with { Marks = [new AnnotationMark { Kind = "cover", X = 1, Y = 1, Width = 4, Height = 4, Color = "#000000" }] });
        try { store.Begin(replacement, settings, replaceId: old.Id, replaceRevision: 0); throw new Exception("stale replacement accepted"); }
        catch (InvalidOperationException) { }

        var metadata = Path.Combine(directory, "capture-draft.json");
        var oldPath = Path.Combine(directory, "drafts", old.Id + ".png");
        var attributes = File.GetAttributes(metadata);
        File.SetAttributes(metadata, attributes | FileAttributes.ReadOnly);
        try
        {
            try { store.Begin(replacement, settings, replaceId: old.Id, replaceRevision: old.Revision); throw new Exception("read-only metadata replaced"); }
            catch (UnauthorizedAccessException) { }
            catch (IOException) { }
            if (store.Read()?.Id != old.Id || store.Read()?.Document.Marks.Count != 1 || !File.Exists(oldPath)) throw new Exception("failed replacement lost the old draft");
            if (Directory.GetFiles(Path.Combine(directory, "drafts")).Length != 1) throw new Exception("failed replacement left a new source behind");
        }
        finally { File.SetAttributes(metadata, attributes); }

        // Simulate a crash after candidate pixels were written, before commit.
        var candidate = Guid.NewGuid().ToString("N");
        var candidatePath = Path.Combine(directory, "drafts", candidate + ".png");
        PngFiles.Write(replacement, candidatePath);
        PngFiles.Write(replacement, Path.Combine(directory, "previews", candidate + ".png"));
        var journal = Path.Combine(directory, "retired-drafts.json");
        File.WriteAllBytes(journal, JsonSerializer.SerializeToUtf8Bytes(new[] { new { Id = old.Id, DesktopPreviewId = (string?)null }, new { Id = candidate, DesktopPreviewId = (string?)null } }, SettingsStore.Json));
        if (new CaptureDraftStore(directory).Read()?.Id != old.Id || !File.Exists(oldPath) || File.Exists(candidatePath)) throw new Exception("recovery did not preserve the committed identity");

        // Post-commit deletion failure keeps a retry record, not a failed retake.
        var oldAttributes = File.GetAttributes(oldPath);
        CaptureDraft next;
        File.SetAttributes(oldPath, oldAttributes | FileAttributes.ReadOnly);
        try
        {
            next = store.Begin(replacement, settings, replaceId: old.Id, replaceRevision: old.Revision);
            if (next.Id == old.Id || store.Read()?.Id != next.Id || !File.Exists(oldPath) || !File.Exists(journal)) throw new Exception("cleanup failure changed the committed replacement");
            using var loaded = store.Load(next.Id);
            if (loaded.Size != replacement.Size || loaded.GetPixel(0, 0).ToArgb() != Color.SteelBlue.ToArgb()) throw new Exception("replacement pixels changed");
        }
        finally { if (File.Exists(oldPath)) File.SetAttributes(oldPath, oldAttributes); }
        var recovered = new CaptureDraftStore(directory).Read();
        if (recovered?.Id != next.Id || File.Exists(oldPath)) throw new Exception("deferred cleanup did not recover");
        if (recovered.Replaces != new CaptureReplacement(old.Id, old.Revision)) throw new Exception("replacement lineage did not survive restart");
        store.Cancel(next.Id, next.Revision);
        if (File.Exists(journal) || Directory.GetFiles(Path.Combine(directory, "drafts")).Length != 0) throw new Exception("cancel retained retired images");
    }
}
