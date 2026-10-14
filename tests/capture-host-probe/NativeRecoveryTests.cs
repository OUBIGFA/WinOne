using System.Runtime.InteropServices;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;

internal static class NativeRecoveryTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    public static void Run(string directory, Rectangle bounds)
    {
        Directory.CreateDirectory(directory);
        var settings = new SettingsStore(directory);
        settings.Save(settings.Snapshot() with { OpenEditor = true, OutputDirectory = Path.Combine(directory, "exports") });
        var copies = 0;
        var operations = new CaptureOperations(settings, directory, _ => copies++);
        var drafts = new NativeCaptureDraftStore(directory);
        var reads = 0;
        CapturedFrame Acquire() { reads++; return CapturedFrame.Own(new Bitmap(bounds.Width, bounds.Height), bounds); }
        void Show(WinOneCaptureCanvas canvas, NativeCaptureSession session, Action action)
        {
            var acted = false; Exception? failure = null;
            using var timeout = new System.Windows.Forms.Timer { Interval = 10000 };
            timeout.Tick += (_, _) => { failure = new TimeoutException("Recovery interaction timed out"); canvas.Window.Close(); };
            session.Updated += () => {
                if (acted || !session.Ready) return;
                acted = true;
                canvas.Window.BeginInvoke((Action)(() => {
                    try { action(); } catch (Exception error) { failure = error; canvas.Window.Close(); }
                }));
            };
            timeout.Start(); canvas.ShowDialog(); timeout.Stop();
            if (failure is not null) throw failure;
        }
        using var stopping = new CancellationTokenSource();
        try {
            NativeCaptureWorkflow.Run(settings, operations, "region", null, stopping.Token, Acquire,
                show: (canvas, session) => Show(canvas, session, () => {
                    canvas.SetRegion(new Rectangle(100, 100, 400, 240));
                    canvas.AddRectangle(new Rectangle(120, 120, 100, 80));
                    session.RestoreText(new NativeTextInputState(null, "重启继续编辑", 140, 150, 240, 50, 190, 24, Color.Red.ToArgb(), 2, 2));
                    stopping.Cancel();
                }), drafts: drafts).GetAwaiter().GetResult();
            throw new Exception("Interrupted capture reported completion");
        } catch (OperationCanceledException) { }
        var preserved = new NativeCaptureDraftStore(directory).Read() ?? throw new Exception("Interrupted capture did not preserve a draft");
        if (preserved.Text?.Value != "重启继续编辑" || preserved.Document.Layers.Length != 1 || copies != 0)
            throw new Exception("Shutdown lost live text or drawing layers");
        NativeCaptureWorkflow.Run(settings, operations, "window", null, CancellationToken.None,
            () => throw new Exception("Recovery acquired a new desktop frame"),
            show: (canvas, session) => Show(canvas, session, () => {
                if (canvas.AnnotationCount != 1 || session.TextInput?.Text != "重启继续编辑" || session.TextInput.SelectionStart != 2)
                    throw new Exception("Recovered session did not restore inline editing");
                SendMessageW(canvas.Window.Handle, 0x0203, IntPtr.Zero, IntPtr.Zero);
            }), drafts: new NativeCaptureDraftStore(directory)).GetAwaiter().GetResult();
        if (copies != 1 || drafts.Read() is not null || drafts.ReadReservation() is not null || reads != 1)
            throw new Exception("Recovery confirmation duplicated output or retained a completed draft");
        try {
            NativeCaptureWorkflow.Run(settings, operations, "region", null, CancellationToken.None, Acquire,
                show: (canvas, session) => Show(canvas, session, () => SendMessageW(canvas.Window.Handle, 0x0205, IntPtr.Zero, IntPtr.Zero)), drafts: drafts).GetAwaiter().GetResult();
            throw new Exception("Right-click cancellation reported success");
        } catch (OperationCanceledException) { }
        if (drafts.Read() is not null || drafts.ReadReservation() is not null || copies != 1)
            throw new Exception("Right-click cancellation retained a draft or copied pixels");
        VerifyCloseRetry(Path.Combine(directory, "close-retry"), bounds);
        VerifyIncompleteChoice(Path.Combine(directory, "incomplete-choice"));
        NativeViewportSessionTests.Run();
        VerifyChangedDisplay(Path.Combine(directory, "changed-display"), bounds);
        Console.WriteLine("PASS: native workflow preserves live edits, resumes without recapture, confirms once and retires cancellation");
    }

    private static void VerifyCloseRetry(string directory, Rectangle bounds)
    {
        Directory.CreateDirectory(directory);
        var settings = new SettingsStore(directory);
        using var image = new Bitmap(bounds.Width, bounds.Height);
        using var source = new WinOneCaptureCanvas(image);
        source.SetRegion(new Rectangle(100, 100, 300, 200));
        var store = new NativeCaptureDraftStore(directory);
        store.Begin(image, settings.Snapshot(), source.ExportDocument(), "region", CapturePlacement.From(bounds, bounds));
        var ledger = Path.Combine(directory, "native-capture", "retired.json");
        File.WriteAllText(ledger, "[]");
        File.SetAttributes(ledger, FileAttributes.ReadOnly);
        var copies = 0; var retried = false;
        var operations = new CaptureOperations(settings, directory, _ => copies++);
        try {
            NativeCaptureWorkflow.Run(settings, operations, "region", null, CancellationToken.None,
                () => throw new Exception("Close retry recaptured the desktop"), show: (canvas, session) => {
                    var started = false; Exception? failure = null;
                    using var timeout = new System.Windows.Forms.Timer { Interval = 10000 };
                    timeout.Tick += (_, _) => { failure = new TimeoutException("Close retry timed out"); File.SetAttributes(ledger, FileAttributes.Normal); canvas.Window.Close(); };
                    session.Updated += () => {
                        if (!started && session.Ready) {
                            started = true;
                            canvas.Window.BeginInvoke((Action)(() => SendMessageW(canvas.Window.Handle, 0x0203, IntPtr.Zero, IntPtr.Zero)));
                        }
                        if (session.Error is null || retried) return;
                        retried = true;
                        if (!canvas.Window.Visible || session.Confirmed || !session.Copied || copies != 1)
                            failure = new Exception("Failed retirement hid the screenshot or lost output status");
                        File.SetAttributes(ledger, FileAttributes.Normal);
                        canvas.Window.BeginInvoke((Action)(() => SendMessageW(canvas.Window.Handle, 0x0203, IntPtr.Zero, IntPtr.Zero)));
                    };
                    timeout.Start(); canvas.ShowDialog(); timeout.Stop();
                    if (failure is not null) throw failure;
                }, drafts: store).GetAwaiter().GetResult();
        } finally { if (File.Exists(ledger)) File.SetAttributes(ledger, FileAttributes.Normal); }
        if (!retried || copies != 1 || store.Read() is not null) throw new Exception("Close retry duplicated output or retained the completed draft");
        Console.WriteLine("PASS: retirement failure keeps the same canvas open; retry closes without copying again");
    }

    private static void VerifyIncompleteChoice(string directory)
    {
        using var image = new Bitmap(100, 80);
        using var canvas = new WinOneCaptureCanvas(image);
        var store = new NativeCaptureDraftStore(directory);
        var ticket = store.Reserve(image.Size, new CaptureSettings(), canvas.ExportDocument(), "region");
        bool Dialog(string language, int choice) => NativeCaptureRecovery.ConfirmRestart(language, page => {
            if (page.Buttons.Count != 2 || page.DefaultButton != page.Buttons[1] || !page.AllowCancel)
                throw new Exception("Recovery prompt lacks a safe default and cancel path");
            page.Created += (_, _) => page.Buttons[choice].PerformClick();
        });
        try { NativeCaptureRecovery.Read(store, "zh-CN", CancellationToken.None, () => Dialog("zh-CN", 1)); throw new Exception("Deferring recovery started a new capture"); }
        catch (OperationCanceledException) { }
        if (store.ReadReservation()?.Id != ticket.Id) throw new Exception("Deferring recovery removed the incomplete draft");
        if (NativeCaptureRecovery.Read(store, "en-US", CancellationToken.None, () => Dialog("en-US", 0)) is not null || store.ReadReservation() is not null)
            throw new Exception("Explicit restart did not release the incomplete generation");
        var next = store.Begin(image, new CaptureSettings(), canvas.ExportDocument(), "region");
        store.Retire(next.Id, next.Revision);
        Console.WriteLine("PASS: incomplete recovery offers native restart/defer choices and preserves deferred data");
    }

    private static void VerifyChangedDisplay(string directory, Rectangle currentBounds)
    {
        foreach (var size in new[] { new Size(800, 600), new Size(currentBounds.Width + 300, currentBounds.Height + 200) }) {
            var profile = Path.Combine(directory, $"{size.Width}x{size.Height}");
            Directory.CreateDirectory(profile);
            var settings = new SettingsStore(profile);
            var store = new NativeCaptureDraftStore(profile);
            using var image = new Bitmap(size.Width, size.Height);
            using var source = new WinOneCaptureCanvas(image);
            source.SetRegion(new Rectangle(50, 50, 700, 500));
            source.AddRectangle(new Rectangle(100, 100, 80, 60));
            var oldBounds = new Rectangle(-1920, -200, size.Width, size.Height);
            var draft = store.Begin(image, settings.Snapshot(), source.ExportDocument(), "region", CapturePlacement.From(oldBounds, oldBounds));
            store.Update(draft.Id, draft.Revision, draft.Document,
                new NativeTextInputState(null, "跨显示布局恢复", 100, 220, 500, 80, 330, 24, Color.Red.ToArgb(), 1, 2));
            var copies = 0;
            var operations = new CaptureOperations(settings, profile, output => {
                if (output.Size != new Size(700, 500)) throw new Exception("Changed-display recovery resized output pixels");
                copies++;
            });
            NativeCaptureWorkflow.Run(settings, operations, "region", null, CancellationToken.None,
                () => throw new Exception("Changed-display recovery acquired a replacement frame"), show: (canvas, session) => {
                    var started = false; Exception? failure = null;
                    using var timeout = new System.Windows.Forms.Timer { Interval = 10000 };
                    timeout.Tick += (_, _) => { failure = new TimeoutException("Changed-display recovery timed out"); canvas.Window.Close(); };
                    session.Updated += () => {
                        if (started || !session.Ready) return;
                        started = true;
                        canvas.Window.BeginInvoke((Action)(() => {
                            try {
                                if (!canvas.HasViewport || canvas.AnnotationCount != 1 || session.TextInput?.Text != "跨显示布局恢复" ||
                                    session.TextInput.Font.Size != 24 * canvas.ViewScale || !canvas.Window.ClientRectangle.Contains(session.Toolbar.Bounds))
                                    throw new Exception("Changed-display recovery did not restore editable content in a bounded viewport");
                                SendMessageW(canvas.Window.Handle, 0x0203, IntPtr.Zero, IntPtr.Zero);
                            } catch (Exception error) { failure = error; canvas.Window.Close(); }
                        }));
                    };
                    timeout.Start(); canvas.ShowDialog(); timeout.Stop();
                    if (failure is not null) throw failure;
                }, drafts: store).GetAwaiter().GetResult();
            if (copies != 1 || store.Read() is not null) throw new Exception("Changed-display recovery did not finish exactly once");
        }
        Console.WriteLine("PASS: smaller/larger previous displays recover editable layers and inline text at original output size");
    }
}
