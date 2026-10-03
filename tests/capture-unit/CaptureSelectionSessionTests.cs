using WinOne.Capture;

internal static class CaptureSelectionSessionTests
{
    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        var copies = 0;
        var operations = new CaptureOperations(new SettingsStore(directory), directory, _ => copies++);
        using var frame = CapturedFrame.Own(new Bitmap(120, 80), new Rectangle(-120, -80, 120, 80));
        var draft = operations.PreparePendingSelection(frame, "window", new CaptureSettings { OutputDirectory = Path.Combine(directory, "output") });
        if (!draft.SelectionPending || draft.SourceKind != "window") throw new Exception("new screenshot skipped selection");
        try { operations.ConfirmDraft(draft.Id, draft.Revision); throw new Exception("unselected desktop copied"); }
        catch (InvalidOperationException error) when (error.Message == "请先选择截图区域") { }
        if (copies != 0) throw new Exception("pending screenshot touched clipboard");
        var selected = operations.SelectDraft(draft.Id, draft.Revision, new AnnotationCrop(10, 20, 40, 30));
        if (selected.Id != draft.Id || selected.SelectionPending || selected.Revision != 1 || selected.Document.Crop != new AnnotationCrop(10, 20, 40, 30)) throw new Exception("selection replaced the session or used incorrect coordinates");
        operations.ConfirmDraft(selected.Id, selected.Revision);
        if (copies != 1 || operations.GetDraft() is not null || Directory.Exists(Path.Combine(directory, "output"))) throw new Exception("confirmation did not finish without saving");
    }
}
