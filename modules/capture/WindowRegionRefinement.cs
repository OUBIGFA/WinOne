namespace WinOne.Capture;

// One selector owns this cache and its pending query. Top-level windows are
// immediately usable; interior collection starts only after movement within one.
internal sealed class WindowRegionRefinement : IDisposable
{
    private readonly IReadOnlyList<WindowCaptureTarget> targets;
    private readonly Dictionary<IntPtr, IReadOnlyList<Rectangle>> native;
    private readonly Func<WindowCaptureTarget, IReadOnlyList<Rectangle>>? readNative;
    private readonly HashSet<IntPtr> nativeCollected = new();
    private readonly Action<IReadOnlyList<Rectangle>> update;
    private readonly Func<WindowCaptureTarget, CancellationToken, Task<IReadOnlyList<Rectangle>>> read;
    private readonly Dictionary<IntPtr, IReadOnlyList<Rectangle>> cache = new();
    private readonly HashSet<IntPtr> settled = new();
    private CancellationTokenSource? active;
    private IntPtr hovered;
    private Point enteredAt;
    private int edgeWidth = 12;
    private bool interiorEnabled;
    private bool requested;
    private IReadOnlyList<Rectangle>? published;
    private bool disposed;
    internal Task Pending { get; private set; } = Task.CompletedTask;

    internal WindowRegionRefinement(IReadOnlyList<WindowCaptureTarget> targets, IReadOnlyDictionary<IntPtr, IReadOnlyList<Rectangle>> native, Action<IReadOnlyList<Rectangle>> update,
        Func<WindowCaptureTarget, CancellationToken, Task<IReadOnlyList<Rectangle>>>? read = null,
        Func<WindowCaptureTarget, IReadOnlyList<Rectangle>>? readNative = null)
    {
        this.targets = targets; this.native = native.ToDictionary(pair => pair.Key, pair => pair.Value); this.update = update; this.readNative = readNative;
        this.read = read ?? ((target, cancellation) => AccessibleWindowRegions.ReadAsync(target, cancellation));
    }
    internal void Hover(Point point)
    {
        if (disposed) return;
        var target = targets.FirstOrDefault(target => target.Bounds.Contains(point));
        var handle = target?.Handle ?? IntPtr.Zero;
        if (handle != hovered)
        {
            hovered = handle; enteredAt = point; interiorEnabled = false; requested = false;
            if (target is not null) edgeWidth = WindowCaptureTargets.EdgeWidth(target);
            active?.Cancel(); active = null; Pending = Task.CompletedTask;
            Publish();
            return;
        }
        if (target is null) return;
        // A narrow hysteresis zone keeps tiny pointer movements from flickering
        // between the complete window and its interior. Width follows target DPI.
        var enterMargin = Math.Min(edgeWidth + Math.Max(1, edgeWidth / 3), Math.Max(1, Math.Min(target.Bounds.Width, target.Bounds.Height) / 3));
        var hysteresis = Math.Min(Math.Max(1, edgeWidth / 3), Math.Max(1, enterMargin / 3));
        var margin = interiorEnabled ? Math.Max(1, enterMargin - hysteresis) : enterMargin;
        // Native title bars and non-client borders belong to the complete
        // window, even when the caption is thicker than the edge hit zone.
        if (target.ClientBounds is { } client && !client.Contains(point) || NearEdge(target.Bounds, point, margin))
        {
            enteredAt = point;
            if (interiorEnabled) { interiorEnabled = false; Publish(); }
            return;
        }
        if (!interiorEnabled)
        {
            // Ignore startup mouse messages and ordinary pointer jitter. The
            // first intentional movement inside this window enables details.
            var threshold = SystemInformation.DragSize;
            if (Math.Abs((long)point.X - enteredAt.X) * 2 < threshold.Width && Math.Abs((long)point.Y - enteredAt.Y) * 2 < threshold.Height) return;
            interiorEnabled = true;
            Publish();
        }
        if (active is not null || requested || settled.Contains(handle)) return;
        requested = true;
        var request = new CancellationTokenSource(); active = request;
        Pending = RefineAsync(target, request);
    }
    internal static bool NearEdge(Rectangle bounds, Point point, int margin)
    {
        if (!bounds.Contains(point)) return false;
        margin = Math.Clamp(margin, 1, Math.Max(1, Math.Min(bounds.Width, bounds.Height) / 3));
        return (long)point.X - bounds.Left < margin || (long)bounds.Right - point.X <= margin ||
            (long)point.Y - bounds.Top < margin || (long)bounds.Bottom - point.Y <= margin;
    }
    private async Task RefineAsync(WindowCaptureTarget target, CancellationTokenSource request)
    {
        try
        {
            await Task.Delay(100, request.Token);
            if (readNative is not null && !nativeCollected.Contains(target.Handle))
            {
                var nativeRegions = await Task.Run(() => readNative(target), request.Token);
                if (disposed || request.IsCancellationRequested || !ReferenceEquals(active, request)) return;
                native[target.Handle] = nativeRegions; nativeCollected.Add(target.Handle); Publish();
            }
            var elapsed = System.Diagnostics.Stopwatch.StartNew();
            var regions = await read(target, request.Token);
            var confirm = elapsed.ElapsedMilliseconds < 500;
            if (disposed || request.IsCancellationRequested || !ReferenceEquals(active, request)) return;
            Apply(target.Handle, regions);
            // Chromium can expose only its outer accessibility nodes on the
            // first request. Confirm a fast snapshot once, without polling or
            // duplicating slow legacy-provider queries. Keep the first result
            // usable while the provider finishes exposing its document.
            if (confirm)
            {
                await Task.Delay(150, request.Token);
                regions = await read(target, request.Token);
                if (disposed || request.IsCancellationRequested || !ReferenceEquals(active, request)) return;
                Apply(target.Handle, regions);
            }
            settled.Add(target.Handle);
        }
        catch (OperationCanceledException) when (request.IsCancellationRequested) { }
        catch (Exception error) { Console.Error.WriteLine($"Window region refinement unavailable: {error.Message}"); }
        finally
        {
            if (ReferenceEquals(active, request)) active = null;
            request.Dispose();
        }
    }
    private void Apply(IntPtr handle, IReadOnlyList<Rectangle> regions)
    {
        if (cache.TryGetValue(handle, out var previous) && previous.SequenceEqual(regions)) return;
        cache[handle] = regions;
        Publish();
    }
    private void Publish()
    {
        var combined = new List<Rectangle>();
        foreach (var window in targets)
        {
            if (interiorEnabled && window.Handle == hovered)
            {
                if (cache.TryGetValue(window.Handle, out var details)) combined.AddRange(details);
                combined.AddRange(native[window.Handle]);
            }
            else combined.Add(window.Bounds);
        }
        var result = combined.Distinct().ToArray();
        if (published is not null && published.SequenceEqual(result)) return;
        published = result; update(result);
    }
    internal void Suspend()
    {
        if (disposed) return;
        active?.Cancel(); active = null; Pending = Task.CompletedTask;
        hovered = IntPtr.Zero; interiorEnabled = false; requested = false;
        Publish();
    }
    public void Dispose() { disposed = true; active?.Cancel(); active = null; }
}
