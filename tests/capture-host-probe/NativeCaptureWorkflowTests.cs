using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;

internal static class NativeCaptureWorkflowTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);

    public static int Run(string directory)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal))
            throw new InvalidOperationException("Use a private test desktop");
        if (Directory.Exists(directory)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(directory);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        Application.SetUnhandledExceptionMode(UnhandledExceptionMode.ThrowException);
        NativeSaveCloseTests.Run(Path.Combine(directory,"save-close"));
        NativeLegacyDocumentTests.Run(Path.Combine(directory,"legacy-document"));
        NativeLegacyMigrationTests.Run(Path.Combine(directory,"legacy-migration"));
        NativeLegacyAsyncTests.Run(Path.Combine(directory,"legacy-async"));
        NativeLegacyRenderLifetimeTests.Run(Path.Combine(directory,"legacy-render-lifetime"));
        NativeLegacyEditTests.Run(Path.Combine(directory,"legacy-edit"));
        NativeLegacyWorkflowTests.Run(Path.Combine(directory,"legacy-resume"));
        NativeLastCaptureTests.Run(Path.Combine(directory,"last-capture"));
        var bounds = CaptureHelpers.GetScreenBounds();
        var copies = 0; var reads = 0; var shows = 0;
        CapturedFrame Acquire() { reads++; return CapturedFrame.Own(new Bitmap(bounds.Width, bounds.Height), bounds); }
        var settings = new SettingsStore(directory);
        settings.Save(settings.Snapshot() with { OutputDirectory = Path.Combine(directory, "images"), OpenEditor = false, AskWhereToSave = true, CopyToClipboard = false });
        var operations = new CaptureOperations(settings, directory, _ => copies++, (_, _) => throw new Exception("Quick save opened a picker"));
        var full = (CaptureOutput)NativeCaptureWorkflow.Run(settings, operations, "fullscreen", null, CancellationToken.None, Acquire,
            show: (_, _) => throw new Exception("Quick fullscreen opened an editor")).GetAwaiter().GetResult();
        if (!File.Exists(full.Path) || full.Width != bounds.Width || full.Height != bounds.Height || copies != 0) throw new Exception("Fullscreen output changed");

        object RunSelection(string mode, Action<WinOneCaptureCanvas, NativeCaptureSession> act) => NativeCaptureWorkflow.Run(settings, operations, mode, null,
            CancellationToken.None, Acquire, () => Array.Empty<WindowCaptureTarget>(), (canvas, session) => {
                shows++;
                using var deadline = new System.Windows.Forms.Timer { Interval = 5000 };
                var timedOut = false;
                deadline.Tick += (_, _) => { timedOut = true; canvas.Window.Close(); };
                canvas.Window.Shown += (_, _) => canvas.Window.BeginInvoke((Action)(() => act(canvas, session)));
                deadline.Start(); canvas.ShowDialog(); deadline.Stop();
                if (timedOut) throw new Exception("Native workflow did not finish");
            }).GetAwaiter().GetResult();

        var quick = (CaptureOutput)RunSelection("window", (canvas, session) => {
            canvas.SetRegion(new Rectangle(100, 100, 240, 140));
            if (session.Toolbar.Visible) throw new Exception("Quick save exposed editing controls");
        });
        if (quick.Width != 240 || quick.Height != 140 || !File.Exists(quick.Path)) throw new Exception("Quick selection output changed");

        var blocked = Path.Combine(directory, "blocked-output"); File.WriteAllText(blocked, "test-owned blocker");
        settings.Save(settings.Snapshot() with { OutputDirectory = blocked });
        var sawFailure = false;
        var retried = (CaptureOutput)RunSelection("region", (canvas, session) => {
            session.Updated += () => {
                if (session.Error is null || sawFailure) return;
                sawFailure = true;
                if (session.Confirmed || !session.HintVisible) throw new Exception("Failed quick save did not preserve selection and feedback");
                File.Move(blocked, Path.Combine(directory, "blocked-file"));
                canvas.Window.BeginInvoke((Action)(() => SendMessageW(canvas.Window.Handle, 0x0203, IntPtr.Zero, IntPtr.Zero)));
            };
            canvas.SetRegion(new Rectangle(100, 100, 240, 140));
        });
        if (!sawFailure || !File.Exists(retried.Path) || Directory.GetFiles(blocked, "*.png").Length != 1) throw new Exception("Quick save retry failed");

        settings.Save(settings.Snapshot() with { OpenEditor = true });
        var edited = RunSelection("region", (canvas, session) => {
            canvas.SetRegion(new Rectangle(100, 100, 240, 140));
            SendMessageW(canvas.Window.Handle, 0x0203, IntPtr.Zero, IntPtr.Zero);
        });
        if (copies != 1 || !JsonSerializer.Serialize(edited).Contains("\"copied\":true")) throw new Exception("Edit confirmation did not copy exactly once");
        try { RunSelection("window", (canvas, _) => SendMessageW(canvas.Window.Handle, 0x0205, IntPtr.Zero, IntPtr.Zero)); throw new Exception("Cancel returned success"); }
        catch (OperationCanceledException) { }
        if (copies != 1) throw new Exception("Cancel copied an image");

        var previousReads = reads;
        using var cancelled = new CancellationTokenSource(); cancelled.Cancel();
        try { NativeCaptureWorkflow.Run(settings, operations, "window", null, cancelled.Token, Acquire).GetAwaiter().GetResult(); throw new Exception("Cancelled request acquired a frame"); }
        catch (OperationCanceledException) { }
        if (reads != previousReads) throw new Exception("Cancellation was checked after acquisition");
        using var during = new CancellationTokenSource();
        try {
            NativeCaptureWorkflow.Run(settings, operations, "region", null, during.Token, Acquire, show: (canvas, _) => {
                canvas.Window.Shown += (_, _) => during.Cancel();
                canvas.ShowDialog();
            }).GetAwaiter().GetResult();
            throw new Exception("Active cancellation returned success");
        } catch (OperationCanceledException) { }
        if (copies != 1) throw new Exception("Active cancellation copied an image");
        var snapshots = 0;
        try {
            NativeCaptureWorkflow.Run(settings, operations, "window", null, CancellationToken.None, Acquire,
                () => ++snapshots == 1 ? new[] { new WindowCaptureTarget(new IntPtr(1), bounds) } : Array.Empty<WindowCaptureTarget>(),
                (_, _) => throw new Exception("Changed windows reached the selector")).GetAwaiter().GetResult();
            throw new Exception("Window layout change was accepted");
        } catch (InvalidOperationException error) when (error.Message == "窗口布局已变化，请重新截图") { }
        settings.Save(settings.Snapshot() with { OpenEditor = false, CopyToClipboard = false });
        var originalDirectory = settings.Snapshot().OutputDirectory;
        var frozen = (CaptureOutput)NativeCaptureWorkflow.Run(settings, operations, "fullscreen", null, CancellationToken.None, () => {
            settings.Save(settings.Snapshot() with { OutputDirectory = Path.Combine(directory, "next-session"), CopyToClipboard = true });
            return Acquire();
        }).GetAwaiter().GetResult();
        if (Path.GetDirectoryName(frozen.Path) != originalDirectory || frozen.Copied || copies != 1)
            throw new Exception("Settings changed during acquisition leaked into the current output");
        settings.Save(settings.Snapshot() with { OpenEditor = true });
        NativeCaptureWorkflow.Run(settings, operations, "region", null, CancellationToken.None, Acquire, show: (canvas, session) => {
            var scheduled = false; var timeout = false;
            Exception? interactionFailure = null;
            void Stage(string value) => File.WriteAllText(Path.Combine(directory, "text-style-stage.txt"), value);
            using var deadline = new System.Windows.Forms.Timer { Interval = 5000 };
            deadline.Tick += (_, _) => { timeout = true; canvas.Window.Close(); };
            session.Updated += () => {
                if (!session.Ready || scheduled) return; scheduled = true;
                canvas.Window.BeginInvoke((Action)(() => {
                    try {
                    Stage("selection");
                    canvas.SetRegion(new Rectangle(100, 100, 500, 300));
                    var items = session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(strip => strip.Items.Cast<ToolStripItem>()).ToArray();
                    items.Single(item => item.Name == "text").PerformClick();
                    Stage("text input");
                    SendMessageW(canvas.Window.Handle, 0x0201, new IntPtr(1), new IntPtr((150 << 16) | 140));
                    var input = session.TextInput ?? throw new Exception("Text tool did not open inline input");
                    input.Text = "原位文字 WinOne"; var originalHandle = input.Handle;
                    var options = items.OfType<NativeToolOptions>().Single(); options.ShowDropDown();
                    Stage("font size");
                    var numeric = options.DropDownItems.OfType<ToolStripControlHost>().Select(host => host.Control).OfType<NumericUpDown>().Single();
                    numeric.Value = 36;
                    Stage("text color");
                    options.DropDownItems.OfType<ToolStripMenuItem>().Single(item => item.Tag is Color color && color.ToArgb() == Color.RoyalBlue.ToArgb()).PerformClick();
                    options.HideDropDown();
                    Stage("confirm");
                    if (input.Handle != originalHandle || input.Font.Size != 36 || input.ForeColor.ToArgb() != Color.RoyalBlue.ToArgb() || input.Text != "原位文字 WinOne")
                        throw new Exception("Native style menu did not update the same inline input");
                    SendMessageW(canvas.Window.Handle, 0x0203, IntPtr.Zero, IntPtr.Zero);
                    Stage("confirmed");
                    } catch (Exception error) { interactionFailure = error; File.WriteAllText(Path.Combine(directory, "text-style-error.txt"), error.ToString()); canvas.Window.Close(); }
                }));
            };
            Stage("awaiting native readiness");
            deadline.Start(); canvas.ShowDialog(); deadline.Stop();
            if (timeout) throw new Exception("Inline style session did not finish");
            if (interactionFailure is not null) System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(interactionFailure).Throw();
        }).GetAwaiter().GetResult();
        if (copies != 2) throw new Exception("Styled text was not confirmed exactly once");
        NativeRecoveryTests.Run(Path.Combine(directory, "recovery"), bounds);
        NativeScrollingWorkflowTests.Run(Path.Combine(directory, "scrolling"));
        File.WriteAllText(Path.Combine(directory, "verification.json"), JsonSerializer.Serialize(new { full, quick, retried, sawFailure, copies, reads, shows, realClipboard = false, recovery = true }));
        Console.WriteLine("PASS: native workflow fullscreen, quick save, failure retry, edit confirmation and cancellation");
        return 0;
    }
}
