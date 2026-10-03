using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// Connect the existing WinOne intent/refinement policy to the native engine.
// Candidate order and coordinates remain desktop-space values until ShareX
// converts the chosen rectangle into its own client coordinates.
internal sealed class NativeWindowRegions : IDisposable
{
    private readonly WinOneCaptureCanvas capture;
    private readonly WindowRegionRefinement refinement;
    private bool pausedForGesture;
    public Task Pending => refinement.Pending;
    public NativeWindowRegions(WinOneCaptureCanvas capture, IReadOnlyList<WindowCaptureTarget> targets,
        Func<WindowCaptureTarget, CancellationToken, Task<IReadOnlyList<Rectangle>>>? read = null,
        Func<WindowCaptureTarget, IReadOnlyList<Rectangle>>? readNative = null)
    {
        this.capture = capture;
        var initial = targets.ToDictionary(target => target.Handle, target => (IReadOnlyList<Rectangle>)new[] { target.Bounds });
        capture.UpdateWindowCandidates(targets.Select(target => target.Bounds).ToArray());
        if (read is null && readNative is null) readNative = target => {
            if (ShareX.HelpersLib.CaptureHelpers.GetWindowRectangle(target.Handle) != target.Bounds)
                throw new InvalidOperationException("窗口布局已变化，请重新截图");
            return WindowCaptureTargets.Regions(new[] { target });
        };
        refinement = new WindowRegionRefinement(targets, initial, regions => capture.UpdateWindowCandidates(regions.ToArray()), read, readNative);
        capture.PointerMoved += Hover;
        capture.ToolChanged += ToolChanged;
    }
    public void Hover(Point point)
    {
        if (capture.SelectedTool != "region") return;
        if (capture.RegionGestureActive) {
            if (!pausedForGesture) { pausedForGesture = true; refinement.Suspend(); }
            return;
        }
        pausedForGesture = false; refinement.Hover(point);
    }
    private void ToolChanged(string tool) { if (tool != "region") { pausedForGesture = false; refinement.Suspend(); } }
    public void Dispose() { capture.PointerMoved -= Hover; capture.ToolChanged -= ToolChanged; refinement.Dispose(); }
}
