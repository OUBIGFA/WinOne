using ShareX.ScreenCaptureLib;
using WinOne.Capture;

internal static class ScrollingFrameTests
{
    internal sealed class ProgressLease : IDisposable { public bool Disposed; public void Dispose() => Disposed = true; }
    internal sealed class Source : IScrollingCaptureSource
    {
        private Bitmap? image;
        public readonly TaskCompletionSource<ScrollingCaptureStatus> Completion = new();
        public bool Selected = true, Disposed, Stopped;
        public Action? Started;
        public Source(bool pixels = true) {
            if (pixels) { image = new Bitmap(8, 24); image.SetPixel(7, 23, Color.Coral); }
        }
        public Source(Bitmap pixels) => image = pixels;
        public bool SelectTarget() => Selected;
        public Task<ScrollingCaptureStatus> CaptureAsync() { Started?.Invoke(); return Completion.Task; }
        public void Stop() { Stopped = true; Completion.TrySetResult(ScrollingCaptureStatus.PartiallySuccessful); }
        public Bitmap? TakeResult() { var value = image; image = null; return value; }
        public void Dispose() { Disposed = true; image?.Dispose(); image = null; }
    }

    public static void Run(string directory)
    {
        // Timers and async continuations use a normal STA message loop, with
        // no visible windows, screen capture, or input sent to the desktop.
        using var context = new ApplicationContext();
        using var start = new System.Windows.Forms.Timer { Interval = 1 };
        Exception? failure = null;
        start.Tick += async (_, _) => {
            start.Stop();
            try { await Verify(directory); } catch (Exception error) { failure = error; }
            finally { context.ExitThread(); }
        };
        start.Start(); Application.Run(context);
        if (failure is not null) System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(failure).Throw();
        Console.WriteLine("PASS: scrolling frame ownership, partial review, stop, cancellation and job exclusion");
    }

    private static async Task Verify(string directory)
    {
        var store = new SettingsStore(directory);
        var operations = new CaptureOperations(store, directory, _ => throw new Exception("Acquisition copied to clipboard"));
        var settings = store.Snapshot() with { OpenEditor = false };
        Source Ready(ScrollingCaptureStatus status) { var source = new Source(); source.Completion.SetResult(status); return source; }
        using var completed = Ready(ScrollingCaptureStatus.Successful);
        var frame = await operations.AcquireScrollingFrameAsync("complete", settings, CancellationToken.None, "en-US", createSource: _ => completed);
        if (!completed.Disposed || frame.NeedsReview || frame.Notice is not null || frame.Frame.Bounds != new Rectangle(0, 0, 8, 24) ||
            frame.Frame.WithImage(image => image.GetPixel(7, 23).ToArgb()) != Color.Coral.ToArgb())
            throw new Exception("Completed frame lost pixels, ownership or review settings");
        frame.Dispose();
        try { frame.Frame.WithImage(image => image.Width); throw new Exception("Disposed frame stayed readable"); }
        catch (ObjectDisposedException) { }
        if (operations.StopScrolling("complete")) throw new Exception("Finished job still accepted stop");

        using var partial = Ready(ScrollingCaptureStatus.PartiallySuccessful);
        using var partialFrame = await operations.AcquireScrollingFrameAsync("partial", settings, CancellationToken.None, null, createSource: _ => partial);
        if (!partialFrame.NeedsReview || partialFrame.Notice != "scrollPartial") throw new Exception("Partial output bypassed review");
        using var changed = new Source(); changed.Completion.SetException(new InvalidOperationException("WinOne.ScrollingTargetChanged"));
        using var changedFrame = await operations.AcquireScrollingFrameAsync("changed", settings, CancellationToken.None, null, createSource: _ => changed);
        if (changedFrame.Notice != "scrollTargetChanged") throw new Exception("Target-change notice was lost");

        using var rejected = new Source { Selected = false };
        try { await operations.AcquireScrollingFrameAsync("rejected", settings, CancellationToken.None, null, createSource: _ => rejected); throw new Exception("Cancelled selection returned a frame"); }
        catch (OperationCanceledException) { }
        if (!rejected.Disposed || operations.StopScrolling("rejected")) throw new Exception("Cancelled selection leaked the source/job");

        using var running = new Source();
        var progress = new ProgressLease(); Action? stopFromPanel = null;
        var pending = operations.AcquireScrollingFrameAsync("running", settings, CancellationToken.None, null, createSource: _ => running,
            showProgress: stop => { stopFromPanel = stop; return progress; });
        using var competing = Ready(ScrollingCaptureStatus.Successful);
        var competingCreated = false;
        try { await operations.AcquireScrollingFrameAsync("competing", settings, CancellationToken.None, null, createSource: _ => { competingCreated = true; return competing; }); throw new Exception("Concurrent scrolling was admitted"); }
        catch (InvalidOperationException error) when (error.Message == "请先完成当前滚动截图") { }
        if (competingCreated) throw new Exception("Rejected scrolling request initialized another capture source");
        if (operations.StopScrolling("stale") || stopFromPanel is null) throw new Exception("Stop targeted the wrong job or panel was missing");
        stopFromPanel();
        using var stopped = await pending.WaitAsync(TimeSpan.FromSeconds(3));
        if (!running.Stopped || !running.Disposed || !progress.Disposed || stopped.Notice != "scrollStopped" || !stopped.NeedsReview)
            throw new Exception("Stopping did not retain partial output for review");

        using var cancellation = new CancellationTokenSource();
        using var cancelling = new Source();
        var cancelledProgress = new ProgressLease();
        var cancelled = operations.AcquireScrollingFrameAsync("cancel", settings, cancellation.Token, null, createSource: _ => cancelling, showProgress: _ => cancelledProgress);
        cancellation.Cancel();
        try { await cancelled.WaitAsync(TimeSpan.FromSeconds(3)); throw new Exception("Cancellation returned an image"); }
        catch (OperationCanceledException) { }
        if (!cancelling.Stopped || !cancelling.Disposed || !cancelledProgress.Disposed || operations.StopScrolling("cancel")) throw new Exception("Cancellation leaked the source/job/panel");
        var created = false;
        try { await operations.AcquireScrollingFrameAsync("cancel-before", settings, cancellation.Token, null, createSource: _ => { created = true; return new Source(); }); }
        catch (OperationCanceledException) { }
        if (created) throw new Exception("Cancelled request created a capture source");

        using var unavailable = new Source(); var started = false; unavailable.Started = () => started = true;
        try { await operations.AcquireScrollingFrameAsync("unavailable", settings, CancellationToken.None, null, createSource: _ => unavailable,
            showProgress: _ => throw new IOException("progress unavailable")); throw new Exception("Capture continued without its stop panel"); }
        catch (IOException error) when (error.Message == "progress unavailable") { }
        if (started || !unavailable.Disposed || operations.StopScrolling("unavailable")) throw new Exception("Panel failure started capture or retained its task");

        using var empty = new Source(pixels: false); empty.Completion.SetResult(ScrollingCaptureStatus.Failed);
        try { await operations.AcquireScrollingFrameAsync("empty", settings, CancellationToken.None, "en-US", createSource: _ => empty); throw new Exception("Empty capture returned success"); }
        catch (InvalidOperationException error) when (error.Message == "No scrolling image was captured. Select the area again.") { }
        using var failed = new Source(); failed.Completion.SetException(new IOException("stitch failure"));
        try { await operations.AcquireScrollingFrameAsync("failed", settings, CancellationToken.None, null, createSource: _ => failed); throw new Exception("Failed stitching returned success"); }
        catch (IOException error) when (error.Message == "stitch failure") { }
        if (!empty.Disposed || !failed.Disposed || operations.StopScrolling("failed")) throw new Exception("Failed capture leaked its source/job");
        using var retry = Ready(ScrollingCaptureStatus.Successful);
        using var retried = await operations.AcquireScrollingFrameAsync("retry", settings, CancellationToken.None, null, createSource: _ => retry);
        if (retried.Notice is not null) throw new Exception("Retry inherited an old error or stop state");
    }
}
