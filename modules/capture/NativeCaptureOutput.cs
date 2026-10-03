namespace WinOne.Capture;

internal delegate object NativeCaptureHandler(SettingsStore settings, CaptureOperations operations, string mode, string? language, CancellationToken cancellation);

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
    internal NativeUI.NativeHostConfiguration? NativeConfiguration { get; init; }
    internal Action? ActivateNativeCapture { get; set; }
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
