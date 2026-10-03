using System.Security.Cryptography;
using System.Text.Json;
using WinOne.Capture;
using ShareX.ScreenCaptureLib;

internal static class NativeDraftStoreTests
{
    private static int[] Pixels(Bitmap image) => Enumerable.Range(0, image.Height).SelectMany(y => Enumerable.Range(0, image.Width).Select(x => image.GetPixel(x, y).ToArgb())).ToArray();
    public static int Run(string output)
    {
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output); output = Path.GetFullPath(output);
        using var source = new Bitmap(180, 120);
        using (var graphics = Graphics.FromImage(source)) graphics.Clear(Color.LightBlue);
        using var canvas = new WinOneCaptureCanvas(source);
        canvas.SetRegion(new Rectangle(10, 10, 160, 100));
        canvas.AddRectangle(new Rectangle(30, 25, 65, 45));
        var store = new NativeCaptureDraftStore(output);
        if (store.Read() is not null) throw new Exception("New store had a draft");
        var settings = new CaptureSettings { OutputDirectory = Path.Combine(output, "exports"), FileNameTemplate = "work-{date}", CopyToClipboard = false };
        var placement = CapturePlacement.From(new Rectangle(-1920, 0, 180, 120), new Rectangle(-1920, 0, 180, 120));
        var draft = store.Begin(source, settings, canvas.ExportDocument(), "window", placement);
        var imagePath = Path.Combine(output, "native-capture", "images", draft.Id + ".png");
        var metadataPath = Path.Combine(output, "native-capture", "current.json");
        var originalPng = File.ReadAllBytes(imagePath); var originalHash = SHA256.HashData(originalPng);
        var reopened = new NativeCaptureDraftStore(output);
        var loaded = reopened.Read() ?? throw new Exception("Persisted draft disappeared");
        if (loaded.Id != draft.Id || loaded.Revision != 0 || loaded.Settings.FileNameTemplate != settings.FileNameTemplate || loaded.Placement != placement)
            throw new Exception("Draft metadata did not survive reopen");
        using (var original = reopened.LoadImage(loaded.Id, loaded.Revision)) {
            if (!Pixels(original).SequenceEqual(Pixels(source))) throw new Exception("Original PNG changed pixels");
            using var restored = new WinOneCaptureCanvas(original); restored.RestoreDocument(loaded.Document);
            using var before = canvas.RenderSelection(); using var after = restored.RenderSelection();
            if (!Pixels(before).SequenceEqual(Pixels(after))) throw new Exception("Persisted editable layers changed output");
        }
        canvas.AddText("原生恢复", new Rectangle(25, 30, 130, 70), 24, new[] { new WinOneTextLine("原生恢复", 0, 0) });
        var next = store.Update(draft.Id, draft.Revision, canvas.ExportDocument());
        if (next.Revision != 1 || reopened.Read()?.Revision != 1 || !SHA256.HashData(File.ReadAllBytes(imagePath)).SequenceEqual(originalHash))
            throw new Exception("Document update changed the immutable original or lost its revision");
        try { store.Update(draft.Id, 0, canvas.ExportDocument()); throw new Exception("Stale draft update was accepted"); } catch (InvalidOperationException) { }
        try { store.Begin(source, settings, canvas.ExportDocument(), "window", placement); throw new Exception("Active draft was overwritten"); } catch (InvalidOperationException) { }
        var savedMetadata = File.ReadAllBytes(metadataPath);
        var attributes = File.GetAttributes(metadataPath);
        File.SetAttributes(metadataPath, attributes | FileAttributes.ReadOnly);
        try {
            try { store.Update(draft.Id, 1, canvas.ExportDocument()); throw new Exception("Read-only metadata update unexpectedly succeeded"); }
            catch (UnauthorizedAccessException) { } catch (IOException) { }
        } finally { File.SetAttributes(metadataPath, attributes); }
        if (!File.ReadAllBytes(metadataPath).SequenceEqual(savedMetadata) || reopened.Read()?.Revision != 1) throw new Exception("Failed save damaged the previous draft");
        using (var replacement = new Bitmap(source.Width, source.Height)) PngFiles.Write(replacement, imagePath);
        try { reopened.LoadImage(draft.Id, 1).Dispose(); throw new Exception("Changed original PNG was accepted"); } catch (InvalidDataException) { }
        File.WriteAllBytes(imagePath, originalPng);
        var invalid = originalPng.ToArray(); invalid[19] ^= 1; File.WriteAllBytes(imagePath, invalid);
        try { reopened.Read(); throw new Exception("Mismatched PNG header was accepted"); } catch (InvalidDataException) { }
        File.WriteAllBytes(imagePath, originalPng);
        if (reopened.Read()?.Revision != 1) throw new Exception("Image validation damaged metadata");
        File.WriteAllText(Path.Combine(output, "report.json"), JsonSerializer.Serialize(new { reopened = true, editable = true, originalUnchanged = true, staleRejected = true, failedWritePreserved = true, changedImageRejected = true }));
        Console.WriteLine("PASS: native draft disk roundtrip, immutable pixels, revision guard, failed-write preservation and image validation");
        return 0;
    }
}
