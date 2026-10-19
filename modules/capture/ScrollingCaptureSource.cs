using ShareX.ScreenCaptureLib;

namespace WinOne.Capture;

// The producer owns capture and stitching; the workflow owns task cancellation
// and the returned frame. A source never opens the result editor itself.
internal interface IScrollingCaptureSource : IDisposable
{
    bool SelectTarget(Rectangle? selectedRegion = null);
    Task<ScrollingCaptureStatus> CaptureAsync();
    void Stop();
    Bitmap? TakeResult();
}

internal sealed class ShareXScrollingCaptureSource : IScrollingCaptureSource
{
    private readonly ScrollingCaptureManager manager;
    private readonly Func<ScrollingCaptureManager, bool> select;
    private readonly string? language;
    public ShareXScrollingCaptureSource(CaptureSettings settings, Func<ScrollingCaptureManager, bool> select, string? language = null)
    {
        this.select = select; this.language = language;
        manager = new(new ScrollingCaptureOptions {
            AutoUpload = false, ScrollDelay = settings.ScrollDelay, AutoScrollTop = settings.ScrollFromTop
        });
    }
    public bool SelectTarget(Rectangle? selectedRegion = null) => selectedRegion is { } region
        ? CaptureOperations.SelectScrollingTarget(manager, region, WindowCaptureTargets.Snapshot(), language)
        : select(manager);
    public Task<ScrollingCaptureStatus> CaptureAsync() => manager.StartCapture();
    public void Stop() => manager.StopCapture();
    public Bitmap? TakeResult() => manager.TakeResult();
    public void Dispose() => manager.Dispose();
}
