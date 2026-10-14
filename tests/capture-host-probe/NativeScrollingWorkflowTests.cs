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
        NativeRetakeWorkflowTests.Run(Path.Combine(directory,"retake-workflow"));
        NativeCaptureRoutingTests.RunAsync(Path.Combine(directory, "routing")).GetAwaiter().GetResult();
        NativeDraftReplacementTests.Run(Path.Combine(directory, "replacement"));
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
            using var cancelledSource = new ScrollingFrameTests.Source(new Bitmap(source));
            cancelledSource.Selected = false;
            var cancelledProgress = false;
            try {
                NativeCaptureWorkflow.RunScrollingAsync(store, operations, "cancelled-selection", "zh-CN", CancellationToken.None, drafts,
                    createSource: _ => cancelledSource,
                    showProgress: _ => { cancelledProgress = true; return new ScrollingFrameTests.ProgressLease(); },
                    show: (_, _) => throw new Exception("Cancelled selection opened an editor")).GetAwaiter().GetResult();
                throw new Exception("Cancelled scrolling selection returned success");
            } catch (OperationCanceledException) { }
            if (!cancelledSource.Disposed || cancelledProgress || copies != 0 || drafts.Recover() is not null || operations.GetDraft() is not null)
                throw new Exception("Cancelled scrolling selection left resources or output");
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
            // Retake acquisition must be independent of the committed draft.
            // Keep a real edited long image on disk while a second selection
            // is cancelled or stitching fails, then recover the old pixels.
            var retained = JsonSerializer.Serialize(recovered);
            foreach (var cancelRetake in new[] { true, false }) {
                using var retakeSource = new ScrollingFrameTests.Source(new Bitmap(source)) { Selected = !cancelRetake };
                if (!cancelRetake) retakeSource.Completion.SetException(new IOException("retake stitch failed"));
                try {
                    using var unexpected = operations.AcquireScrollingFrameAsync("retake-failure-" + cancelRetake, store.Snapshot(), CancellationToken.None, "en-US",
                        createSource: _ => retakeSource, showProgress: _ => new ScrollingFrameTests.ProgressLease()).GetAwaiter().GetResult();
                    throw new Exception("Unsuccessful retake returned a replacement");
                } catch (OperationCanceledException) when (cancelRetake) { }
                catch (IOException error) when (!cancelRetake && error.Message == "retake stitch failed") { }
                if (!retakeSource.Disposed || JsonSerializer.Serialize(drafts.Recover()) != retained || copies != 0)
                    throw new Exception("Retake failure changed the committed draft or retained acquisition resources");
                using var original = drafts.LoadImage(recovered.Id, recovered.Revision);
                using var restored = new WinOneCaptureCanvas(original, viewport: true);
                restored.RestoreDocument(recovered.Document);
                using var restoredOutput = restored.RenderSelection();
                for (var y = 0; y < restoredOutput.Height; y++) for (var x = 0; x < restoredOutput.Width; x++)
                    if (restoredOutput.GetPixel(x, y) != expected!.GetPixel(x, y)) throw new Exception("Retake failure changed recoverable annotated pixels");
            }
            try {
                NativeCaptureWorkflow.Resume(store, operations, new string('f', 32), "en-US", CancellationToken.None, drafts,
                    show: (_, _) => throw new Exception("Wrong draft opened a window")).GetAwaiter().GetResult();
                throw new Exception("Wrong draft was accepted");
            } catch (InvalidOperationException error) when (error.Message == "截图草稿已变化，请重新加载") { }
            if (JsonSerializer.Serialize(drafts.Recover()) != retained) throw new Exception("A stale resume changed the saved draft");
            NativeCaptureWorkflow.Resume(store, operations, recovered.Id, "en-US", CancellationToken.None, new NativeCaptureDraftStore(directory),
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
                outputPixelsIdentical = true, directSave = true, retakeFailurePreservedDraft = true, legacyDraft = false, realClipboard = false
            }));
        } finally { expected?.Dispose(); }
        Console.WriteLine("PASS: long scrolling edit survives interruption and restores exact annotated output without recapture");
    }
}
