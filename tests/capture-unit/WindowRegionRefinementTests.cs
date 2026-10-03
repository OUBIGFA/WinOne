using WinOne.Capture;

internal static class WindowRegionRefinementTests
{
    internal static async Task Run(string directory)
    {
        var first = new WindowCaptureTarget(new IntPtr(1), new Rectangle(0, 0, 100, 100));
        var second = new WindowCaptureTarget(new IntPtr(2), new Rectangle(100, 0, 100, 100));
        var native = new Dictionary<IntPtr, IReadOnlyList<Rectangle>> { [first.Handle] = [first.Bounds], [second.Handle] = [second.Bounds] };
        var a = new TaskCompletionSource<IReadOnlyList<Rectangle>>(TaskCreationOptions.RunContinuationsAsynchronously);
        var b = new TaskCompletionSource<IReadOnlyList<Rectangle>>(TaskCreationOptions.RunContinuationsAsynchronously);
        var started = new System.Collections.Concurrent.ConcurrentBag<IntPtr>(); var updates = new List<IReadOnlyList<Rectangle>>();
        using var refinement = new WindowRegionRefinement([first, second], native, update => updates.Add(update), (target, _) => { started.Add(target.Handle); return target.Handle == first.Handle ? a.Task : b.Task; });
        refinement.Hover(new Point(20, 20));
        if (!refinement.Pending.IsCompleted || started.Count != 0 || !updates.Last().SequenceEqual(new[] { first.Bounds, second.Bounds })) throw new Exception("first entry did not prefer top-level windows");
        refinement.Hover(new Point(20, 20));
        if (started.Count != 0) throw new Exception("stationary pointer started detection");
        refinement.Hover(new Point(40, 40)); var old = refinement.Pending;
        await WaitUntil(() => started.Contains(first.Handle));
        refinement.Hover(new Point(120, 20));
        if (!updates.Last().SequenceEqual(new[] { first.Bounds, second.Bounds })) throw new Exception("entering another window retained internal regions");
        refinement.Hover(new Point(140, 40)); var current = refinement.Pending;
        await WaitUntil(() => started.Contains(second.Handle));
        var detail = new Rectangle(110, 10, 20, 20); b.SetResult([detail]); await current;
        a.SetResult([new Rectangle(10, 10, 20, 20)]); await old;
        if (updates.Count != 2 || !updates.Last().SequenceEqual(new[] { first.Bounds, detail, second.Bounds })) throw new Exception("stale refinement replaced the new target or changed Z order");
        refinement.Hover(new Point(500, 500)); refinement.Hover(new Point(120, 20));
        if (!updates.Last().SequenceEqual(new[] { first.Bounds, second.Bounds })) throw new Exception("cached regions bypassed top-level priority on reentry");
        refinement.Hover(new Point(140, 40));
        if (!updates.Last().Contains(detail)) throw new Exception("movement did not reveal cached details");
        if (started.Count != 3) throw new Exception("settled target started another process or confirmation was unbounded");
        refinement.Hover(new Point(101, 50));
        if (updates.Last().Contains(detail)) throw new Exception("window edge retained an internal selection");
        refinement.Hover(new Point(113, 50));
        if (updates.Last().Contains(detail)) throw new Exception("edge jitter caused selection flicker");
        refinement.Hover(new Point(140, 50));
        if (!updates.Last().Contains(detail) || started.Count != 3) throw new Exception("returning inside did not reuse detected regions");

        var negative = new Rectangle(-200, -100, 100, 80);
        foreach (var edge in new[] { new Point(-199, -60), new Point(-101, -60), new Point(-150, -99), new Point(-150, -21) })
            if (!WindowRegionRefinement.NearEdge(negative, edge, 12)) throw new Exception("negative-coordinate window edge missed");
        if (WindowRegionRefinement.NearEdge(negative, new Point(-150, -60), 12)) throw new Exception("window interior was treated as an edge");

        var small = new WindowCaptureTarget(new IntPtr(3), new Rectangle(0, 0, 30, 60));
        var smallDetail = new Rectangle(10, 10, 12, 20);
        IReadOnlyList<Rectangle> smallSelection = [];
        using var smallIntent = new WindowRegionRefinement([small], new Dictionary<IntPtr, IReadOnlyList<Rectangle>> { [small.Handle] = [small.Bounds] },
            value => smallSelection = value, (_, _) => Task.FromResult<IReadOnlyList<Rectangle>>([smallDetail]));
        smallIntent.Hover(new Point(15, 15)); smallIntent.Hover(new Point(15, 20)); await smallIntent.Pending;
        smallIntent.Hover(new Point(9, 20));
        if (!smallSelection.Contains(smallDetail)) throw new Exception("small-window edge clamping removed hysteresis");
        smallIntent.Hover(new Point(6, 20)); smallIntent.Hover(new Point(8, 20));
        if (smallSelection.Contains(smallDetail)) throw new Exception("small-window edge jitter reentered internal selection");
        smallIntent.Hover(new Point(12, 20));
        if (!smallSelection.Contains(smallDetail)) throw new Exception("small-window interior could not be selected again");

        var captioned = new WindowCaptureTarget(new IntPtr(4), new Rectangle(0, 0, 200, 120), new Rectangle(8, 32, 184, 80));
        var captionQueries = 0; IReadOnlyList<Rectangle> captionSelection = [];
        using var captionIntent = new WindowRegionRefinement([captioned], new Dictionary<IntPtr, IReadOnlyList<Rectangle>> { [captioned.Handle] = [captioned.Bounds] },
            value => captionSelection = value, (_, _) => { captionQueries++; return Task.FromResult<IReadOnlyList<Rectangle>>([new Rectangle(50, 40, 100, 40)]); });
        captionIntent.Hover(new Point(100, 20)); captionIntent.Hover(new Point(120, 20)); await captionIntent.Pending;
        if (captionQueries != 0 || !captionSelection.SequenceEqual(new[] { captioned.Bounds })) throw new Exception("native title bar became an internal region");
        captionIntent.Hover(new Point(100, 60)); await captionIntent.Pending;
        if (captionQueries == 0 || captionSelection.Count < 2) throw new Exception("client area could not select its interior");
        captionIntent.Hover(new Point(100, 20));
        if (!captionSelection.SequenceEqual(new[] { captioned.Bounds })) throw new Exception("returning to title bar did not restore the whole window");

        var delayed = new TaskCompletionSource<IReadOnlyList<Rectangle>>(TaskCreationOptions.RunContinuationsAsynchronously);
        var edgeSnapshots = new List<IReadOnlyList<Rectangle>>(); var querying = false;
        using var edgeIntent = new WindowRegionRefinement([first], native, snapshot => edgeSnapshots.Add(snapshot), (_, _) => { querying = true; return delayed.Task; });
        edgeIntent.Hover(new Point(20, 20)); edgeIntent.Hover(new Point(40, 40));
        await WaitUntil(() => querying); edgeIntent.Hover(new Point(1, 50));
        delayed.SetResult([new Rectangle(20, 20, 30, 30)]); await edgeIntent.Pending;
        if (!edgeSnapshots.Last().SequenceEqual(new[] { first.Bounds })) throw new Exception("late detection overrode whole-window edge intent");
        edgeIntent.Hover(new Point(40, 40));
        if (edgeSnapshots.Last()[0] != new Rectangle(20, 20, 30, 30)) throw new Exception("moving inside did not select the detected region");

        var snapshots = new List<IReadOnlyList<Rectangle>>(); var reads = 0;
        using var warming = new WindowRegionRefinement([first], native, snapshot => snapshots.Add(snapshot), (_, _) => Task.FromResult<IReadOnlyList<Rectangle>>(++reads == 1 ? [first.Bounds] : [new Rectangle(10, 10, 30, 30)]));
        warming.Hover(new Point(20, 20)); warming.Hover(new Point(40, 40)); await warming.Pending;
        if (reads != 2 || snapshots.Count != 2 || snapshots[1][0] != new Rectangle(10, 10, 30, 30)) throw new Exception("cold accessibility tree was cached before its interior appeared");

        var cancelled = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        var closingUpdates = 0;
        using var closing = new WindowRegionRefinement([first], native, _ => closingUpdates++, async (_, token) => { token.Register(() => cancelled.TrySetResult()); await Task.Delay(10000, token); return []; });
        closing.Hover(new Point(20, 20)); closing.Hover(new Point(40, 40)); await Task.Delay(150); var beforeClose = closingUpdates; closing.Dispose(); await closing.Pending;
        if (closingUpdates != beforeClose) throw new Exception("disposed selector updated");
        await cancelled.Task.WaitAsync(TimeSpan.FromSeconds(2));

        var nativeReads = 0;
        using var lazy = new WindowRegionRefinement([first], native, _ => {}, (_, _) => Task.FromResult<IReadOnlyList<Rectangle>>([]), _ => { nativeReads++; return [first.Bounds]; });
        lazy.Hover(new Point(20, 20)); await lazy.Pending;
        if (nativeReads != 0) throw new Exception("native child collection delayed first entry");
        lazy.Hover(new Point(40, 40)); await lazy.Pending;
        if (nativeReads != 1) throw new Exception("native children were not collected on demand");

        var failures = 0;
        using var unavailable = new WindowRegionRefinement([first], native, _ => {}, (_, _) => { failures++; throw new IOException("provider unavailable"); });
        unavailable.Hover(new Point(20, 20)); unavailable.Hover(new Point(40, 40)); await unavailable.Pending;
        unavailable.Hover(new Point(50, 50)); unavailable.Hover(new Point(60, 60)); await unavailable.Pending;
        if (failures != 1) throw new Exception("pointer movement repeatedly restarted a failed provider");

        if (AccessibleWindowRegions.Parse("-20 -10 80 90", new Rectangle(0, 0, 100, 100)) != new Rectangle(0, 0, 80, 90)) throw new Exception("accessibility region was not clipped");
        foreach (var invalid in new[] { "1 2 3", "1 1 0 0", "-2147483648 0 2147483647 10" })
        {
            try { AccessibleWindowRegions.Parse(invalid, first.Bounds); throw new Exception("invalid accessibility bounds accepted"); }
            catch (InvalidDataException) { }
        }
        var helper = Path.ChangeExtension(typeof(WindowRegionRefinementTests).Assembly.Location, ".exe");
        var name = Path.GetFileNameWithoutExtension(helper);
        static HashSet<int> Processes(string name)
        {
            return System.Diagnostics.Process.GetProcessesByName(name).Select(process => { using (process) return process.Id; }).ToHashSet();
        }
        var before = Processes(name);
        try
        {
            await AccessibleWindowRegions.ReadAsync(new WindowCaptureTarget(new IntPtr(-777), first.Bounds), CancellationToken.None, helper, TimeSpan.FromMilliseconds(100));
            throw new Exception("hung helper ignored its deadline");
        }
        catch (TimeoutException) { }
        if (Processes(name).Except(before).Any()) throw new Exception("timed-out helper process was retained");
        Directory.CreateDirectory(directory);
        var retryFile = Path.Combine(directory, "attempt.txt");
        var previous = Environment.GetEnvironmentVariable("WINONE_REGION_RETRY_TEST_FILE");
        try
        {
            Environment.SetEnvironmentVariable("WINONE_REGION_RETRY_TEST_FILE", retryFile);
            var recovered = await AccessibleWindowRegions.ReadAsync(new WindowCaptureTarget(new IntPtr(-778), first.Bounds), CancellationToken.None, helper);
            if (!recovered.SequenceEqual(new[] { first.Bounds }) || File.ReadAllText(retryFile) != "2") throw new Exception("provider initialization was not retried exactly once");
        }
        finally { Environment.SetEnvironmentVariable("WINONE_REGION_RETRY_TEST_FILE", previous); }
    }
    private static async Task WaitUntil(Func<bool> ready)
    {
        var deadline = Environment.TickCount64 + 2000;
        while (!ready()) { if (Environment.TickCount64 > deadline) throw new TimeoutException("refinement did not start"); await Task.Delay(10); }
    }
}
