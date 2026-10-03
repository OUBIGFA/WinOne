using WinOne.Capture;

if (args.Length == 2 && args[0] == "--native-output")
{
    NativeCaptureOutputTests.Run(Path.GetFullPath(args[1]));
    return 0;
}

if (args.Length == 2 && args[0] == "--mature-capture-reference")
{
    MatureCaptureReference.Run(Path.GetFullPath(args[1]));
    return 0;
}

if (args.Length == 2 && args[0] == "--isolated-modal-worker")
{
    try {
        ModalHandoffTests.RunWorker(Path.GetFullPath(args[1]));
        File.WriteAllText(Path.Combine(args[1], "result.txt"), "PASS: 8 isolated modal selections, real hold windows, retry, negative origins, draft recovery and exact PNG export");
        return 0;
    } catch (Exception error) {
        File.WriteAllText(Path.Combine(args[1], "result.txt"), error.ToString());
        return 1;
    }
}

if (args.Length == 2 && args[0] == "--isolated-modal-handoff")
{
    ModalHandoffTests.Run(Path.GetFullPath(args[1]));
    return 0;
}

// Used only by the process deadline regression; no production code has this mode.
if ((args.Length is 1 or 2) && args[0] == "-777") { Thread.Sleep(10000); return 0; }
if ((args.Length is 1 or 2) && args[0] == "-778")
{
    var path = Environment.GetEnvironmentVariable("WINONE_REGION_RETRY_TEST_FILE");
    if (string.IsNullOrEmpty(path)) return 2;
    var attempt = File.Exists(path) ? int.Parse(File.ReadAllText(path)) + 1 : 1;
    File.WriteAllText(path, attempt.ToString());
    if (attempt == 1) return 5;
    Console.WriteLine("0 0 100 100"); return 0;
}

if (args.Length == 1 && args[0] == "--native-window-regions")
{
    WindowCaptureRegionTests.Run();
    return 0;
}

if (args.Length == 2 && args[1] == "--text-export-recovery")
{
    TextExportRecoveryTests.Run(Path.GetFullPath(args[0]));
    return 0;
}

if (args.Length == 2 && args[0] == "--region-helper-lifetime")
{
    CaptureHelperLifetimeTests.Run(Path.GetFullPath(args[1]));
    return 0;
}

if (args.Length == 2 && args[1] == "--native-editor-fixture")
{
    EditorFixture.NativeDraft(Path.GetFullPath(args[0]));
    return 0;
}

if (args.Length == 2 && args[1] == "--native-selection-fixture")
{
    EditorFixture.NativeDraft(Path.GetFullPath(args[0]), selecting: true);
    return 0;
}

if (args.Length == 2 && args[1] == "--native-pin-menu")
{
    PinMenuTests.Run(Path.GetFullPath(args[0]));
    return 0;
}

if (args.Length == 2 && args[1] == "--native-scroll-ui")
{
    ScrollingWindowTests.Run(Path.GetFullPath(args[0]));
    return 0;
}

// Explicit browser-fixture mode renders only synthetic images; no desktop capture.
if (args.Length == 2 && args[1] == "--editor-fixture")
{
    EditorFixture.Render(Path.GetFullPath(args[0]));
    return 0;
}

static void Check(bool condition, string message) { if (!condition) throw new Exception(message); }
if (args.Length is < 1 or > 2) return 2;
var directory = Path.Combine(Path.GetFullPath(args[0]), Guid.NewGuid().ToString("N"));
Directory.CreateDirectory(directory);
try
{
    CapturedFrameTests.Run();
    CaptureConfirmationTests.Run(Path.Combine(directory, "confirmation"));
    CaptureSelectionSessionTests.Run(Path.Combine(directory, "single-session"));
    NativeSelectionTests.HandoffMessages();
    WindowRegionRefinementTests.Run(Path.Combine(directory, "region-client")).GetAwaiter().GetResult();
    CaptureHoldTests.Run();
    SelectionGeometryTests.Run();
    PngFileTests.Run(Path.Combine(directory, "unique-png"));
    SaveCopyTests.Run(Path.Combine(directory, "save-copy"));
    TextExportRecoveryTests.Run(Path.Combine(directory, "text-export-recovery"));
    CapturePlacementTests.Run(Path.Combine(directory, "placement"));
    AnnotationTests.Run();
    CaptureDraftTests.Run(directory);
    CaptureDraftCancellationTests.Run(Path.Combine(directory, "draft-cancellation"));
    CaptureDraftReplacementTests.Run(Path.Combine(directory, "draft-replacement"));
    // A one-pixel pattern above the thumbnail limit must survive editing.
    using (var detail = new Bitmap(2001, 2))
    {
        for (var x = 0; x < detail.Width; x++) detail.SetPixel(x, 0, x % 2 == 0 ? Color.Black : Color.White);
        var previews = new CapturePreviews(Path.Combine(directory, "detail"));
        var rendered = previews.WriteEdit(detail, new string('a', 32));
        using var decoded = new Bitmap(Path.Combine(directory, "detail", "previews", rendered.Id + ".png"));
        Check(decoded.Size == detail.Size, "edit raster was downsampled");
        for (var x = 0; x < detail.Width; x++) Check(decoded.GetPixel(x, 0).ToArgb() == detail.GetPixel(x, 0).ToArgb(), "edit raster lost pixel detail");
        previews.Write(detail, new string('b', 32));
        using var thumbnail = new Bitmap(Path.Combine(directory, "detail", "previews", new string('b', 32) + ".png"));
        Check(thumbnail.Width == 1600, "list thumbnail is no longer bounded");
    }
    if (args.Length == 2 && args[1] == "--native-draft-actions") NativeDraftActionTests.RunOnSta(Path.Combine(directory, "native-draft-actions"));
    var priorityBefore = InteractiveWorkScope.ReadState();
    try
    {
        using var priority = new InteractiveWorkScope();
        var active = InteractiveWorkScope.ReadState();
        Check((active.ControlMask & 1) != 0 && (active.StateMask & 1) == 0, "interactive capture did not request HighQoS");
        using (var nested = new InteractiveWorkScope()) { }
        Check(InteractiveWorkScope.ReadState().Equals(active), "nested work lost interactive priority");
        throw new OperationCanceledException();
    }
    catch (OperationCanceledException) { }
    Check(InteractiveWorkScope.ReadState().Equals(priorityBefore), "interactive capture did not restore its original QoS");
    if (args.Length == 2 && args[1] == "--native-selection") NativeSelectionTests.RunOnSta(Path.Combine(Path.GetFullPath(args[0]), "native-selection.png"));
    var store = new SettingsStore(directory);
    var first = store.Snapshot();
    var saved = store.Save(first with { CopyToClipboard = false, ToolbarTools = ["mosaic", "arrow"] });
    Check(saved.Revision == 1, "revision did not advance");
    Check(!new SettingsStore(directory).Snapshot().CopyToClipboard, "settings were not persisted");
    Check(new SettingsStore(directory).Snapshot().ToolbarTools!.SequenceEqual(new[] { "mosaic", "arrow" }), "toolbar preferences were not persisted");
    try { store.Save(saved with { ToolbarTools = ["mosaic", "mosaic"] }); throw new Exception("duplicate toolbar tools accepted"); }
    catch (InvalidDataException) { }
    try { store.Save(first); throw new Exception("stale settings were accepted"); }
    catch (InvalidOperationException) { }
    File.WriteAllText(Path.Combine(directory, "settings.json"), "broken");
    try { _ = new SettingsStore(directory); throw new Exception("corrupt settings were accepted"); }
    catch (System.Text.Json.JsonException) { }
    Check(File.ReadAllText(Path.Combine(directory, "settings.json")) == "broken", "corrupt settings were overwritten");
    var managedDirectory = Path.Combine(directory, "managed");
    Directory.CreateDirectory(managedDirectory);
    var projection = Path.Combine(managedDirectory, "managed-settings.json");
    var managedSettings = new CaptureSettings { Revision = 7, OpenEditor = true };
    File.WriteAllBytes(projection, System.Text.Json.JsonSerializer.SerializeToUtf8Bytes(managedSettings, SettingsStore.Json));
    var managedStore = new SettingsStore(managedDirectory);
    var activeSession = managedStore.Snapshot();
    Check(managedStore.IsManaged && activeSession.Revision == 7, "managed preferences were not loaded");
    try { managedStore.Save(activeSession); throw new Exception("worker wrote managed preferences"); }
    catch (InvalidOperationException) { }
    File.WriteAllBytes(projection, System.Text.Json.JsonSerializer.SerializeToUtf8Bytes(managedSettings with { Revision = 8, OpenEditor = false }, SettingsStore.Json));
    Check(managedStore.ReloadManaged().Revision == 8 && !managedStore.Snapshot().OpenEditor, "managed reload failed");
    Check(activeSession.OpenEditor && activeSession.Revision == 7, "reload changed an active session snapshot");
    File.WriteAllText(projection, "{\"schemaVersion\":1,\"schemaVersion\":2}");
    try { managedStore.ReloadManaged(); throw new Exception("duplicate settings accepted"); }
    catch (System.Text.Json.JsonException) { }
    Check(managedStore.Snapshot().Revision == 8, "invalid reload changed preferences");
    var invalidProjectionDirectory = Path.Combine(directory, "invalid-projection");
    Directory.CreateDirectory(Path.Combine(invalidProjectionDirectory, "managed-settings.json"));
    try { _ = new SettingsStore(invalidProjectionDirectory); throw new Exception("invalid projection fell back to standalone settings"); }
    catch (UnauthorizedAccessException) { }
    catch (IOException) { }
    using var stream = new MemoryStream();
    await PipeProtocol.WriteAsync(stream, "窗口"u8.ToArray(), CancellationToken.None);
    stream.Position = 0;
    Check(System.Text.Encoding.UTF8.GetString(await PipeProtocol.ReadAsync(stream, CancellationToken.None)) == "窗口", "UTF-8 frame roundtrip failed");
    using var oversized = new MemoryStream(new byte[] { 1, 0, 16, 0 });
    try { await PipeProtocol.ReadAsync(oversized, CancellationToken.None); throw new Exception("oversized frame was accepted"); }
    catch (InvalidDataException) { }
    var exportDirectory = Path.Combine(directory, "export");
    var exportStore = new SettingsStore(exportDirectory);
    var exportSettings = exportStore.Save(exportStore.Snapshot() with { OutputDirectory = Path.Combine(exportDirectory, "images"), CopyToClipboard = false });
    var operations = new CaptureOperations(exportStore, exportDirectory);
    using (var large = new Bitmap(2000, 1000))
    {
        using (var drawing = Graphics.FromImage(large)) drawing.Clear(Color.Coral);
        var result = operations.Export(large, exportSettings, CancellationToken.None);
        Check(result.PreviewReady, "capture preview was not created");
        using var preview = new Bitmap(Path.Combine(exportDirectory, "previews", result.Id + ".png"));
        Check(preview.Size == new Size(1600, 800), "preview size was not bounded");
        Check(preview.GetPixel(800, 400).ToArgb() == Color.Coral.ToArgb(), "preview changed a flat color");
    }
    using (var image = new Bitmap(32, 24))
    {
        using (var drawing = Graphics.FromImage(image)) drawing.Clear(Color.FromArgb(120, 77, 88, 99));
        var output = operations.Export(image, exportSettings, CancellationToken.None);
        using var decoded = new Bitmap(output.Path);
        Check(decoded.Size == new Size(32, 24) && decoded.GetPixel(3, 3).ToArgb() == image.GetPixel(3, 3).ToArgb(), "PNG export changed pixels");
        Check(new CaptureOperations(exportStore, exportDirectory).Last?.Path == output.Path, "last capture did not survive worker restart");
        var copyPath = Path.Combine(exportDirectory, "saved-copy.png");
        var copy = operations.SaveLastAs(copyPath);
        using var savedCopy = new Bitmap(copy.Path);
        Check(savedCopy.Size == image.Size && savedCopy.GetPixel(3, 3).ToArgb() == image.GetPixel(3, 3).ToArgb(), "save-as changed image pixels");
        Check(File.Exists(output.Path) && operations.Last?.Path == copyPath, "save-as lost the original or latest result");
        try { operations.SaveLastAs(Path.Combine(exportDirectory, "invalid.jpg")); throw new Exception("non-PNG save-as was accepted"); }
        catch (InvalidOperationException) { }
        Check(operations.Last?.Path == copyPath, "failed save-as changed the latest result");
        try { operations.SaveLastAs(Path.Combine(exportDirectory, "stale.png"), output.Id); throw new Exception("save-as used a newer capture than the reviewed one"); }
        catch (InvalidOperationException) { }
        Check(!File.Exists(Path.Combine(exportDirectory, "stale.png")), "stale capture request wrote a file");
        var editedPath = Path.Combine(exportDirectory, "edited.png");
        var previewDocument = new AnnotationDocument { Width = 32, Height = 24, Marks = [new AnnotationMark { Kind = "cover", X = 1, Y = 1, Width = 5, Height = 5, Color = "#000000" }] };
        var firstPreview = operations.PreviewAnnotations(copy.Id, previewDocument);
        using (var previewImage = new Bitmap(Path.Combine(exportDirectory, "previews", firstPreview.Id + ".png")))
            Check(previewImage.GetPixel(3, 3).ToArgb() == Color.Black.ToArgb(), "edit preview omitted native rendering");
        Check(operations.Last?.Id == copy.Id && firstPreview.SourceId == copy.Id, "preview changed the current capture");
        var secondPreview = operations.PreviewAnnotations(copy.Id, previewDocument with { Version = 2, Crop = new AnnotationCrop(1, 1, 5, 5) });
        Check(secondPreview.Width == 5 && secondPreview.Height == 5 && !File.Exists(Path.Combine(exportDirectory, "previews", firstPreview.Id + ".png")), "edit preview did not crop or retire the older result");
        try { operations.PreviewAnnotations(output.Id, previewDocument); throw new Exception("stale edit preview accepted"); }
        catch (InvalidOperationException) { }
        Check(File.Exists(Path.Combine(exportDirectory, "previews", secondPreview.Id + ".png")), "failed preview discarded the last valid result");
        var edited = operations.ApplyAnnotations(editedPath, copy.Id, new AnnotationDocument { Width = 32, Height = 24, Marks = [new AnnotationMark { Kind = "cover", X = 1, Y = 1, Width = 5, Height = 5, Color = "#000000" }] });
        using var editedImage = new Bitmap(edited.Path);
        Check(editedImage.Size == image.Size && editedImage.GetPixel(3, 3).ToArgb() == Color.Black.ToArgb(), "annotation export did not use original image pixels");
        Check(File.Exists(copyPath) && edited.Id != copy.Id, "annotation export lost the source identity");
    }
    if (args.Length == 2 && args[1] == "--native")
    {
        Exception? failure = null;
        var thread = new Thread(() =>
        {
            try
            {
                Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
                using (var scrolling = new ScrollingWindow(exportSettings))
                {
                    var source = new Bitmap(20, 30);
                    source.SetPixel(2, 3, Color.Coral);
                    scrolling.OwnPreview(source);
                    var replacement = new Bitmap(12, 18);
                    scrolling.OwnPreview(replacement);
                    try { _ = source.GetPixel(2, 3); throw new Exception("replaced preview retained its bitmap"); }
                    catch (ArgumentException) { }
                    using var latest = scrolling.TakeResult();
                    Check(ReferenceEquals(latest, replacement) && latest?.Size == new Size(12, 18), "scroll retry did not transfer preview");
                }
                var image = new Bitmap(64, 64);
                using (var drawing = Graphics.FromImage(image)) drawing.Clear(Color.SlateGray);
                ShareX.PinToScreenForm.OpenManaged(image, new ShareX.PinToScreenOptions { Shadow = false });
                using var timer = new System.Windows.Forms.Timer { Interval = 300 };
                timer.Tick += (_, _) =>
                {
                    timer.Stop();
                    try {
                        Check(ShareX.PinToScreenForm.ActiveCount == 1, "pin was not registered"); ShareX.PinToScreenForm.CloseAll(); Check(ShareX.PinToScreenForm.ActiveCount == 0, "pin did not close");
                        using var editor = new ShareX.ScreenCaptureLib.RegionCaptureForm(ShareX.ScreenCaptureLib.RegionCaptureMode.TaskEditor, new ShareX.ScreenCaptureLib.RegionCaptureOptions { EnableSharing = false }, new Bitmap(64, 64));
                        // Upstream continuously invalidates its canvas, which can starve WM_TIMER.
                        // Host cancellation uses a posted UI callback, so exercise that same path.
                        editor.TryCopyImageRequested += _ => false;
                        using var cancelTimer = new System.Threading.Timer(_ => editor.BeginInvoke(() =>
                        {
                            try
                            {
                                var flags = System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.Public | System.Reflection.BindingFlags.NonPublic;
                                var shapes = editor.GetType().GetProperty("ShapeManager", flags)?.GetValue(editor) ?? throw new Exception("editor shapes unavailable");
                                var modified = shapes.GetType().GetProperty("IsImageModified", flags) ?? throw new Exception("editor modification state unavailable");
                                modified.SetValue(shapes, true);
                                editor.GetType().GetMethod("OnCopyImageRequested", flags, Type.EmptyTypes)!.Invoke(editor, null);
                                Check(editor.Visible && editor.IsImageModified, "failed copy closed the editor or cleared unsaved changes");
                                modified.SetValue(shapes, false);
                            }
                            catch (Exception error) { failure = error; }
                            finally { editor.Close(); }
                        }), null, 300, Timeout.Infinite);
                        editor.ShowDialog();
                        Check(editor.Result is not (ShareX.ScreenCaptureLib.RegionResult.AnnotateContinueTask or ShareX.ScreenCaptureLib.RegionResult.AnnotateRunAfterCaptureTasks), "closing editor committed an edit");
                        using var dispatcher = new Form { ShowInTaskbar = false };
                        _ = dispatcher.Handle;
                        var editedPath = Path.Combine(exportDirectory, "editor-save.png");
                        // Exercise the real editor's Save callback without a file-picker interaction.
                        using var saveTimer = new System.Threading.Timer(_ => dispatcher.BeginInvoke(() =>
                        {
                            var active = Application.OpenForms.OfType<ShareX.ScreenCaptureLib.RegionCaptureForm>().Single();
                            try
                            {
                                active.ImageFilePath = editedPath;
                                var save = active.GetType().GetMethod("OnSaveImageRequested", System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.NonPublic)
                                    ?? throw new Exception("editor Save action unavailable");
                                save.Invoke(active, null);
                            }
                            catch (Exception error) { failure = error; }
                            finally { active.Close(); }
                        }), null, 300, Timeout.Infinite);
                        var edited = (CaptureOutput)operations.EditLast(CancellationToken.None);
                        Check(edited.Path == editedPath && File.Exists(editedPath), "save then close lost the committed editor result");
                        Check(new CaptureOperations(exportStore, exportDirectory).Last?.Path == editedPath, "editor save did not persist recent capture");
                    }
                    catch (Exception error) { failure = error; }
                    finally { Application.ExitThread(); }
                };
                timer.Start(); Application.Run();
            }
            catch (Exception error) { failure = error; }
        }) { IsBackground = true };
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        if (!thread.Join(10_000)) throw new Exception("native pin/editor verification timed out");
        if (failure is not null) throw failure;
        Console.WriteLine("PASS: native pins, editor cancel/save, recent capture persistence, and rejected-copy state");
    }
    Console.WriteLine("PASS: capture settings persistence, conflicts, corruption, and protocol bounds");
    return 0;
}
finally { Directory.Delete(directory, true); }
