using System.Runtime.InteropServices;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

internal static class NativeCaptureRetake
{
    [DllImport("dwmapi.dll")] private static extern int DwmFlush();

    internal static async Task<NativeCaptureDraft> RunAsync(WinOneCaptureCanvas canvas, NativeCaptureSession session,
        CaptureOperations operations, CaptureSettings settings, NativeCaptureDraftStore drafts, NativeCaptureAutosave autosave,
        string? language, CancellationToken cancellation, Action<NativeCaptureDraft> committed,
        Func<CaptureSettings,IScrollingCaptureSource>? createSource = null, Func<Action,IDisposable>? showProgress = null)
    {
        canvas.FinishGesture();
        var snapshot = session.Snapshot() ?? throw new InvalidOperationException("请先完成当前选区调整");
        autosave.Publish(snapshot);
        var current = await autosave.FlushAsync();
        cancellation.ThrowIfCancellationRequested();
        var visible = canvas.Window.Visible;
        try {
            if (visible) { canvas.Window.Hide(); if (createSource is null && DwmFlush() < 0) throw new InvalidOperationException("无法准备重截画面，请重试"); }
            using var acquired = await operations.AcquireScrollingFrameAsync(operations.NativeJobId ?? Guid.NewGuid().ToString("N"),
                settings with { OpenEditor=true }, cancellation, language, createSource:createSource,
                showProgress:showProgress ?? (stop=>NativeScrollProgress.Open(stop,ShareXResources.Theme.IsDarkTheme,language)));
            cancellation.ThrowIfCancellationRequested();
            using var prepared = await Task.Run(()=>acquired.Frame.WithImage(WinOneCanvasReplacement.Prepare),cancellation);
            var document = new WinOneCaptureDocument(1,prepared.Size.Width,prepared.Size.Height,
                new Rectangle(Point.Empty,prepared.Size),"select",[]);
            var next = await Task.Run(()=>acquired.Frame.WithImage(image=>drafts.ReplaceScroll(current.Id,current.Revision,
                image,acquired.Settings,document,acquired.Notice,cancellation)),cancellation);
            // Once metadata commits, finish the in-memory swap even if parent
            // shutdown arrived meanwhile; the close guard waits for this task.
            committed(next);
            canvas.ApplyReplacement(prepared);
            session.SetCaptureNotice(acquired.Notice,language);
            return next;
        } finally {
            if (visible && !canvas.Window.IsDisposed && !session.Closing && !cancellation.IsCancellationRequested)
                canvas.Window.Show();
        }
    }
}
