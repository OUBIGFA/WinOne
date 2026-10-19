using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class NativeScrollInteractionTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    internal static void Confirm(NativeCaptureSession session)
    {
        // A shown HWND is not yet a painted/interactive toolbar. Use the
        // session's readiness event rather than a sleep or bypassing its guard.
        if (!session.Ready) {
            void Ready() {
                if (!session.Ready) return;
                session.Updated -= Ready;
                session.Toolbar.BeginInvoke((Action)(() => Confirm(session)));
            }
            session.Updated += Ready;
            return;
        }
        session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row => row.Items.Cast<ToolStripItem>())
            .Single(item => item.Name == "confirm").PerformClick();
    }
    private static IntPtr PointMessage(Point point) => new(unchecked((int)((uint)(ushort)point.X | ((uint)(ushort)point.Y << 16))));

    private static void VerifyDismissalGuards(string directory)
    {
        foreach (var state in new[] { "recognition", "replacement-error" }) {
            using var image = new Bitmap(480, 1800);
            using var canvas = new WinOneCaptureCanvas(image, viewport: true);
            canvas.SetRegion(new Rectangle(Point.Empty, image.Size));
            var service = new PendingResult();
            using var session = new NativeCaptureSession(canvas, _ => throw new Exception("Unexpected confirmation"), _ => { }, textService: service);
            using var context = new ApplicationContext(canvas.Window);
            Exception? failure = null; var acted = false; var verified = false;
            using var deadline = new System.Windows.Forms.Timer { Interval = 10000 };
            deadline.Tick += (_, _) => { failure ??= new TimeoutException("Long-image dismissal guard timed out: " + state); canvas.Window.Close(); };
            session.Updated += () => {
                if (acted || !session.Ready) return;
                acted = true;
                canvas.Window.BeginInvoke((Action)(async () => {
                    try {
                        if (state == "recognition") {
                            session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row => row.Items.Cast<ToolStripItem>()).Single(item => item.Name == "ocr").PerformClick();
                            await service.Started.Task.WaitAsync(TimeSpan.FromSeconds(5));
                            if (!session.RecognitionBusy) throw new Exception("The pending result fixture did not hold recognition open");
                        } else session.ReportReplacementFailure("Generated test replacement error");
                        foreach (var message in new uint[] { 0x0203, 0x0204, 0x0205 }) {
                            SendMessageW(canvas.Window.Handle, message, IntPtr.Zero, PointMessage(new Point(80, 80)));
                            if (canvas.Window.IsDisposed || !canvas.Window.Visible || session.CloseRequested)
                                throw new Exception($"Long-image {state} was dismissed by 0x{message:X}");
                        }
                        using (var other = new Form { Text = "Owned focus fixture", ShowInTaskbar = false }) {
                            other.Show(); other.Activate(); await Task.Yield();
                            if (canvas.Window.IsDisposed || !canvas.Window.Visible) throw new Exception("Focus loss dismissed the long-image editor: " + state);
                        }
                        verified = true;
                        // Escape is an explicit user exit and must remain usable
                        // even while a result handoff or replacement is pending.
                        SendMessageW(canvas.Window.Handle, 0x0100, new IntPtr((int)Keys.Escape), IntPtr.Zero);
                    } catch (Exception error) { failure = error; canvas.Window.Close(); }
                }));
            };
            deadline.Start(); Application.Run(context); deadline.Stop();
            if (failure is not null) throw failure;
            if (!verified) throw new Exception("Long-image dismissal guard was not exercised: " + state);
            File.WriteAllText(Path.Combine(directory, state + "-dismissal.json"), JsonSerializer.Serialize(new { clicksRetained = true, focusRetained = true, explicitExit = true }));
        }
    }
    private sealed class PendingResult : ICaptureTextService
    {
        internal TaskCompletionSource Started { get; } = new(TaskCreationOptions.RunContinuationsAsynchronously);
        public Task<string> Recognize(Bitmap image, CancellationToken token) => throw new Exception("Unexpected silent OCR");
        public Task<string> Translate(string text, CancellationToken token) => throw new Exception("Unexpected in-editor translation");
        public async Task<string> OpenResult(Bitmap image, bool translate, CancellationToken token)
        {
            Started.TrySetResult();
            await Task.Delay(Timeout.Infinite, token);
            throw new Exception("A cancelled generated result must not open");
        }
    }
    internal static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        VerifyDismissalGuards(directory);
        var settings = new SettingsStore(directory);
        var bounds = CaptureHelpers.GetScreenBounds();
        using var desktop = new Bitmap(bounds.Width, bounds.Height); // Generated pixels, never the real desktop.
        using var canvas = new WinOneCaptureCanvas(desktop);
        var selected = new Rectangle(100, 100, 400, 260);
        canvas.SetRegion(selected);
        var copies = 0;
        using var session = new NativeCaptureSession(canvas, image => {
            copies++;
            if (image.Size != new Size(480, 1800)) throw new Exception("Long-image copy was clipped to its viewport");
        }, _ => throw new Exception("Unexpected save"));
        var drafts = new NativeCaptureDraftStore(directory);
        var autosave = new NativeCaptureAutosave(drafts, new Bitmap(desktop), settings.Snapshot(), canvas.ExportDocument(), "region", CapturePlacement.From(bounds, bounds));
        var operations = new CaptureOperations(settings, directory, _ => throw new Exception("Retake must not copy to the real clipboard"));
        using var source = new ScrollingFrameTests.Source(new Bitmap(480, 1800));
        source.Completion.SetResult(ScrollingCaptureStatus.Successful);
        Exception? failure = null;
        var acted = false;
        using var context = new ApplicationContext(canvas.Window);
        using var deadline = new System.Windows.Forms.Timer { Interval = 15000 };
        deadline.Tick += (_, _) => { failure ??= new TimeoutException("Scroll interaction test did not complete"); canvas.Window.Close(); };
        session.Updated += () => {
            if (acted || !session.Ready) return;
            acted = true;
            canvas.Window.BeginInvoke((Action)(async () => {
                try {
                    var handle = canvas.Window.Handle;
                    await NativeCaptureRetake.RunAsync(canvas, session, operations, settings.Snapshot(), drafts, autosave, "zh-CN", CancellationToken.None,
                        _ => { }, _ => source, _ => new ScrollingFrameTests.ProgressLease());
                    await autosave.FinishAsync(false);
                    var expected = selected; expected.Offset(bounds.Location);
                    if (source.RequestedRegion != expected || !canvas.HasViewport || canvas.Window.Handle != handle)
                        throw new Exception("The first physical selection was not reused in the same editor");
                    // The actual native input path must not treat editing clicks
                    // (including right/double click) as implicit completion.
                    foreach (var message in new uint[] { 0x0201, 0x0202, 0x0203, 0x0204, 0x0205 }) {
                        SendMessageW(handle, message, message == 0x0201 ? new IntPtr(1) : IntPtr.Zero, PointMessage(new Point(40, 40)));
                        if (canvas.Window.IsDisposed || !canvas.Window.Visible || copies != 0)
                            throw new Exception($"Editing input 0x{message:X} closed or confirmed the long image");
                    }
                    canvas.SetViewport(0.8f, new PointF(20, 0));
                    var focus = new Point(180, 160);
                    var imagePoint = canvas.ViewToImage(focus);
                    var before = NativeCaptureDocumentCodec.Encode(canvas.ExportDocument(), new Size(480, 1800));
                    var screenPoint = canvas.Window.PointToScreen(focus);
                    SendMessageW(handle, 0x020a, new IntPtr(120 << 16), PointMessage(screenPoint));
                    var afterPoint = canvas.ViewToImage(focus);
                    if (canvas.ViewScale <= 0.8f || Math.Abs(afterPoint.X - imagePoint.X) > 1 || Math.Abs(afterPoint.Y - imagePoint.Y) > 1)
                        throw new Exception("Plain wheel did not zoom around the mouse position");
                    SendMessageW(handle, 0x020a, new IntPtr(unchecked(-120 << 16)), PointMessage(screenPoint));
                    if (Math.Abs(canvas.ViewScale - 0.8f) > 0.001f || !NativeCaptureDocumentCodec.Encode(canvas.ExportDocument(), new Size(480, 1800)).SequenceEqual(before))
                        throw new Exception("Wheel zoom changed original image/document coordinates");
                    if (!canvas.Window.ClientRectangle.Contains(session.Toolbar.Bounds)) throw new Exception("Zoom moved the toolbar off screen");
                    Confirm(session);
                } catch (Exception error) { failure = error; canvas.Window.Close(); }
            }));
        };
        deadline.Start(); Application.Run(context); deadline.Stop();
        autosave.FinishAsync(false).GetAwaiter().GetResult();
        if (failure is not null) throw failure;
        if (copies != 1 || !source.Disposed) throw new Exception("Explicit confirmation did not finish once or release the scrolling source");
        File.WriteAllText(Path.Combine(directory, "verification.json"), JsonSerializer.Serialize(new {
            selectionReused = true, sameWindow = true, editingClicksRetained = true, wheelAtPointer = true, fullResolutionCopy = true, copies
        }));
    }
}
