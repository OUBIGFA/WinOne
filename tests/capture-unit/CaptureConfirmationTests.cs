using WinOne.Capture;

internal static class CaptureConfirmationTests
{
    public static void Run(string root)
    {
        Directory.CreateDirectory(root);
        var output = Path.Combine(root, "output");
        var settings = new CaptureSettings { OutputDirectory = output, CopyToClipboard = false, OpenEditor = true };
        var copies = 0;
        var fail = true;
        var operations = new CaptureOperations(new SettingsStore(root), root, image => {
            if (fail) throw new InvalidOperationException("Clipboard unavailable");
            if (image.Size != new Size(12, 10) || image.GetPixel(0, 0).ToArgb() != Color.Coral.ToArgb()) throw new Exception("confirmation copied the wrong selection");
            copies++;
        });
        using var image = new Bitmap(12, 10);
        using (var graphics = Graphics.FromImage(image)) graphics.Clear(Color.Coral);
        var draft = operations.PrepareEditor(image, settings);
        try { operations.ConfirmDraft(draft.Id, draft.Revision); throw new Exception("failed copy was reported as completed"); }
        catch (InvalidOperationException error) when (error.Message == "Clipboard unavailable") { }
        if (operations.GetDraft()?.Id != draft.Id) throw new Exception("copy failure lost the screenshot");
        fail = false;
        operations.ConfirmDraft(draft.Id, draft.Revision);
        if (copies != 1 || operations.GetDraft() is not null || new CaptureDraftStore(root).Read() is not null) throw new Exception("confirmed screenshot remained open or copied repeatedly");
        if (Directory.Exists(output) || operations.Last is not null) throw new Exception("confirmation implicitly saved a file");
    }
}
