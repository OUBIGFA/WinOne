using WinOne.Capture;

internal static class TextExportRecoveryTests
{
    public static void Run(string directory)
    {
        foreach (var copy in new[] { false, true })
        {
            var root = Path.Combine(directory, copy ? "save-copy" : "save");
            Directory.CreateDirectory(root);
            var settings = new CaptureSettings { OutputDirectory = root, CopyToClipboard = false, AskWhereToSave = false, PinAfterCapture = false };
            using var source = new Bitmap(320, 200);
            using (var graphics = Graphics.FromImage(source)) graphics.Clear(Color.White);
            var original = new CaptureOperations(new SettingsStore(root), root);
            var draft = original.PrepareEditor(source, settings);
            draft = original.UpdateDraft(draft.Id, draft.Revision, draft.Document with
            {
                Version = 2, Crop = new AnnotationCrop(40, 30, 200, 120),
                Marks = [new AnnotationMark { Kind = "text", X = 60, Y = 50, Width = 160, Height = 80, Text = "WinOne\n项目进度", FontSize = 24, Color = "#000000" }]
            });
            var copied = 0;
            Bitmap? clipboardImage = null;
            try
            {
                // Reopen persisted state, not the original in-memory document.
                // Clipboard output is captured in memory; the user's clipboard is untouched.
                var recovered = new CaptureOperations(new SettingsStore(root), root, image => { copied++; clipboardImage = new Bitmap(image); });
                var restored = recovered.GetDraft() ?? throw new Exception("text draft was lost on restart");
                if (restored.Id != draft.Id || restored.Revision != draft.Revision || restored.Document.Marks.Single().Text != "WinOne\n项目进度") throw new Exception("text draft changed on restart");
                if (!copy)
                {
                    var blocked = Path.Combine(root, "blocked");
                    File.WriteAllText(blocked, "keep");
                    try { recovered.CompleteDraft(restored.Id, restored.Revision, Path.Combine(blocked, "capture.png"), CancellationToken.None); throw new Exception("invalid output path was accepted"); }
                    catch (IOException) { }
                    if (recovered.GetDraft()?.Revision != restored.Revision || File.ReadAllText(blocked) != "keep") throw new Exception("save failure damaged the draft or existing file");
                }
                var output = recovered.CompleteDraft(restored.Id, restored.Revision, null, CancellationToken.None, saveCopy: copy);
                using var saved = new Bitmap(output.Path);
                if (saved.Size != new Size(200, 120) || output.Width != 200 || output.Height != 120) throw new Exception("saved text used the full image instead of the selection");
                var firstLine = 0; var secondLine = 0;
                for (var y = 0; y < saved.Height; y++)
                for (var x = 0; x < saved.Width; x++)
                {
                    var pixel = saved.GetPixel(x, y).ToArgb();
                    if (clipboardImage is not null && clipboardImage.GetPixel(x, y).ToArgb() != pixel) throw new Exception("copied image differs from saved text image");
                    if (pixel == Color.White.ToArgb()) continue;
                    if (x < 20 || x >= 180 || y < 20 || y >= 100) throw new Exception("text moved outside its selected position");
                    if (y < 50) firstLine++; else secondLine++;
                }
                if (firstLine == 0 || secondLine == 0) throw new Exception("saved image lost a line of text");
                if (copied != (copy ? 1 : 0) || output.Copied != copy) throw new Exception("save changed the clipboard preference");
                if (new CaptureOperations(new SettingsStore(root), root).GetDraft() is not null) throw new Exception("completed text draft returned after restart");
            }
            finally { clipboardImage?.Dispose(); }
        }
        Console.WriteLine("PASS: cropped multiline text survives restart and save retry; PNG and copy match; completed draft stays closed");
    }
}
