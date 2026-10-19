namespace WinOne.Capture.NativeUI;

internal sealed partial class NativeCaptureSession
{
    private Func<string, Task<RecordingStatus?>>? recording;
    private bool recordingActive;
    internal RecordingStatus? Recorded { get; private set; }
    internal Task RecordingCompletion { get; private set; } = Task.CompletedTask;

    internal void EnableRecording(Func<string, Task<RecordingStatus?>> handler)
    {
        recording = handler; nativeToolbar.SetRecordingAvailable(true);
    }
    private void BeginRecording(string format)
    {
        if (recording is null || recordingActive || retaking || closing || preparingImage || Finalized || textDraft is not null || capture.Selection is null) return;
        // Install the completion task before a native file picker can enter its
        // nested message loop. Host shutdown awaits this same lifetime.
        var completion = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        RecordingCompletion = completion.Task; recordingActive = true;
        capture.FinishGesture(); nativeToolbar.Enabled = false; capture.SetTextEditing(true);
        _ = RunRecording(format, completion);
    }
    private async Task RunRecording(string format, TaskCompletionSource completion)
    {
        string? error = null;
        try {
            Recorded = await recording!(format);
            if (Recorded is not null && !capture.Window.IsDisposed) capture.Window.Close();
        } catch (OperationCanceledException) { }
        catch (Exception failure) { error = failure.Message; }
        finally {
            recordingActive = false;
            try {
                if (!disposed && !capture.Window.IsDisposed) {
                    nativeToolbar.Enabled = !closing; capture.SetTextEditing(closing || Finalized || nativeText is not null);
                    Refresh(error);
                }
            } finally { completion.TrySetResult(); }
        }
    }
}
