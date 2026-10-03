namespace WinOne.Capture;

internal sealed partial class CaptureOperations
{
    private CaptureHoldWindow? captureHold;
    private void HoldCapture(CapturedFrame frame, Rectangle selection)
    {
        captureHold?.Dispose();
        var hold = new CaptureHoldWindow(frame, selection); captureHold = hold;
        hold.FormClosed += (_, _) => { if (ReferenceEquals(captureHold, hold)) captureHold = null; };
        try { hold.Show(); hold.Refresh(); }
        catch { hold.Dispose(); captureHold = null; throw; }
    }
    private CaptureDraft PrepareHeldCapture(CapturedFrame frame, Rectangle selection, CaptureSettings settings, bool fullscreen = false)
    {
        try
        {
            if (captureHold is null) HoldCapture(frame, selection);
            CaptureDraft draft;
            if (fullscreen)
            {
                using var background = frame.CreatePreview();
                draft = frame.WithImage(image => drafts.Begin(image, settings, CapturePlacement.From(frame.Bounds, frame.Bounds), background));
            }
            else draft = PrepareSelectionEditor(frame, selection, settings);
            captureHold!.Adopt(frame, draft.Id);
            return draft;
        }
        catch { captureHold?.Dispose(); captureHold = null; throw; }
    }
    // Called only on the worker's STA. A stale release cannot remove a newer frame.
    internal bool ReleaseCaptureHold(string id)
    {
        if (captureHold?.DraftId != id) return false;
        captureHold.Dispose(); captureHold = null; return true;
    }
}
