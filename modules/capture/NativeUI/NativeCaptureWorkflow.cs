using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// Acquires one immutable desktop frame before showing the native session.
// Tests replace acquisition and the modal runner, never the user's clipboard.
internal static class NativeCaptureWorkflow
{
    internal static async Task<object> RunScrollingAsync(SettingsStore store, CaptureOperations operations, string id,
        string? language, CancellationToken cancellation, NativeCaptureDraftStore drafts,
        Func<CaptureSettings, IScrollingCaptureSource>? createSource = null,
        Action<WinOneCaptureCanvas, NativeCaptureSession>? show = null, Func<Action, IDisposable>? showProgress = null)
    {
        cancellation.ThrowIfCancellationRequested();
        if (operations.GetDraft() is not null) throw new InvalidOperationException("请先完成或放弃当前截图草稿");
        var recovered = NativeCaptureRecovery.Read(drafts, language, cancellation);
        var settings = store.Snapshot();
        if (recovered is not null) {
            using var restored = RecoverFrame(drafts, recovered);
            settings = recovered.Settings with { Theme = settings.Theme, ToolbarTools = settings.ToolbarTools };
            CaptureOperations.ApplyTheme(settings);
            return RunFrame(restored, settings, operations, recovered.Mode, language, cancellation, false,
                recovered.Mode == "scroll" || restored.Bounds != CaptureHelpers.GetScreenBounds(), recovered, null, show, drafts, recovered.Notice);
        }
        using var capture = await operations.AcquireScrollingFrameAsync(id, settings, cancellation, language, createSource: createSource,
            showProgress: showProgress ?? (stop => NativeScrollProgress.Open(stop, ShareXResources.Theme.IsDarkTheme, language)));
        cancellation.ThrowIfCancellationRequested();
        if (!capture.NeedsReview) {
            var output = operations.CreateNativeOutput(cancellation, language, capture.Settings with { AskWhereToSave = false });
            return capture.Frame.WithImage(image => output.Save(image));
        }
        return EditScrollingFrame(capture.Frame, capture.Settings, operations, capture.Notice, language, cancellation, drafts, show);
    }

    internal static object Run(SettingsStore store, CaptureOperations operations, string mode, string? language,
        CancellationToken cancellation, Func<CapturedFrame>? acquire = null,
        Func<IReadOnlyList<WindowCaptureTarget>>? windows = null,
        Action<WinOneCaptureCanvas, NativeCaptureSession>? show = null, NativeCaptureDraftStore? drafts = null)
    {
        if (mode is not ("window" or "region" or "fullscreen")) throw new ArgumentException("不支持的截图方式", nameof(mode));
        cancellation.ThrowIfCancellationRequested();
        var recovered = drafts is null ? null : NativeCaptureRecovery.Read(drafts, language, cancellation);
        if (recovered is not null) mode = recovered.Mode;
        var currentSettings = store.Snapshot();
        var settings = recovered is null ? currentSettings : recovered.Settings with { Theme = currentSettings.Theme, ToolbarTools = currentSettings.ToolbarTools };
        var quickSave = recovered is null && !settings.OpenEditor;
        if (quickSave) settings = settings with { AskWhereToSave = false };
        CaptureOperations.ApplyTheme(settings);
        windows ??= WindowCaptureTargets.Snapshot;
        var targets = recovered is null && mode == "window" ? windows() : null;
        using var frame = recovered is null ? (acquire ?? operations.CaptureDesktopFrame)() : RecoverFrame(drafts!, recovered);
        cancellation.ThrowIfCancellationRequested();
        if (targets is not null && !targets.SequenceEqual(windows())) throw new InvalidOperationException("窗口布局已变化，请重新截图");
        return RunFrame(frame, settings, operations, mode, language, cancellation, quickSave,
            mode == "scroll" || recovered is not null && frame.Bounds != CaptureHelpers.GetScreenBounds(), recovered, targets, show, drafts, recovered?.Notice);
    }

    // The scrolling producer transfers its owned frame into the same editor.
    // No PNG/browser handoff or second screenshot is needed to begin editing.
    internal static object EditScrollingFrame(CapturedFrame frame, CaptureSettings settings, CaptureOperations operations,
        string? notice, string? language, CancellationToken cancellation, NativeCaptureDraftStore? drafts = null,
        Action<WinOneCaptureCanvas, NativeCaptureSession>? show = null)
    {
        cancellation.ThrowIfCancellationRequested();
        settings = settings with { ToolbarTools = settings.ToolbarTools?.ToArray() };
        SettingsStore.Validate(settings);
        CaptureOperations.ApplyTheme(settings);
        return RunFrame(frame, settings, operations, "scroll", language, cancellation, false, true, null, null, show, drafts, notice);
    }

    private static object RunFrame(CapturedFrame frame, CaptureSettings settings, CaptureOperations operations,
        string mode, string? language, CancellationToken cancellation, bool quickSave, bool viewport,
        NativeCaptureDraft? recovered, IReadOnlyList<WindowCaptureTarget>? targets,
        Action<WinOneCaptureCanvas, NativeCaptureSession>? show, NativeCaptureDraftStore? drafts, string? notice)
    {
        var output = operations.CreateNativeOutput(cancellation, language, settings);
        if (mode == "fullscreen" && quickSave) return frame.WithImage(image => output.Save(image));
        using var canvas = frame.WithImage(image => new WinOneCaptureCanvas(image, viewport: viewport));
        // The native engine uses desktop bounds to map candidates to pixels.
        // Reject display changes rather than stretching or offsetting a frame.
        if (!viewport && canvas.Window.Bounds != frame.Bounds) throw new InvalidOperationException("显示设置已变化，请重新截图");
        // The engine owns its canvas copy. Fresh editable captures transfer the
        // acquisition buffer to one background encoder; other paths release it.
        if (drafts is null || quickSave || recovered is not null) frame.Dispose();
        using var regions = targets is null ? null : new NativeWindowRegions(canvas, targets);
        if (recovered is not null) canvas.RestoreDocument(recovered.Document);
        else if (mode is "fullscreen" or "scroll") canvas.SetRegion(new Rectangle(Point.Empty, frame.Bounds.Size));
        CaptureOutput? saved = null;
        using var session = new NativeCaptureSession(canvas,
            image => { if (quickSave) saved = output.Save(image); else output.Confirm(image); },
            image => { saved = output.Save(image); }, image => { saved = output.SaveAs(image); },
            output.Pin, ShareXResources.Theme.IsDarkTheme, quickSave, language, settings.ToolbarTools,
            operations.NativeConfiguration is { } configuration ? tools => configuration.SaveToolbarAsync(tools, cancellation) : null);
        session.SetCaptureNotice(notice, language);
        if (recovered?.Text is { } text) session.RestoreText(text);
        var autosave = drafts is null || quickSave ? null : recovered is null
            ? new NativeCaptureAutosave(drafts, frame.Detach(), settings, canvas.ExportDocument(), mode,
                mode == "scroll" ? null : CapturePlacement.From(frame.Bounds, frame.Bounds), notice)
            : new NativeCaptureAutosave(drafts, recovered);
        if (autosave is not null) session.SnapshotReady += autosave.Publish;
        using var close = autosave is null ? null : new NativeCaptureClose(canvas.Window, session, async preserve => {
            // Take the final input snapshot before the close disposes controls.
            if (preserve && session.Snapshot() is { } snapshot) autosave.Publish(snapshot);
            var cleanup = await autosave.FinishAsync(!preserve);
            foreach (var error in cleanup.Errors) Console.Error.WriteLine("Native draft cleanup: " + error);
        });
        // Cancellation is marshalled to the owning UI thread. Registration is
        // disposed before the session/window, so no callback outlives the HWND.
        using var cancel = cancellation.Register(() => {
            if (!canvas.Window.IsDisposed && canvas.Window.IsHandleCreated)
                canvas.Window.BeginInvoke((Action)(() => {
                    if (canvas.Window.IsDisposed) return;
                    if (close is not null) close.PreserveAndClose(); else canvas.Window.Close();
                }));
        });
        var returned = false;
        operations.ActivateNativeCapture = () => {
            if (canvas.Window.IsDisposed || !canvas.Window.Visible) return;
            if (canvas.Window.WindowState == FormWindowState.Minimized) canvas.Window.WindowState = FormWindowState.Normal;
            canvas.Window.BringToFront(); canvas.Window.Activate();
        };
        try {
            cancellation.ThrowIfCancellationRequested();
            if (show is null) canvas.ShowDialog(); else show(canvas, session);
            returned = true;
        } finally {
            operations.ActivateNativeCapture = null;
            if (autosave is not null) {
                session.SnapshotReady -= autosave.Publish;
                if (close?.Accepted != true) {
                    var cleanup = autosave.FinishAsync(returned && !cancellation.IsCancellationRequested).GetAwaiter().GetResult();
                    foreach (var error in cleanup.Errors) Console.Error.WriteLine("Native draft cleanup: " + error);
                }
            }
        }
        cancellation.ThrowIfCancellationRequested();
        if (quickSave && session.Confirmed && saved is not null) return saved;
        if (!session.Confirmed && !session.Pinned) throw new OperationCanceledException();
        return new { mode, copied = session.Copied, pinned = session.Pinned };
    }

    private static CapturedFrame RecoverFrame(NativeCaptureDraftStore store, NativeCaptureDraft draft)
    {
        var placement = draft.Placement;
        if (placement is null && draft.Mode != "scroll") throw new InvalidDataException("截图缺少显示位置，暂时无法恢复");
        var image = store.LoadImage(draft.Id, draft.Revision);
        try { return CapturedFrame.Own(image, new Rectangle(placement?.X ?? 0, placement?.Y ?? 0, image.Width, image.Height)); }
        catch { image.Dispose(); throw; }
    }
}
