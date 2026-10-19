using ShareX.ScreenCaptureLib;

namespace WinOne.Capture;

// One result owns one stitched image. Both editing paths consume this result,
// so switching the UI does not add an intermediate PNG or another bitmap copy.
internal sealed record ScrollingCaptureFrame(CapturedFrame Frame, CaptureSettings Settings, string? Notice) : IDisposable
{
    public bool NeedsReview => Settings.OpenEditor || Notice is not null;
    public void Dispose() => Frame.Dispose();
}

internal sealed partial class CaptureOperations
{
    private readonly object scrollGate = new();
    private string? scrollJob;
    private bool scrollStopRequested;

    // The request only records intent. The STA timer applies it to the engine;
    // an old job ID can never stop a newer scrolling capture.
    internal bool StopScrolling(string id)
    {
        lock (scrollGate)
        {
            if (scrollJob != id || string.IsNullOrEmpty(id)) return false;
            scrollStopRequested = true;
            return true;
        }
    }
    private bool ScrollStopRequested { get { lock (scrollGate) return scrollStopRequested; } }

    internal async Task<object> CaptureScrollingAsync(string id, CancellationToken cancellation, string? language, Func<ScrollingCaptureManager, bool>? selectTarget = null, string? replaceId = null, long? replaceRevision = null)
    {
        var current = drafts.Read();
        if (replaceId is null)
        {
            if (current is not null) throw new InvalidOperationException("请先完成或放弃当前截图草稿");
        }
        else if (current is null || current.Id != replaceId || current.SourceKind != "scroll" || current.Revision != replaceRevision)
            throw new InvalidOperationException("截图草稿已变化，请重新加载");
        var settings = store.Snapshot();
        if (replaceId is not null) settings = settings with { OpenEditor = true };
        using var capture = await AcquireScrollingFrameAsync(id, settings, cancellation, language, selectTarget);
        return capture.Frame.WithImage(image => capture.NeedsReview
            ? (object)new CaptureEditorRequest(drafts.Begin(image, capture.Settings, notice: capture.Notice, sourceKind: "scroll", replaceId: replaceId, replaceRevision: replaceRevision).Id)
            : Export(image, capture.Settings with { AskWhereToSave = false }, cancellation, language: language));
    }

    internal async Task<ScrollingCaptureFrame> AcquireScrollingFrameAsync(string id, CaptureSettings settings,
        CancellationToken cancellation, string? language, Func<ScrollingCaptureManager, bool>? selectTarget = null,
        Func<CaptureSettings, IScrollingCaptureSource>? createSource = null, Func<Action, IDisposable>? showProgress = null,
        Rectangle? selectedRegion = null)
    {
        cancellation.ThrowIfCancellationRequested();
        if (string.IsNullOrEmpty(id)) throw new ArgumentException("缺少滚动截图任务标识", nameof(id));
        settings = settings with { ToolbarTools = settings.ToolbarTools?.ToArray() };
        SettingsStore.Validate(settings);
        lock (scrollGate) {
            if (scrollJob is not null) throw new InvalidOperationException("请先完成当前滚动截图");
            scrollJob = id; scrollStopRequested = false;
        }
        try
        {
            // Reject another job before applying its theme or initializing a
            // source, so it cannot disturb an already active capture.
            ApplyTheme(settings);
            var english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
            using var source = createSource is not null ? createSource(settings)
                : new ShareXScrollingCaptureSource(settings,
                    manager => selectTarget?.Invoke(manager) ?? SelectScrollingTarget(manager, language, directConfirm: true), language);
            if (source is null) throw new InvalidOperationException("滚动截图采集不可用");
            using var timer = new System.Windows.Forms.Timer { Interval = 100 };
            timer.Tick += (_, _) => { if (cancellation.IsCancellationRequested || ScrollStopRequested) source.Stop(); };
            if (!source.SelectTarget(selectedRegion)) throw new OperationCanceledException();
            cancellation.ThrowIfCancellationRequested();
            if (ScrollStopRequested) throw new OperationCanceledException();
            using var progress = showProgress?.Invoke(() => StopScrolling(id));
            cancellation.ThrowIfCancellationRequested();
            if (ScrollStopRequested) throw new OperationCanceledException();
            timer.Start();
            var outcome = ScrollingCaptureStatus.Failed;
            string? notice = null;
            try { outcome = await source.CaptureAsync(); }
            catch (InvalidOperationException error) when (error.Message == "WinOne.ScrollingTargetChanged") { notice = "scrollTargetChanged"; }
            cancellation.ThrowIfCancellationRequested();
            var image = source.TakeResult();
            if (image is null) throw new InvalidOperationException(english ? "No scrolling image was captured. Select the area again." : "未生成滚动截图，请重新选择区域。");
            notice ??= ScrollStopRequested ? "scrollStopped" : outcome == ScrollingCaptureStatus.Successful ? null : "scrollPartial";
            try { return new(CapturedFrame.Own(image, new Rectangle(Point.Empty, image.Size)), settings, notice); }
            catch { image.Dispose(); throw; }
        }
        finally
        {
            lock (scrollGate) { scrollJob = null; scrollStopRequested = false; }
        }
    }
}
