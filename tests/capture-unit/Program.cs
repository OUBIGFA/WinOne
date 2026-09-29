using WinOne.Capture;

static void Check(bool condition, string message) { if (!condition) throw new Exception(message); }
if (args.Length is < 1 or > 2) return 2;
var directory = Path.Combine(Path.GetFullPath(args[0]), Guid.NewGuid().ToString("N"));
Directory.CreateDirectory(directory);
try
{
    var store = new SettingsStore(directory);
    var first = store.Snapshot();
    var saved = store.Save(first with { CopyToClipboard = false });
    Check(saved.Revision == 1, "revision did not advance");
    Check(!new SettingsStore(directory).Snapshot().CopyToClipboard, "settings were not persisted");
    try { store.Save(first); throw new Exception("stale settings were accepted"); }
    catch (InvalidOperationException) { }
    File.WriteAllText(Path.Combine(directory, "settings.json"), "broken");
    try { _ = new SettingsStore(directory); throw new Exception("corrupt settings were accepted"); }
    catch (System.Text.Json.JsonException) { }
    Check(File.ReadAllText(Path.Combine(directory, "settings.json")) == "broken", "corrupt settings were overwritten");
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
    using (var image = new Bitmap(32, 24))
    {
        using (var drawing = Graphics.FromImage(image)) drawing.Clear(Color.FromArgb(120, 77, 88, 99));
        var output = operations.Export(image, exportSettings, CancellationToken.None);
        using var decoded = new Bitmap(output.Path);
        Check(decoded.Size == new Size(32, 24) && decoded.GetPixel(3, 3).ToArgb() == image.GetPixel(3, 3).ToArgb(), "PNG export changed pixels");
        Check(new CaptureOperations(exportStore, exportDirectory).Last?.Path == output.Path, "last capture did not survive worker restart");
    }
    if (args.Length == 2 && args[1] == "--native")
    {
        Exception? failure = null;
        var thread = new Thread(() =>
        {
            try
            {
                Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
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
                        using var cancelTimer = new System.Threading.Timer(_ => editor.BeginInvoke((Action)editor.Close), null, 300, Timeout.Infinite);
                        editor.ShowDialog();
                        Check(editor.Result is not (ShareX.ScreenCaptureLib.RegionResult.AnnotateContinueTask or ShareX.ScreenCaptureLib.RegionResult.AnnotateRunAfterCaptureTasks), "closing editor committed an edit");
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
        Console.WriteLine("PASS: native pin creation, assets, and close lifecycle");
    }
    Console.WriteLine("PASS: capture settings persistence, conflicts, corruption, and protocol bounds");
    return 0;
}
finally { Directory.Delete(directory, true); }
