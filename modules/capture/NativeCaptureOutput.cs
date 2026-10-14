namespace WinOne.Capture;

internal delegate Task<object> NativeCaptureHandler(SettingsStore settings, CaptureOperations operations, string mode, string? language, CancellationToken cancellation);
internal delegate Task<object> NativeScrollingHandler(SettingsStore settings, CaptureOperations operations, string jobId, string? language, CancellationToken cancellation);
internal delegate Task<object> NativeResumeHandler(SettingsStore settings, CaptureOperations operations, string draftId, string? language, CancellationToken cancellation);

// One native runtime supplies every editor/capture entrypoint. Grouping the
// handlers keeps the worker independent of the canvas and test acquisition.
internal sealed record NativeCaptureHandlers(NativeCaptureHandler? Capture = null)
{
    public NativeScrollingHandler? Scrolling { get; init; }
    public NativeResumeHandler? Resume { get; init; }
    public NativeResumeHandler? EditLast { get; init; }
    public Func<CaptureOperations,string, bool>? DraftMatches { get; init; }
}

// Borrowed bitmaps are consumed synchronously. The UI retains ownership and
// its editing/history state; this port only reuses the product output services.
internal sealed class NativeCaptureOutput(Func<Bitmap, CaptureOutput> save, Func<Bitmap, CaptureOutput> saveAs, Action<Bitmap> confirm, Action<Bitmap> pin)
{
    public CaptureOutput Save(Bitmap image) => save(image);
    public CaptureOutput SaveAs(Bitmap image) => saveAs(image);
    public void Confirm(Bitmap image) => confirm(image);
    public void Pin(Bitmap image) => pin(image);
}

internal sealed partial class CaptureOperations
{
    internal Bitmap LoadLastForNative(string expectedId) => LoadLast(expectedId);
    internal NativeUI.NativeHostConfiguration? NativeConfiguration { get; init; }
    internal Action? ActivateNativeCapture { get; set; }
    internal string? NativeJobId { get; set; }
    internal NativeCaptureOutput CreateNativeOutput(CancellationToken cancellation, string? language = null, CaptureSettings? sessionSettings = null)
    {
        // Match draft sessions: later settings changes apply to the next
        // screenshot, never to the image the user is already editing.
        var settings = sessionSettings ?? store.Snapshot();
        return new NativeCaptureOutput(
            image => Export(image, settings, cancellation, language: language),
            image => Export(image, settings with { AskWhereToSave = true, CopyToClipboard = false, PinAfterCapture = false }, cancellation, language: language),
            image => { cancellation.ThrowIfCancellationRequested(); Copy(image); },
            image => {
                cancellation.ThrowIfCancellationRequested(); ApplyTheme(settings);
                ShareX.PinToScreenForm.OpenManaged(new Bitmap(image), PinOptions(), language, settings.FileNameTemplate);
            });
    }
}
