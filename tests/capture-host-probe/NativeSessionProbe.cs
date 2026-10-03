using System.Diagnostics;
using System.Drawing.Imaging;
using System.Text;
using System.Text.Json;
using ShareX.ScreenCaptureLib;

internal interface ICaptureProbe : IDisposable
{
    bool Failed { get; }
}

// Test-only observation: the product session never writes JSON, creates a
// WebView or depends on fixture paths. Existing drivers observe its real UI.
internal sealed class NativeSessionProbe : ICaptureProbe
{
    private readonly WinOneCaptureCanvas capture;
    private readonly NativeCaptureSession session;
    private readonly string output, initialWindow;
    private readonly Stopwatch clock;
    private readonly bool dark;
    private readonly bool ownsSession;
    private readonly double? launchToEntryMs;
    private readonly IReadOnlyDictionary<string, double>? startupStages;
    private double? firstPaintMs;
    private int saved, savedAs, saveAsAttempts;
    private NativeSnapshotServer? snapshots;
    public bool Failed => false;
    public bool Confirmed => session.Confirmed;
    public bool Pinned => session.Pinned;

    public NativeSessionProbe(WinOneCaptureCanvas capture, NativeCaptureSession session, string output, Stopwatch clock, bool dark)
    {
        this.capture = capture; this.session = session; this.output = output; this.clock = clock; this.dark = dark;
        initialWindow = capture.Window.Handle.ToInt64().ToString();
        session.Updated += Publish; Publish();
    }

    public NativeSessionProbe(WinOneCaptureCanvas capture, string assets, string output, Stopwatch clock,
        Action<Bitmap> copy, double? launchToEntryMs = null, IReadOnlyDictionary<string, double>? startupStages = null,
        Action<Bitmap>? saveImage = null, Action<Bitmap>? saveAsImage = null, bool dark = false, Action<Bitmap>? pinImage = null)
    {
        this.capture = capture; this.output = output; this.clock = clock; this.dark = dark;
        ownsSession = true;
        this.launchToEntryMs = launchToEntryMs; this.startupStages = startupStages;
        initialWindow = capture.Window.Handle.ToInt64().ToString();
        session = new NativeCaptureSession(capture, copy, image => {
            if (saveImage is null) image.Save(Path.Combine(output, $"capture-{saved + 1:00}.png"), ImageFormat.Png);
            else saveImage(image);
            saved++;
        }, saveAsImage is null ? null : image => { saveAsAttempts++; saveAsImage(image); savedAs++; }, pinImage, dark);
        session.Updated += Publish;
        Publish();
    }

    private void Publish()
    {
        if (session.Ready && firstPaintMs is null) firstPaintMs = clock.Elapsed.TotalMilliseconds;
        var toolbar = session.Toolbar;
        var text = session.EditingText;
        var input = session.TextInput;
        // Keep the old driver's coordinate convention while removing the
        // browser viewport itself from the production UI.
        var viewport = text is null ? Rectangle.Empty : Rectangle.Inflate(text.Bounds, 2, 2);
        var state = JsonSerializer.Serialize(new {
            kind = "state", requestId = 0, nativeTheme = dark ? "dark" : "light", nativeControls = true,
            browserReady = false, nativeProcess = Environment.ProcessId, nativeDesktop = HostedToolbar.CurrentDesktopName(),
            nativeWindow = capture.Window.Handle.ToInt64().ToString(), initialNativeWindow = initialWindow,
            toolbarRoot = toolbar.TopLevelControl?.Handle.ToInt64().ToString(), annotations = capture.AnnotationCount,
            canUndo = capture.CanUndo, canRedo = capture.CanRedo, tool = capture.Tool, selectedTool = capture.SelectedTool,
            textTool = session.TextTool, copied = session.Copied, closeRequested = session.CloseRequested, pinned = session.Pinned,
            error = session.Error, hasSelection = capture.Selection is not null, nativeToolbarVisible = toolbar.Visible,
            windowCandidates = capture.ActiveWindowCandidates, selectionHintVisible = session.HintVisible,
            nativeForms = Application.OpenForms.Count, inputRegion = (object?)null,
            viewWidth = viewport.Width, viewX = viewport.X, viewY = viewport.Y,
            toolbar = new { x = toolbar.Left - viewport.X, y = toolbar.Top - viewport.Y, width = toolbar.Width, height = toolbar.Height },
            text = text is null ? null : new { id = text.Id, x = text.Bounds.X - viewport.X, y = text.Bounds.Y - viewport.Y,
                width = text.Bounds.Width, height = text.Bounds.Height, value = text.Value, fontPixels = text.FontPixels,
                existing = text.Target.HasValue, imageWidth = capture.Window.ClientSize.Width - viewport.X,
                imageHeight = text.Bounds.Top + text.MaxHeight - viewport.Y },
            firstPaintMs, toolbarReadyMs = firstPaintMs, initializationStartedMs = (double?)null,
            environmentReadyMs = (double?)null, controllerReadyMs = (double?)null, domReadyMs = (double?)null,
            navigationReadyMs = (double?)null, visibleInitialization = false, saved, savedAs, saveAsAttempts,
            nativeTextWindow = input?.IsHandleCreated == true ? input.Handle.ToInt64().ToString() : null,
            nativeTextValue = input?.Text, nativeGrip = toolbar.DescribeGrip(), nativeButtons = toolbar.DescribeButtons(),
            startupStages, launchToEntryMs, launchToFirstPaintMs = launchToEntryMs + firstPaintMs,
            launchToToolbarReadyMs = launchToEntryMs + firstPaintMs
        });
        if (snapshots is null) snapshots = new NativeSnapshotServer(output, state);
        else snapshots.Update(state);
    }

    public void Dispose()
    {
        session.Updated -= Publish;
        try { snapshots?.Dispose(); }
        finally { if (ownsSession) session.Dispose(); }
    }
}
