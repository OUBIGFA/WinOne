using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;

internal static class Program
{
    [System.Runtime.InteropServices.DllImport("user32.dll")]
    private static extern IntPtr WindowFromPoint(Point point);
    [STAThread]
    private static int Main(string[] args)
    {
        if (args.Length != 1 || !Path.IsPathFullyQualified(args[0])) return 2;
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
        Application.EnableVisualStyles();
        using var form = new ScrollDocument();
        using var manager = new ScrollingCaptureManager(new ScrollingCaptureOptions {
            AutoUpload = false, AutoScrollTop = false, ShowRegion = false,
            ScrollMethod = ScrollMethod.ScrollMessage, ScrollAmount = 2, ScrollDelay = 100, StartDelay = 300
        });
        Exception? failure = null;
        form.Shown += async (_, _) =>
        {
            try
            {
                // A target change during the initial delay must stop before the
                // next capture/input, not stitch pixels at the old coordinates.
                manager.SelectTarget(new WindowInfo(form.Handle), form.RectangleToScreen(form.ClientRectangle));
                var originalLocation = form.Location;
                var movedCapture = manager.StartCapture();
                form.Left += 24;
                if (await Task.WhenAny(movedCapture, Task.Delay(3000)) != movedCapture)
                {
                    manager.StopCapture(); await movedCapture;
                    throw new Exception("moved target did not stop promptly");
                }
                try { await movedCapture; throw new Exception("moved target was accepted"); }
                catch (InvalidOperationException error) when (error.Message == "WinOne.ScrollingTargetChanged") { }
                form.Location = originalLocation;
                if (manager.IsCapturing || manager.TakeResult() is not null) throw new Exception("moved target retained an invalid image");

                manager.SelectTarget(new WindowInfo(form.Handle), form.RectangleToScreen(form.ClientRectangle));
                form.OnNextScroll = () => form.Left += 24;
                var interrupted = manager.StartCapture();
                if (await Task.WhenAny(interrupted, Task.Delay(3000)) != interrupted)
                {
                    manager.StopCapture(); await interrupted;
                    throw new Exception("mid-capture movement did not stop promptly");
                }
                try { await interrupted; throw new Exception("mid-capture movement was accepted"); }
                catch (InvalidOperationException error) when (error.Message == "WinOne.ScrollingTargetChanged") { }
                using (var partial = manager.TakeResult()) { }
                form.Location = originalLocation; form.ResetDocument();

                manager.SelectTarget(new WindowInfo(form.Handle), form.RectangleToScreen(form.ClientRectangle));
                manager.Options.ScrollMethod = ScrollMethod.MouseWheel;
                var unfocused = manager.StartCapture();
                using (var other = new Form { Text = "WinOne focus test", ShowInTaskbar = false, TopMost = true })
                {
                    try
                    {
                    other.Show(); other.Activate();
                    // Activation is delivered by the UI message loop; checking
                    // synchronously inside Shown can observe the previous window.
                    await Task.Delay(50);
                    if (NativeMethods.GetForegroundWindow() == form.Handle) throw new Exception("focus test could not move focus away from its target");
                    try { await unfocused; throw new Exception("different foreground window was accepted"); }
                    catch (InvalidOperationException error) when (error.Message == "WinOne.ScrollingTargetChanged") { }
                    }
                    finally
                    {
                        if (!unfocused.IsCompleted)
                        {
                            manager.StopCapture();
                            try { await unfocused; }
                            catch (InvalidOperationException error) when (error.Message == "WinOne.ScrollingTargetChanged") { }
                        }
                    }
                }
                if (manager.TakeResult() is not null) throw new Exception("focus change captured an unrelated window");
                manager.Options.ScrollMethod = ScrollMethod.ScrollMessage;
                // The focus-loss probe can leave another topmost application
                // above this fixture. Re-raise only our own test window before
                // validating screen pixels; never hide or close user windows.
                form.TopMost = false; form.TopMost = true; form.BringToFront(); form.Activate();
                await Task.Delay(100);
                if (WindowFromPoint(form.PointToScreen(new Point(180, 180))) != form.Handle)
                    throw new Exception("scroll fixture is covered by another window; pixel regression cannot run");
                // Use the same explicit target boundary as WinOne's selector.
                manager.SelectTarget(new WindowInfo(form.Handle), form.RectangleToScreen(form.ClientRectangle));
                try { manager.SelectTarget(new WindowInfo(form.Handle), Rectangle.Empty); throw new Exception("empty target accepted"); }
                catch (ArgumentException) { }
                try { manager.SelectTarget(new WindowInfo(form.Handle), new Rectangle(-30000, -30000, 100, 100)); throw new Exception("outside target accepted"); }
                catch (ArgumentException) { }
                using var timeout = new System.Windows.Forms.Timer { Interval = 15000 };
                timeout.Tick += (_, _) => manager.StopCapture(); timeout.Start();
                var capturing = manager.StartCapture();
                try { manager.TakeResult(); throw new Exception("active result detached"); }
                catch (InvalidOperationException) { }
                try { manager.SelectTarget(new WindowInfo(form.Handle), form.RectangleToScreen(form.ClientRectangle)); throw new Exception("active target replaced"); }
                catch (InvalidOperationException) { }
                var status = await capturing;
                if (status != ScrollingCaptureStatus.Successful || manager.Result is null)
                    throw new Exception($"scroll capture failed: {status}");
                var originalResult = manager.Result;
                using var result = manager.TakeResult() ?? throw new Exception("result transfer missing");
                if (!ReferenceEquals(result, originalResult) || manager.Result is not null || manager.TakeResult() is not null) throw new Exception("result transfer cloned or retained the bitmap");
                manager.Dispose();
                if (result.Size != new Size(form.ClientSize.Width, ScrollDocument.DocumentHeight))
                {
                    Directory.CreateDirectory(args[0]);
                    result.Save(Path.Combine(args[0], "failed-stitch.png"));
                    var covering = WindowFromPoint(form.PointToScreen(new Point(180, 180)));
                    NativeMethods.GetWindowThreadProcessId(covering, out var coveringProcess);
                    throw new Exception($"stitched size {result.Size}, expected {form.ClientSize.Width}x{ScrollDocument.DocumentHeight}; offset={form.Offset}, topPixel={result.GetPixel(result.Width - 20, 0)}, expectedTop={ScrollDocument.RowColor(0)}; target={form.Handle}, covering={covering}, coveringProcess={coveringProcess}, self={Environment.ProcessId}, foreground={NativeMethods.GetForegroundWindow()}");
                }
                for (var y = 0; y < ScrollDocument.DocumentHeight; y += 37)
                {
                    var actual = result.GetPixel(result.Width - 20, y);
                    if (actual.ToArgb() != ScrollDocument.RowColor(y).ToArgb())
                        throw new Exception($"stitch seam/pixel mismatch at row {y}: {actual}");
                }
                Directory.CreateDirectory(args[0]);
                result.Save(Path.Combine(args[0], "scroll-stitch.png"));
                // Exercise the UI-independent WinOne flow against the same real
                // document window, with deterministic HWND-targeted scrolling.
                var store = new SettingsStore(Path.Combine(args[0], "unified"));
                store.Save(store.Snapshot() with { OutputDirectory = args[0], OpenEditor = true, CopyToClipboard = false, AskWhereToSave = false, ScrollFromTop = false, ScrollDelay = 100 });
                var operations = new CaptureOperations(store, Path.Combine(args[0], "unified"));
                bool Select(ScrollingCaptureManager engine)
                {
                    engine.Options.ScrollMethod = ScrollMethod.ScrollMessage; engine.Options.ShowRegion = false; engine.Options.ScrollAmount = 2;
                    engine.SelectTarget(new WindowInfo(form.Handle), form.RectangleToScreen(form.ClientRectangle)); return true;
                }
                form.ResetDocument();
                var unified = await operations.CaptureScrollingAsync("complete", CancellationToken.None, "en-US", Select);
                if (unified is not CaptureEditorRequest request) throw new Exception("unified scroll did not open its draft");
                var draft = operations.GetDraft(request.DraftId)!;
                if (draft.Height != ScrollDocument.DocumentHeight || draft.Notice is not null || draft.SourceKind != "scroll") throw new Exception("complete scrolling draft was incorrect");
                draft = operations.UpdateDraft(draft.Id, draft.Revision, draft.Document with { Marks = [new AnnotationMark { Kind = "cover", X = 2, Y = 2, Width = 10, Height = 10, Color = "#000000" }] });
                try { await operations.CaptureScrollingAsync("cancel-retake", CancellationToken.None, "en-US", _ => false, draft.Id, draft.Revision); throw new Exception("retake cancellation was accepted"); }
                catch (OperationCanceledException) { }
                if (operations.GetDraft()?.Id != draft.Id || operations.GetDraft()?.Document.Marks.Count != 1) throw new Exception("cancelled retake lost the old draft");
                form.ResetDocument();
                var retaken = await operations.CaptureScrollingAsync("retake", CancellationToken.None, "en-US", Select, draft.Id, draft.Revision);
                if (retaken is not CaptureEditorRequest retakenRequest) throw new Exception("retake bypassed review");
                var replaced = operations.GetDraft(retakenRequest.DraftId)!;
                if (replaced.Id == draft.Id || replaced.Replaces != new CaptureReplacement(draft.Id, draft.Revision)) throw new Exception("retake lineage mismatch");
                operations.CancelDraft(replaced.Id, replaced.Revision);
                store.Save(store.Snapshot() with { OpenEditor = false });
                form.ResetDocument();
                form.OnNextScroll = () =>
                {
                    if (operations.StopScrolling("old-job")) throw new Exception("stale job stopped a new capture");
                    if (!operations.StopScrolling("stop")) throw new Exception("active stop was rejected");
                };
                var stopped = await operations.CaptureScrollingAsync("stop", CancellationToken.None, "en-US", Select);
                if (stopped is not CaptureEditorRequest stoppedRequest) throw new Exception("stopped capture bypassed review");
                draft = operations.GetDraft(stoppedRequest.DraftId)!;
                if (draft.Notice != "scrollStopped" || draft.Height < 1 || draft.Height >= ScrollDocument.DocumentHeight) throw new Exception("stopped result was not retained as partial");
                operations.CancelDraft(draft.Id, draft.Revision);
                Console.WriteLine($"PASS: real window scrolling, {result.Width}x{result.Height} stitched output, sampled rows match the document");
            }
            catch (Exception error) { failure = error; }
            finally { form.Close(); }
        };
        Application.Run(form);
        if (failure is null) return 0;
        Console.Error.WriteLine(failure); return 1;
    }

    private sealed class ScrollDocument : Form
    {
        public const int DocumentHeight = 1600;
        private int offset;
        public int Offset => offset;
        public Action? OnNextScroll;
        public void ResetDocument() { offset = 0; Invalidate(); Update(); }
        public ScrollDocument()
        {
            Text = "WinOne 滚动截图验证"; ClientSize = new Size(360, 360);
            AutoScaleMode = AutoScaleMode.None; StartPosition = FormStartPosition.CenterScreen;
            FormBorderStyle = FormBorderStyle.FixedToolWindow; TopMost = true;
            SetStyle(ControlStyles.AllPaintingInWmPaint | ControlStyles.OptimizedDoubleBuffer | ControlStyles.UserPaint, true);
        }
        public static Color RowColor(int y) => Color.FromArgb(y * 17 % 256, y * 7 % 256, y / 8 % 256);
        protected override void OnPaint(PaintEventArgs e)
        {
            for (var y = 0; y < ClientSize.Height; y++)
            {
                using var pen = new Pen(RowColor(y + offset));
                e.Graphics.DrawLine(pen, 0, y, ClientSize.Width, y);
            }
            for (var row = offset / 80 * 80; row < offset + ClientSize.Height; row += 80)
                TextRenderer.DrawText(e.Graphics, $"Row {row}", Font, new Point(8, row - offset), Color.White);
        }
        protected override void WndProc(ref Message message)
        {
            if (message.Msg == 0x115 && (message.WParam.ToInt64() & 0xffff) == 1)
            {
                offset = Math.Min(DocumentHeight - ClientSize.Height, offset + 60);
                Invalidate(); Update();
                var next = OnNextScroll; OnNextScroll = null; next?.Invoke(); return;
            }
            base.WndProc(ref message);
        }
    }
}
