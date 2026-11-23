using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// An explicit toolbar action produces a reversible preview on the same HWND.
// Every edit invalidates it; a late response cannot revive an old selection.
internal sealed class NativeInPlaceTranslation(WinOneCaptureCanvas canvas, ICaptureTextService service, bool english) : IDisposable
{
    private CancellationTokenSource? request;
    private Bitmap? translated;
    private Rectangle region;
    private bool disposed;
    internal bool Busy => request is not null;
    internal bool Available => translated is not null;
    internal bool Showing { get; private set; }
    internal string? Error { get; private set; }
    internal string? Status => Busy ? T("正在翻译…", "Translating…") : null;
    internal event Action? Changed;

    internal void Start()
    {
        if (disposed || Busy || canvas.Selection is null) return;
        if (Available) { Toggle(); return; }
        canvas.FinishGesture();
        region = canvas.Selection!.Value;
        Error = null;
        request = new CancellationTokenSource();
        Changed?.Invoke();
        _ = Run(request);
    }

    private async Task Run(CancellationTokenSource lifetime)
    {
        try {
            await canvas.PrepareRenderAsync(lifetime.Token);
            using var source = canvas.RenderOriginalSelection();
            var result = await service.TranslateImage(source, lifetime.Token);
            if (disposed || canvas.Window.IsDisposed || lifetime.IsCancellationRequested || request != lifetime || canvas.Selection != region) {
                result.Picture.Dispose();
                return;
            }
            try { canvas.SetTranslationPreview(result.Picture, region); }
            catch { result.Picture.Dispose(); throw; }
            translated = result.Picture;
            Showing = true;
        } catch (OperationCanceledException) when (lifetime.IsCancellationRequested) { }
        catch (Exception error) {
            if (!disposed && !canvas.Window.IsDisposed && request == lifetime && !lifetime.IsCancellationRequested) {
                Error = error.Message switch {
                    "ocrEmpty" => T("选区中没有可翻译的文字。", "No translatable text in this selection."),
                    "pinUnchanged" => T("译文与原文相同。", "The translation matches the original."),
                    "ocrUnavailable" => T("OCR 文件缺失，请修复安装。", "OCR files are missing. Repair the installation."),
                    "ocrTimeout" => T("识别超时，请缩小选区后重试。", "Recognition timed out. Select a smaller area and retry."),
                    _ => T("原位翻译失败，请检查翻译设置后重试。", "In-place translation failed. Check translation settings and retry.")
                };
            }
        } finally {
            if (request == lifetime) {
                request = null;
                if (!disposed && !canvas.Window.IsDisposed) Changed?.Invoke();
            }
            lifetime.Dispose();
        }
    }

    private string T(string zh, string en) => english ? en : zh;
    internal void Toggle()
    {
        if (disposed || translated is null || Busy) return;
        Showing = !Showing;
        canvas.SetTranslationPreview(Showing ? translated : null, region);
        Changed?.Invoke();
    }

    internal void SelectionChanged()
    {
        if ((Busy || Available) && canvas.Selection != region) Clear();
    }

    internal void Cancel()
    {
        var pending = request;
        request = null;
        pending?.Cancel();
    }

    internal void Clear()
    {
        var changed = Busy || Available || Error is not null;
        Cancel();
        if (!canvas.Window.IsDisposed) canvas.SetTranslationPreview(null, Rectangle.Empty);
        translated?.Dispose();
        translated = null;
        Showing = false;
        Error = null;
        if (changed && !disposed && !canvas.Window.IsDisposed) Changed?.Invoke();
    }

    public void Dispose()
    {
        if (disposed) return;
        disposed = true;
        Clear();
    }
}
