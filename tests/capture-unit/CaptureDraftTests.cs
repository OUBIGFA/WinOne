using WinOne.Capture;

internal static class CaptureDraftTests
{
    public static void Run(string root)
    {
        var directory = Path.Combine(root, "drafts-case");
        Directory.CreateDirectory(directory);
        var lastPath = Path.Combine(directory, "last-capture.json");
        File.WriteAllText(lastPath, "previous capture");
        var settings = new CaptureSettings { OutputDirectory = directory, CopyToClipboard = false, OpenEditor = true };
        using var image = new Bitmap(48, 32);
        using (var graphics = Graphics.FromImage(image)) graphics.Clear(Color.SlateBlue);
        var store = new CaptureDraftStore(directory);
        var first = store.Begin(image, settings);
        if (first.Revision != 0 || first.Width != 48 || !first.PreviewReady || first.Document.Marks.Count != 0) throw new Exception("draft initialization failed");
        using (var loaded = store.Load(first.Id))
            if (loaded.GetPixel(3, 3).ToArgb() != Color.SlateBlue.ToArgb()) throw new Exception("draft source pixels changed");
        try { store.Begin(image, settings); throw new Exception("active draft was overwritten"); }
        catch (InvalidOperationException) { }
        var document = first.Document with { Marks = [new AnnotationMark { Kind = "cover", X = 2, Y = 3, Width = 10, Height = 8, Color = "#000000" }] };
        var next = store.Update(first.Id, 0, document);
        var restarted = new CaptureDraftStore(directory);
        if (next.Revision != 1 || restarted.Read()?.Document.Marks[0].Kind != "cover") throw new Exception("draft did not survive restart");
        try { restarted.Update(first.Id, 0, document); throw new Exception("stale draft revision accepted"); }
        catch (InvalidOperationException) { }
        try { restarted.Update(first.Id, 1, document with { Width = 49 }); throw new Exception("invalid draft dimensions accepted"); }
        catch (InvalidDataException) { }
        try { restarted.Cancel(first.Id, 0); throw new Exception("stale cancellation accepted"); }
        catch (InvalidOperationException) { }
        var largeMarks = Enumerable.Range(0, 60).Select(_ => new AnnotationMark { Kind = "text", X = 1, Y = 1, Width = 20, Height = 20, Text = new string('字', 3000) }).ToArray();
        try { restarted.Update(first.Id, 1, document with { Marks = largeMarks }); throw new Exception("oversized draft accepted"); }
        catch (InvalidDataException) { }
        if (restarted.Read()?.Revision != 1) throw new Exception("failed update replaced the valid draft");
        var metadataPath = Path.Combine(directory, "capture-draft.json");
        var attributes = File.GetAttributes(metadataPath);
        File.SetAttributes(metadataPath, attributes | FileAttributes.ReadOnly);
        try
        {
            try { restarted.Update(first.Id, 1, document); throw new Exception("read-only draft was overwritten"); }
            catch (UnauthorizedAccessException) { }
            catch (IOException) { }
        }
        finally { File.SetAttributes(metadataPath, attributes); }
        if (restarted.Read()?.Revision != 1) throw new Exception("failed disk write advanced draft revision");
        try { restarted.Cancel(Guid.NewGuid().ToString("N"), 1); throw new Exception("another draft was cancelled"); }
        catch (InvalidOperationException) { }
        restarted.Cancel(first.Id, 1);
        if (restarted.Read() is not null || File.ReadAllText(lastPath) != "previous capture") throw new Exception("draft cancellation changed the previous capture");
        if (Directory.GetFiles(Path.Combine(directory, "drafts")).Length != 0) throw new Exception("cancelled draft image was retained");

        var operationsDirectory = Path.Combine(root, "draft-complete");
        var operations = new CaptureOperations(new SettingsStore(operationsDirectory), operationsDirectory);
        var exportSettings = settings with { OutputDirectory = operationsDirectory };
        var previous = operations.Export(image, exportSettings, CancellationToken.None);
        var pending = operations.PrepareEditor(image, exportSettings);
        if (operations.Last?.Id != previous.Id) throw new Exception("draft replaced the previous capture");
        var changed = operations.UpdateDraft(pending.Id, 0, document);
        try { operations.CopyDraft(pending.Id, 0); throw new Exception("stale clipboard action accepted"); }
        catch (InvalidOperationException) { }
        try { operations.PinDraft(pending.Id, 0); throw new Exception("stale pin action accepted"); }
        catch (InvalidOperationException) { }
        using (var result = operations.RenderDraft(changed))
            if (result.GetPixel(4, 4).ToArgb() != Color.Black.ToArgb()) throw new Exception("draft actions did not render annotations");
        var preview = operations.PreviewAnnotations(pending.Id, changed.Document);
        if (preview.SourceId != pending.Id || operations.Last?.Id != previous.Id) throw new Exception("draft preview changed previous capture");
        var completed = operations.CompleteDraft(pending.Id, changed.Revision, null, CancellationToken.None);
        using var exported = new Bitmap(completed.Path);
        if (exported.GetPixel(4, 4).ToArgb() != Color.Black.ToArgb() || operations.GetDraft() is not null || !File.Exists(previous.Path)) throw new Exception("draft completion lost pixels or previous output");
        var manual = operations.PrepareLastDraft(completed.Id);
        var manualDraft = operations.GetDraft(manual.DraftId)!;
        if (!manualDraft.Settings.AskWhereToSave || manualDraft.Settings.CopyToClipboard || manualDraft.Settings.PinAfterCapture) throw new Exception("manual edit repeated capture tasks");
        try { operations.CompleteDraft(manual.DraftId, 0, null, CancellationToken.None); throw new Exception("manual edit opened an implicit save dialog"); }
        catch (InvalidDataException) { }
        operations.CancelDraft(manual.DraftId, 0);

        // The recoverable source is the full immutable frame, but every action
        // must render only the selected area, including after expanding it.
        var selectionDirectory = Path.Combine(root, "selection-draft");
        var selectionOperations = new CaptureOperations(new SettingsStore(selectionDirectory), selectionDirectory);
        var pixels = new Bitmap(120, 80);
        using (var graphics = Graphics.FromImage(pixels)) graphics.Clear(Color.SlateBlue);
        pixels.SetPixel(10, 10, Color.Coral);
        using var frame = CapturedFrame.Own(pixels, new Rectangle(-40, 20, 120, 80));
        var selected = selectionOperations.PrepareSelectionEditor(frame, new Rectangle(-30, 30, 40, 30), settings with { OutputDirectory = selectionDirectory });
        if (!selected.SelectionRequired || selected.Document.Crop != new AnnotationCrop(10, 10, 40, 30) || selected.Width != 120) throw new Exception("selection handoff lost source or crop");
        using (var rendered = selectionOperations.RenderDraft(selected))
            if (rendered.Size != new Size(40, 30) || rendered.GetPixel(0, 0).ToArgb() != Color.Coral.ToArgb()) throw new Exception("initial crop output included the desktop");
        try { selectionOperations.UpdateDraft(selected.Id, 0, selected.Document with { Crop = null }); throw new Exception("required crop was cleared"); }
        catch (InvalidDataException) { }
        var expanded = selectionOperations.UpdateDraft(selected.Id, 0, selected.Document with { Crop = new AnnotationCrop(5, 5, 50, 40) });
        using (var rendered = selectionOperations.RenderDraft(expanded))
            if (rendered.Size != new Size(50, 40) || rendered.GetPixel(5, 5).ToArgb() != Color.Coral.ToArgb()) throw new Exception("expanded crop changed source coordinates");
        var savedSelection = selectionOperations.CompleteDraft(expanded.Id, expanded.Revision, null, CancellationToken.None);
        using (var saved = new Bitmap(savedSelection.Path))
            if (saved.Size != new Size(50, 40) || saved.GetPixel(5, 5).ToArgb() != Color.Coral.ToArgb()) throw new Exception("save did not use the selected crop");
        if (Directory.GetFiles(Path.Combine(selectionDirectory, "drafts")).Length != 0) throw new Exception("completed selection retained the desktop frame");
        var stoppedDraft = new CaptureDraftStore(Path.Combine(root, "scroll-notice"));
        var stopped = stoppedDraft.Begin(image, settings, notice: "scrollStopped");
        if (new CaptureDraftStore(Path.Combine(root, "scroll-notice")).Read()?.Notice != "scrollStopped") throw new Exception("partial capture notice did not survive restart");
        stoppedDraft.Cancel(stopped.Id, stopped.Revision);
        if (selectionOperations.StopScrolling("old-job")) throw new Exception("inactive scrolling stop was accepted");
    }
}
