using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;

internal static class NativeScrollingWorkflowTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);

    public static void Run(string directory)
    {
        Directory.CreateDirectory(directory);
        var store = new SettingsStore(directory);
        store.Save(store.Snapshot() with { OpenEditor = false, CopyToClipboard = false, AskWhereToSave = true, OutputDirectory = Path.Combine(directory, "images") });
        var drafts = new NativeCaptureDraftStore(directory);
        var copies = 0;
        using var source = new Bitmap(240, 2400);
        using (var g = Graphics.FromImage(source)) {
            g.Clear(Color.White); g.FillRectangle(Brushes.RoyalBlue, 0, 0, 240, 300);
            g.FillRectangle(Brushes.Gold, 0, 2100, 240, 300);
        }
        Bitmap? expected = null;
        var operations = new CaptureOperations(store, directory, image => {
            copies++;
            if (image.Size != source.Size || expected is null) throw new Exception("Long image dimensions changed");
            for (var y = 0; y < image.Height; y++) for (var x = 0; x < image.Width; x++)
                if (image.GetPixel(x, y) != expected.GetPixel(x, y)) throw new Exception($"Recovered scrolling output changed at {x},{y}");
        });
        void Show(WinOneCaptureCanvas canvas, NativeCaptureSession session, Action act)
        {
            Exception? failure = null;
            using var deadline = new System.Windows.Forms.Timer { Interval = 10000 };
            deadline.Tick += (_, _) => { failure = new TimeoutException("Scrolling editor did not complete"); canvas.Window.Close(); };
            canvas.Window.Shown += (_, _) => canvas.Window.BeginInvoke((Action)(() => {
                try { act(); } catch (Exception error) { failure = error; canvas.Window.Close(); }
            }));
            deadline.Start(); canvas.ShowDialog(); deadline.Stop();
            if (failure is not null) System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(failure).Throw();
        }
        try {
            using var cancellation = new CancellationTokenSource();
            using var producer = new ScrollingFrameTests.Source(new Bitmap(source));
            producer.Completion.SetResult(ScrollingCaptureStatus.PartiallySuccessful);
            producer.Started = () => {
                if (!operations.StopScrolling("native-scroll")) throw new Exception("Native scrolling job did not accept stop");
            };
            var progress = new ScrollingFrameTests.ProgressLease();
            try {
                NativeCaptureWorkflow.RunScrollingAsync(store, operations, "native-scroll", "zh-CN", cancellation.Token, drafts,
                    createSource: _ => producer, showProgress: _ => progress, show: (canvas, session) => Show(canvas, session, () => {
                        if (!progress.Disposed) throw new Exception("Scroll controls remained open over the editor");
                        if (canvas.Selection != new Rectangle(Point.Empty, source.Size)) throw new Exception("Long image crop was clipped to the screen");
                        canvas.AddRectangle(new Rectangle(20, 2150, 150, 100));
                        expected = canvas.RenderSelection();
                        cancellation.Cancel();
                    })).GetAwaiter().GetResult();
                throw new Exception("Interrupted scrolling edit returned success");
            } catch (OperationCanceledException) when (cancellation.IsCancellationRequested) { }
            var recovered = new NativeCaptureDraftStore(directory).Recover() ?? throw new Exception("Interrupted long image was lost");
            if (recovered.Mode != "scroll" || recovered.Notice != "scrollStopped") throw new Exception("Scrolling recovery lost its review notice");
            if (copies != 0) throw new Exception("Interrupted edit changed the clipboard");
            NativeCaptureWorkflow.RunScrollingAsync(store, operations, "recovered-scroll", "en-US", CancellationToken.None, new NativeCaptureDraftStore(directory),
                createSource: _ => throw new Exception("Scrolling recovery took a new screenshot"),
                show: (canvas, session) => Show(canvas, session, () => {
                    if (canvas.Selection != new Rectangle(Point.Empty, source.Size) || canvas.AnnotationCount != 1)
                        throw new Exception("Long image recovery lost crop or annotations");
                    SendMessageW(canvas.Window.Handle, 0x0203, IntPtr.Zero, IntPtr.Zero);
                })).GetAwaiter().GetResult();
            if (copies != 1 || drafts.Recover() is not null) throw new Exception("Confirmed scrolling recovery did not finish exactly once");
            using var complete = new ScrollingFrameTests.Source(new Bitmap(source));
            complete.Completion.SetResult(ScrollingCaptureStatus.Successful);
            var quick = (CaptureOutput)NativeCaptureWorkflow.RunScrollingAsync(store, operations, "quick-scroll", "en-US", CancellationToken.None, drafts,
                createSource: _ => complete, showProgress: _ => new ScrollingFrameTests.ProgressLease(), show: (_, _) => throw new Exception("Completed direct-save capture opened an editor")).GetAwaiter().GetResult();
            using (var saved = new Bitmap(quick.Path)) {
                if (saved.Size != source.Size || saved.GetPixel(120, 2250) != source.GetPixel(120, 2250))
                    throw new Exception("Direct scrolling save changed the long image");
            }
            if (!complete.Disposed || copies != 1 || drafts.Recover() is not null || operations.GetDraft() is not null)
                throw new Exception("Native scrolling left a legacy draft or changed the clipboard");
            File.WriteAllText(Path.Combine(directory, "verification.json"), JsonSerializer.Serialize(new {
                width = source.Width, height = source.Height, copies, preservedNotice = true, recoveredAnnotations = 1,
                outputPixelsIdentical = true, directSave = true, legacyDraft = false, realClipboard = false
            }));
        } finally { expected?.Dispose(); }
        Console.WriteLine("PASS: long scrolling edit survives interruption and restores exact annotated output without recapture");
    }
}
