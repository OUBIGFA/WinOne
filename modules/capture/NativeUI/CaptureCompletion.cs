namespace WinOne.Capture.NativeUI;

// One completion transaction for native gestures and toolbar commands.
internal sealed class CaptureCompletion(Func<Bitmap> render, Action<Bitmap> commit, Action close, Func<bool>? isClosed = null)
{
    private bool executing;
    public bool Committed { get; private set; }
    public bool CloseRequested { get; private set; }

    public void Complete()
    {
        if (executing || CloseRequested) return;
        executing = true;
        try {
            if (!Committed) {
                using var image = render();
                commit(image);
                Committed = true;
            }
            // If closing itself fails, retry only the close, not the output action.
            close();
            CloseRequested = CloseRequested || (isClosed?.Invoke() ?? true);
        } finally { executing = false; }
    }
    public void NotifyClosed() { if (Committed) CloseRequested = true; }
}
