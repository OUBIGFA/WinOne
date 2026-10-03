using ShareX.ScreenCaptureLib;

namespace WinOne.Capture;

// The producer owns capture and stitching; the workflow owns task cancellation
// and the returned frame. A source never opens the result editor itself.
internal interface IScrollingCaptureSource : IDisposable
{
    bool SelectTarget();
    Task<ScrollingCaptureStatus> CaptureAsync();
    void Stop();
    Bitmap? TakeResult();
}

internal sealed class ShareXScrollingCaptureSource : IScrollingCaptureSource
{
    private readonly ScrollingCaptureManager manager;
    private readonly Func<ScrollingCaptureManager, bool> select;
    public ShareXScrollingCaptureSource(CaptureSettings settings, Func<ScrollingCaptureManager, bool> select)
    {
        this.select = select;
        manager = new(new ScrollingCaptureOptions {
            AutoUpload = false, ScrollDelay = settings.ScrollDelay, AutoScrollTop = settings.ScrollFromTop
        });
    }
    public bool SelectTarget() => select(manager);
    public Task<ScrollingCaptureStatus> CaptureAsync() => manager.StartCapture();
    public void Stop() => manager.StopCapture();
    public Bitmap? TakeResult() => manager.TakeResult();
    public void Dispose() => manager.Dispose();
}
