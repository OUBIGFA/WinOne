using WinOne.Capture;

internal static class SaveCopyTests
{
    public static void Run(string directory)
    {
        var settings = new CaptureSettings { OutputDirectory = directory, AskWhereToSave = true, CopyToClipboard = false, OpenEditor = true };
        var copied = 0;
        var operations = new CaptureOperations(new SettingsStore(directory), directory, image => { if (image.Size != new Size(8, 6) || image.GetPixel(1, 1).ToArgb() != Color.Coral.ToArgb()) throw new Exception("wrong copied pixels"); copied++; });
        using var source = new Bitmap(8, 6);
        using (var graphics = Graphics.FromImage(source)) graphics.Clear(Color.Coral);
        var draft = operations.PrepareEditor(source, settings);
        var output = operations.CompleteDraft(draft.Id, draft.Revision, null, CancellationToken.None, saveCopy: true);
        if (!output.Copied || copied != 1 || !File.Exists(output.Path) || operations.GetDraft() is not null) throw new Exception("save-copy did not complete both actions");
        if (settings.CopyToClipboard || !settings.AskWhereToSave) throw new Exception("save-copy changed preferences");
        var failedDirectory = Path.Combine(directory, "copy-failed");
        var failed = new CaptureOperations(new SettingsStore(failedDirectory), failedDirectory, _ => throw new InvalidOperationException("Test clipboard failure"));
        var retained = failed.PrepareEditor(source, settings with { OutputDirectory = failedDirectory });
        try { failed.CompleteDraft(retained.Id, retained.Revision, null, CancellationToken.None, saveCopy: true); throw new Exception("clipboard failure was ignored"); }
        catch (InvalidOperationException error) when (error.Message.Contains("Test clipboard failure")) { }
        if (failed.GetDraft()?.Id != retained.Id || failed.Last is null || !File.Exists(failed.Last.Path)) throw new Exception("partial save lost draft or saved file");
    }
}
