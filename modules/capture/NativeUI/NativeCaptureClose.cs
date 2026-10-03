namespace WinOne.Capture.NativeUI;

// FormClosing stays on the existing canvas until persistence accepts the close.
// An asynchronous failure restores its controls and allows a user-initiated
// retry. No replacement window, timer or second output operation is involved.
internal sealed class NativeCaptureClose : IDisposable
{
    private readonly Form window;
    private readonly NativeCaptureSession session;
    private readonly Func<bool, Task> finish;
    private bool pending, approved, disposed, preserveNext;
    public bool Accepted => approved;
    public NativeCaptureClose(Form window, NativeCaptureSession session, Func<bool, Task> finish)
    {
        this.window = window; this.session = session; this.finish = finish;
        window.FormClosing += Closing;
    }
    public void PreserveAndClose() { preserveNext = true; window.Close(); }
    private async void Closing(object? sender, FormClosingEventArgs args)
    {
        if (approved) return;
        args.Cancel = true;
        if (pending) return;
        pending = true;
        var preserve = preserveNext; preserveNext = false;
        session.SetClosing(true);
        try {
            await finish(preserve);
            if (disposed || window.IsDisposed) return;
            approved = true; pending = false;
            // Defer the second Close even when finish completed synchronously,
            // so this FormClosing invocation can unwind before closing again.
            window.BeginInvoke((Action)(() => { if (!disposed && !window.IsDisposed) window.Close(); }));
        } catch (Exception error) {
            pending = false;
            if (disposed || window.IsDisposed) return;
            session.SetClosing(false);
            session.ReportPersistenceError(error.Message);
        }
    }
    public void Dispose() { disposed = true; window.FormClosing -= Closing; }
}
