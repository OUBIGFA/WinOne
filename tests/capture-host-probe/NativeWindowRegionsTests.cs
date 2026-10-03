using ShareX.ScreenCaptureLib;
using WinOne.Capture;

internal static class NativeWindowRegionsTests
{
    public static void RunWithMessageLoop()
    {
        Exception? failure = null;
        using var dispatcher = new Form { ShowInTaskbar = false };
        _ = dispatcher.Handle;
        dispatcher.BeginInvoke(async () => {
            try { await Run(); } catch (Exception error) { failure = error; }
            finally { Application.ExitThread(); }
        });
        Application.Run();
        if (failure is not null) throw failure;
    }
    public static async Task Run()
    {
        using var source = new Bitmap(400, 300);
        using var capture = new WinOneCaptureCanvas(source);
        var front = new WindowCaptureTarget(new IntPtr(1), new Rectangle(-100, 20, 240, 180));
        var behind = new WindowCaptureTarget(new IntPtr(2), new Rectangle(-120, 0, 400, 280));
        var detail = new Rectangle(-60, 50, 100, 80);
        var calls = 0;
        using var regions = new NativeWindowRegions(capture, new[] { front, behind }, (_, _) => {
            calls++; return Task.FromResult<IReadOnlyList<Rectangle>>(new[] { detail });
        });
        regions.Hover(new Point(-50, 70));
        if (calls != 0 || !capture.ActiveWindowCandidates.SequenceEqual(new[] { front.Bounds, behind.Bounds })) throw new Exception("First entry did not prioritize complete windows");
        regions.Hover(new Point(-30, 85)); await regions.Pending;
        if (capture.ActiveWindowCandidates[0] != detail || capture.ActiveWindowCandidates[^1] != behind.Bounds) throw new Exception("Interior refinement lost native candidate order");
        regions.Hover(new Point(-99, 70));
        if (capture.ActiveWindowCandidates[0] != front.Bounds) throw new Exception("Window edge did not restore complete-window selection");
        capture.SelectTool("rectangle");
        if (capture.ActiveWindowCandidates.Length != 0) throw new Exception("Drawing tool inherited window snapping");
        var previousCalls = calls;
        regions.Hover(new Point(-30, 85)); await regions.Pending;
        if (calls != previousCalls) throw new Exception("Drawing started an unnecessary window-region query");
        capture.SelectTool("region");
        if (capture.ActiveWindowCandidates[0] != front.Bounds) throw new Exception("Region tool did not restore candidates");
        using var drawing = new WinOneCaptureCanvas(source);
        var started = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        var delayed = new TaskCompletionSource<IReadOnlyList<Rectangle>>(TaskCreationOptions.RunContinuationsAsynchronously);
        var duringDrag = 0; CancellationToken queryToken = default;
        using var pending = new NativeWindowRegions(drawing, new[] { front }, (_, token) => {
            duringDrag++; queryToken = token; started.TrySetResult(); return delayed.Task;
        });
        drawing.BeginRegion(new Rectangle(0, 0, 50, 50));
        pending.Hover(new Point(-50, 70)); pending.Hover(new Point(-30, 85));
        await Task.Delay(200);
        if (duringDrag != 0) { delayed.TrySetResult(Array.Empty<Rectangle>()); throw new Exception("Manual region drag started accessibility work"); }
        drawing.FinishGesture();
        pending.Hover(new Point(-50, 70)); pending.Hover(new Point(-30, 85));
        await started.Task.WaitAsync(TimeSpan.FromSeconds(3));
        var oldRequest = pending.Pending;
        drawing.SelectTool("rectangle");
        if (!queryToken.IsCancellationRequested) throw new Exception("Leaving selection did not cancel the query");
        delayed.SetResult(new[] { detail }); await oldRequest;
        if (drawing.ActiveWindowCandidates.Length != 0) throw new Exception("Late query restored snapping while drawing");
        drawing.SelectTool("region");
        if (!drawing.ActiveWindowCandidates.SequenceEqual(new[] { front.Bounds })) throw new Exception("Cancelled details leaked into the next selection");
    }
}
