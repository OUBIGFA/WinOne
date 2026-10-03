using WinOne.Capture;

internal static class CaptureDraftCancellationTests
{
    internal static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        using var image = new Bitmap(24, 16);
        image.SetPixel(5, 5, Color.Coral);
        var store = new CaptureDraftStore(directory);
        var draft = store.Begin(image, new CaptureSettings());
        var metadata = Path.Combine(directory, "capture-draft.json");
        // A viewer or scanner may allow reads while temporarily denying delete.
        using (var lockedMetadata = new FileStream(metadata, FileMode.Open, FileAccess.Read, FileShare.Read))
        {
            try { store.Cancel(draft.Id, draft.Revision); throw new Exception("locked metadata cancellation unexpectedly succeeded"); }
            catch (IOException) { }
            using var recovered = new CaptureDraftStore(directory).Load(draft.Id);
            if (recovered.GetPixel(5, 5).ToArgb() != Color.Coral.ToArgb()) throw new Exception("failed cancellation lost the recoverable source");
        }
        store.Cancel(draft.Id, draft.Revision);
        if (store.Read() is not null) throw new Exception("cancellation did not recover after the lock was released");

        draft = store.Begin(image, new CaptureSettings());
        var source = Path.Combine(directory, "drafts", draft.Id + ".png");
        using (var lockedImage = new FileStream(source, FileMode.Open, FileAccess.Read, FileShare.Read))
        {
            store.Cancel(draft.Id, draft.Revision);
            if (store.Read() is not null || !File.Exists(source)) throw new Exception("locked retired source remained an active draft");
            if (!File.Exists(Path.Combine(directory, "retired-drafts.json"))) throw new Exception("deferred cleanup was not persisted");
        }
        if (new CaptureDraftStore(directory).Read() is not null || File.Exists(source)) throw new Exception("restart did not clean up the retired image");

        var saveDirectory = Path.Combine(directory, "save");
        Directory.CreateDirectory(saveDirectory);
        var operations = new CaptureOperations(new SettingsStore(saveDirectory), saveDirectory);
        draft = operations.PrepareEditor(image, new CaptureSettings { OutputDirectory = saveDirectory, CopyToClipboard = false, AskWhereToSave = false });
        source = Path.Combine(saveDirectory, "drafts", draft.Id + ".png");
        using (var lockedImage = new FileStream(source, FileMode.Open, FileAccess.Read, FileShare.Read))
        {
            var output = operations.CompleteDraft(draft.Id, draft.Revision, null, CancellationToken.None);
            using var saved = new Bitmap(output.Path);
            if (saved.Size != image.Size || saved.GetPixel(5, 5).ToArgb() != Color.Coral.ToArgb()) throw new Exception("saved image changed during deferred cleanup");
            if (operations.GetDraft() is not null) throw new Exception("successful save retained an active draft");
        }
        if (new CaptureDraftStore(saveDirectory).Read() is not null || File.Exists(source)) throw new Exception("completed save did not retire its locked source");

        draft = operations.PrepareEditor(image, new CaptureSettings { OutputDirectory = saveDirectory, CopyToClipboard = false, AskWhereToSave = false });
        source = Path.Combine(saveDirectory, "drafts", draft.Id + ".png");
        var savedCount = Directory.GetFiles(saveDirectory, "WinOne-*.png").Length;
        using (var lockedMetadata = new FileStream(Path.Combine(saveDirectory, "capture-draft.json"), FileMode.Open, FileAccess.Read, FileShare.Read))
        {
            try { operations.CancelDraft(draft.Id, draft.Revision); throw new Exception("cancellation ignored the metadata lock"); }
            catch (IOException) { }
            var output = operations.CompleteDraft(draft.Id, draft.Revision, null, CancellationToken.None);
            if (!File.Exists(output.Path) || operations.GetDraft() is not null || new CaptureDraftStore(saveDirectory).Read() is not null)
                throw new Exception("saved screenshot was offered for editing again after cleanup failed");
            if (!File.Exists(source)) throw new Exception("completion marker was not retained with locked metadata");
        }
        if (new CaptureDraftStore(saveDirectory).Read() is not null || File.Exists(source)) throw new Exception("completed metadata did not clean up after restart");
        if (Directory.GetFiles(saveDirectory, "WinOne-*.png").Length != savedCount + 1) throw new Exception("cleanup created duplicate saved images");
    }
}
