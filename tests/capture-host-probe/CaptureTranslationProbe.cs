using System.Diagnostics;
using System.IO.Pipes;
using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class CaptureTranslationProbe
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);

    public static int Run(string output)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal))
            throw new InvalidOperationException("Use a private test desktop");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        Application.EnableVisualStyles();
        Control.CheckForIllegalCrossThreadCalls = true;
        using var deadline = new CancellationTokenSource(TimeSpan.FromSeconds(48));
        using var parent = Process.GetCurrentProcess();
        var pipeName = "CaptureTranslation-" + Guid.NewGuid().ToString("N");
        var owner = new NativeHostConfiguration(pipeName, parent);
        int requests = 0;
        // Exercise the production pipe, OCR, native renderer and screenshot
        // workflow. Only the online service response is deterministic here.
        var server = Task.Run(async () => {
            try {
                while (!deadline.IsCancellationRequested) {
                    using var pipe = new NamedPipeServerStream(pipeName, PipeDirection.InOut, 1, PipeTransmissionMode.Byte,
                        PipeOptions.Asynchronous | PipeOptions.CurrentUserOnly);
                    await pipe.WaitForConnectionAsync(deadline.Token);
                    using var document = JsonDocument.Parse(await PipeProtocol.ReadAsync(pipe, deadline.Token));
                    var request = document.RootElement;
                    object result;
                    switch (request.GetProperty("method").GetString()) {
                        case "ai.translate.target": result = new { target = "zh-CN" }; break;
                        case "ai.translate.start":
                            var text = request.GetProperty("params").GetProperty("text").GetString()!;
                            if (!text.Contains("garden", StringComparison.OrdinalIgnoreCase) || text.Contains("example.org"))
                                throw new Exception("Incorrect source paragraph: " + text);
                            Interlocked.Increment(ref requests);
                            result = new { id = "fixture" }; break;
                        case "ai.jobs.get": result = new { state = "completed", text = "欢迎来到花园。" }; break;
                        case "ai.jobs.cancel": result = new { cancelled = true }; break;
                        default: throw new Exception("Unexpected request, including result-window opening: " + request.GetProperty("method").GetString());
                    }
                    await PipeProtocol.WriteAsync(pipe, JsonSerializer.SerializeToUtf8Bytes(new { id = request.GetProperty("id").GetString(), result }), deadline.Token);
                }
            } catch (OperationCanceledException) when (deadline.IsCancellationRequested) { }
        });
        try {
            var store = new SettingsStore(Path.Combine(output, "data"));
            store.Save(store.Snapshot() with { PinTranslation = true, OpenEditor = true, AskWhereToSave = false });
            Bitmap? copied = null;
            using var copiedImage = new ImageOwner();
            var operations = new CaptureOperations(store, Path.Combine(output, "data"), image => {
                copied = new Bitmap(image); copiedImage.Image = copied;
            }) { NativeConfiguration = owner };
            var bounds = CaptureHelpers.GetScreenBounds();
            var selected = new Rectangle(200, 160, 520, 165);
            var generated = new Bitmap(bounds.Width, bounds.Height);
            using (var g = Graphics.FromImage(generated)) {
                g.Clear(Color.White);
                using var font = new Font("Arial", 20);
                g.DrawString("Welcome to the garden.", font, Brushes.Black, selected.X + 16, selected.Y + 20);
                g.DrawString("https://example.org", font, Brushes.Black, selected.X + 16, selected.Y + 85);
            }
            using var frame = CapturedFrame.Own(generated, bounds);
            using var expected = new ImageOwner();
            NativeCaptureWorkflow.Run(store, operations, "region", "en", deadline.Token, acquire: () => frame,
                show: (canvas, session) => {
                    Exception? failure = null;
                    canvas.Window.Shown += (_, _) => {
                        try {
                            var hwnd = canvas.Window.Handle;
                            void Wait(Func<bool> condition) {
                                while (!condition()) {
                                    deadline.Token.ThrowIfCancellationRequested();
                                    if (server.IsFaulted) server.GetAwaiter().GetResult();
                                    Application.DoEvents(); Thread.Sleep(10);
                                }
                            }
                            Wait(() => session.Ready);
                            canvas.SetRegion(selected); canvas.SelectTool("select"); Application.DoEvents();
                            ToolStripItem Button(string id) => session.Toolbar.Controls.OfType<ToolStrip>()
                                .SelectMany(row => row.Items.Cast<ToolStripItem>()).Single(item => item.Name == id);
                            var action = (ToolStripButton)Button("translate");
                            if (requests != 0) throw new Exception("Selection started a translation");
                            using var original = canvas.RenderSelection();
                            if (!action.Enabled || canvas.Selection != selected) throw new Exception($"Translation action unavailable: enabled={action.Enabled}, selection={canvas.Selection}");
                            action.PerformClick();
                            Wait(() => !session.RecognitionBusy);
                            if (!action.Checked || session.CloseRequested || session.ResultPresented || !canvas.Window.Visible || canvas.Window.Handle != hwnd)
                                throw new Exception($"Translation state checked={action.Checked}, close={session.CloseRequested}, result={session.ResultPresented}, visible={canvas.Window.Visible}, sameHwnd={canvas.Window.Handle == hwnd}, requests={requests}, selection={canvas.Selection}, label={action.Text}, error={session.Error}");
                            if (requests != 1 || ShareX.PinToScreenForm.ActiveCount != 0) throw new Exception("Unexpected request or pin");
                            using var translated = canvas.RenderSelection();
                            if (SamePixels(original, translated)) throw new Exception("Export still contains the original");
                            for (int y = 83; y < 120; y++) for (int x = 15; x < 300; x++)
                                if (original.GetPixel(x, y) != translated.GetPixel(x, y)) throw new Exception("URL was changed");
                            // Render the actual canvas and child toolbar on the private desktop.
                            using (var screen = new Bitmap(canvas.Window.Width, canvas.Window.Height)) {
                                canvas.Window.DrawToBitmap(screen, canvas.Window.ClientRectangle);
                                var viewport = Rectangle.Union(canvas.ImageToView(Rectangle.Inflate(selected, 16, 16)), Rectangle.Inflate(session.Toolbar.Bounds, 8, 8));
                                viewport = Rectangle.Intersect(viewport, canvas.Window.ClientRectangle);
                                using var crop = screen.Clone(viewport, screen.PixelFormat);
                                crop.Save(Path.Combine(output, "capture-translated.png"));
                            }
                            translated.Save(Path.Combine(output, "translated.png"));
                            original.Save(Path.Combine(output, "original.png"));
                            action.PerformClick();
                            using (var comparison = canvas.RenderSelection()) if (!SamePixels(original, comparison)) throw new Exception("Original did not restore");
                            SendMessageW(hwnd, 0x0100, new IntPtr((int)Keys.Tab), IntPtr.Zero);
                            using (var comparison = canvas.RenderSelection()) if (!SamePixels(translated, comparison)) throw new Exception("Tab did not restore translation");
                            if (requests != 1) throw new Exception("Comparison translated again");
                            // Editing invalidates the preview while retaining the editable document.
                            canvas.AddRectangle(new Rectangle(selected.X + 400, selected.Y + 12, 55, 30));
                            if (action.Checked || action.Text != "Translate in place") throw new Exception("Edited image retained an old translation");
                            canvas.Undo();
                            action.PerformClick(); Wait(() => !session.RecognitionBusy);
                            if (!action.Checked || requests != 2) throw new Exception("Could not translate after editing: " + session.Error);
                            expected.Image = canvas.RenderSelection();
                            Button("confirm").PerformClick(); Wait(() => session.CloseRequested);
                        } catch (Exception error) { failure = error; canvas.Window.Close(); }
                    };
                    Application.Run(canvas.Window);
                    if (failure is not null) throw failure;
                }).GetAwaiter().GetResult();
            if (copied is null || expected.Image is null || !SamePixels(copied, expected.Image))
                throw new Exception("Confirm did not copy the visible translation");
            RunLifecycleCases();
            File.WriteAllText(Path.Combine(output, "result.json"), JsonSerializer.Serialize(new {
                requests, sameWindow = true, noResultWindow = true, directToolbar = true, protectedUrl = true,
                originalRestored = true, copiedTranslation = true, savedTranslation = true, pinnedTranslation = true, shortcut = true, editingInvalidates = true,
                failurePreserved = true, lateResultIgnored = true, selectionInvalidates = true, longCapture = true
            }));
            return 0;
        } catch (Exception error) { File.WriteAllText(Path.Combine(output, "failure.txt"), error.ToString()); throw; }
        finally { deadline.Cancel(); server.GetAwaiter().GetResult(); }
    }

    private static void RunLifecycleCases()
    {
        foreach (var mode in new[] { "failure", "late", "selection", "viewport", "save", "pin", "shortcut" }) {
            using var image = new Bitmap(900, 700);
            using (var g = Graphics.FromImage(image)) g.Clear(Color.White);
            using var canvas = new WinOneCaptureCanvas(image, viewport: mode == "viewport");
            var service = new DelayedTranslation();
            var exported = false;
            void CheckExport(Bitmap value) {
                if (value.Size != new Size(300, 200) || value.GetPixel(50, 50).ToArgb() != Color.Red.ToArgb())
                    throw new Exception("Output did not receive the translated image: " + mode);
                exported = true;
            }
            using var session = new NativeCaptureSession(canvas, _ => { }, CheckExport, pin: CheckExport,
                textService: service, translateInPlace: true, textMode: mode == "shortcut" ? "translate" : null);
            Exception? failure = null;
            canvas.Window.Shown += (_, _) => {
                try {
                    void Pump(Func<bool> condition) {
                        var until = Stopwatch.StartNew();
                        while (!condition()) { if (until.ElapsedMilliseconds > 3000) throw new Exception("Lifecycle check timed out: " + mode); Application.DoEvents(); Thread.Sleep(5); }
                    }
                    Pump(() => session.Ready);
                    canvas.SetRegion(new Rectangle(100, 90, 300, 200));
                    if (mode == "viewport") canvas.SetViewport(0.75f, new PointF(36, 44));
                    var translate = session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row => row.Items.Cast<ToolStripItem>()).Single(item => item.Name == "translate");
                    if (mode != "shortcut") translate.PerformClick();
                    Pump(() => service.Calls == 1);
                    if (mode == "failure") service.Result.SetException(new InvalidOperationException("pinRenderFailed"));
                    else {
                        if (mode == "late") canvas.Window.Close();
                        if (mode == "selection") canvas.SetRegion(new Rectangle(450, 250, 300, 200));
                        var translated = new Bitmap(300, 200);
                        using (var g = Graphics.FromImage(translated)) g.Clear(Color.Red);
                        service.Result.SetResult(new PinTranslationResult(translated, "fixture"));
                    }
                    Pump(() => !session.RecognitionBusy);
                    Application.DoEvents();
                    if (mode != "late") {
                        using var actual = canvas.RenderSelection();
                        if (mode is "viewport" or "save" or "pin" or "shortcut") {
                            if (actual.GetPixel(50, 50).ToArgb() != Color.Red.ToArgb()) throw new Exception("Viewport export lost translation");
                            if (mode == "viewport") {
                                using var painted = new Bitmap(canvas.Window.Width, canvas.Window.Height);
                                canvas.Window.DrawToBitmap(painted, canvas.Window.ClientRectangle);
                                var p = canvas.ImageToView(new Rectangle(150, 140, 1, 1)).Location;
                                if (painted.GetPixel(p.X, p.Y).ToArgb() != Color.Red.ToArgb()) throw new Exception("Viewport translation position changed");
                            }
                            if (mode is "save" or "pin") {
                                session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row => row.Items.Cast<ToolStripItem>())
                                    .Single(item => item.Name == mode).PerformClick();
                                Pump(() => session.CloseRequested);
                                if (!exported) throw new Exception("Output action did not run: " + mode);
                            }
                        } else if (actual.GetPixel(50, 50).ToArgb() != Color.White.ToArgb() || session.CloseRequested)
                            throw new Exception("Failed/stale result changed capture: " + mode);
                        if (mode == "failure" && session.Error is null) throw new Exception("Failure was hidden");
                        canvas.Window.Close();
                    }
                } catch (Exception error) { failure = error; canvas.Window.Close(); }
            };
            Application.Run(canvas.Window);
            if (failure is not null) throw failure;
        }
    }

    private static bool SamePixels(Bitmap first, Bitmap second)
    {
        if (first.Size != second.Size) return false;
        for (int y = 0; y < first.Height; y++) for (int x = 0; x < first.Width; x++)
            if (first.GetPixel(x, y) != second.GetPixel(x, y)) return false;
        return true;
    }

    private sealed class ImageOwner : IDisposable
    {
        internal Bitmap? Image;
        public void Dispose() => Image?.Dispose();
    }

    private sealed class DelayedTranslation : ICaptureTextService
    {
        internal int Calls;
        internal readonly TaskCompletionSource<PinTranslationResult> Result = new(TaskCreationOptions.RunContinuationsAsynchronously);
        public Task<PinTranslationResult> TranslateImage(Bitmap image, CancellationToken cancellation) { Calls++; return Result.Task; }
        public Task<string> Recognize(Bitmap image, CancellationToken cancellation) => throw new Exception("Unexpected OCR handoff");
        public Task<string> Translate(string text, CancellationToken cancellation) => throw new Exception("Unexpected text path");
        public Task<string> OpenResult(Bitmap image, bool translate, CancellationToken cancellation) => throw new Exception("Opened result window");
    }
}
