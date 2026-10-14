using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// Acquires one immutable desktop frame before showing the native session.
// Tests replace acquisition and the modal runner, never the user's clipboard.
internal static class NativeCaptureWorkflow
{
    internal static async Task<object> EditLast(SettingsStore store, CaptureOperations operations, string expectedId,
        string? language, CancellationToken cancellation, NativeCaptureDraftStore drafts,
        Action<WinOneCaptureCanvas, NativeCaptureSession>? show = null)
    {
        cancellation.ThrowIfCancellationRequested();
        // Editing a saved capture must not overwrite either pending format.
        if (operations.GetDraft() is not null || drafts.Read() is not null || drafts.ReadReservation() is not null)
            throw new InvalidOperationException("请先完成或放弃当前截图草稿");
        var image = operations.LoadLastForNative(expectedId);
        CapturedFrame frame;
        try { frame = CapturedFrame.Own(image, new Rectangle(Point.Empty, image.Size)); }
        catch { image.Dispose(); throw; }
        using var ownedFrame = frame;
        var settings = store.Snapshot() with { OpenEditor = true };
        CaptureOperations.ApplyTheme(settings);
        return await RunFrame(frame, settings, operations, "fullscreen", language, cancellation, false, true, null, null, show, drafts, null);
    }

    // Resume only the requested persisted generation; unlike a fresh capture,
    // an absent or incomplete draft must never silently acquire a new screen.
    internal static async Task<object> Resume(SettingsStore store, CaptureOperations operations, string expectedId,
        string? language, CancellationToken cancellation, NativeCaptureDraftStore drafts,
        Action<WinOneCaptureCanvas, NativeCaptureSession>? show = null)
    {
        cancellation.ThrowIfCancellationRequested();
        if (operations.GetDraft() is not null) throw new InvalidOperationException("请先处理现有截图草稿");
        var recovered = drafts.Recover();
        if (recovered is null || recovered.Id != expectedId) throw new InvalidOperationException("截图草稿已变化，请重新加载");
        var current = store.Snapshot();
        var settings = recovered.Settings with { Theme = current.Theme, ToolbarTools = current.ToolbarTools };
        CaptureOperations.ApplyTheme(settings);
        using var frame = RecoverFrame(drafts, recovered);
        return await RunFrame(frame, settings, operations, recovered.Mode, language, cancellation, false,
            recovered.Mode == "scroll" || recovered.Document.OrderedEffects || frame.Bounds != CaptureHelpers.GetScreenBounds(), recovered, null, show, drafts, recovered.Notice);
    }

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
            return await RunFrame(restored, settings, operations, recovered.Mode, language, cancellation, false,
                recovered.Mode == "scroll" || recovered.Document.OrderedEffects || restored.Bounds != CaptureHelpers.GetScreenBounds(), recovered, null, show, drafts, recovered.Notice, createSource, showProgress);
        }
        using var capture = await operations.AcquireScrollingFrameAsync(id, settings, cancellation, language, createSource: createSource,
            showProgress: showProgress ?? (stop => NativeScrollProgress.Open(stop, ShareXResources.Theme.IsDarkTheme, language)));
        cancellation.ThrowIfCancellationRequested();
        if (!capture.NeedsReview) {
            var output = operations.CreateNativeOutput(cancellation, language, capture.Settings with { AskWhereToSave = false });
            return capture.Frame.WithImage(image => output.Save(image));
        }
        return await EditScrollingFrame(capture.Frame, capture.Settings, operations, capture.Notice, language, cancellation, drafts, show, createSource, showProgress);
    }

    internal static async Task<object> Run(SettingsStore store, CaptureOperations operations, string mode, string? language,
        CancellationToken cancellation, Func<CapturedFrame>? acquire = null,
        Func<IReadOnlyList<WindowCaptureTarget>>? windows = null,
        Action<WinOneCaptureCanvas, NativeCaptureSession>? show = null, NativeCaptureDraftStore? drafts = null)
    {
        var textMode = mode is "ocr" or "translate" ? mode : null;
        if (textMode is not null) mode = "region";
        if (mode is not ("window" or "region" or "fullscreen")) throw new ArgumentException("不支持的截图方式", nameof(mode));
        if(operations.GetDraft() is not null)throw new InvalidOperationException("请先处理旧截图草稿");
        cancellation.ThrowIfCancellationRequested();
        var recovered = drafts is null ? null : NativeCaptureRecovery.Read(drafts, language, cancellation);
        if (recovered is not null) mode = recovered.Mode;
        var currentSettings = store.Snapshot();
        var settings = recovered is null ? currentSettings : recovered.Settings with { Theme = currentSettings.Theme, ToolbarTools = currentSettings.ToolbarTools };
        var quickSave = textMode is null && recovered is null && !settings.OpenEditor;
        if (quickSave) settings = settings with { AskWhereToSave = false };
        CaptureOperations.ApplyTheme(settings);
        windows ??= WindowCaptureTargets.Snapshot;
        var targets = recovered is null && mode == "window" ? windows() : null;
        using var frame = recovered is null ? (acquire ?? operations.CaptureDesktopFrame)() : RecoverFrame(drafts!, recovered);
        cancellation.ThrowIfCancellationRequested();
        if (targets is not null && !targets.SequenceEqual(windows())) throw new InvalidOperationException("窗口布局已变化，请重新截图");
        return await RunFrame(frame, settings, operations, mode, language, cancellation, quickSave,
            mode == "scroll" || recovered?.Document.OrderedEffects==true || recovered is not null && frame.Bounds != CaptureHelpers.GetScreenBounds(), recovered, targets, show, drafts, recovered?.Notice, textMode: recovered is null ? textMode : null);
    }

    // The scrolling producer transfers its owned frame into the same editor.
    // No PNG/browser handoff or second screenshot is needed to begin editing.
    internal static async Task<object> EditScrollingFrame(CapturedFrame frame, CaptureSettings settings, CaptureOperations operations,
        string? notice, string? language, CancellationToken cancellation, NativeCaptureDraftStore? drafts = null,
        Action<WinOneCaptureCanvas, NativeCaptureSession>? show = null,
        Func<CaptureSettings,IScrollingCaptureSource>? retakeSource = null, Func<Action,IDisposable>? retakeProgress = null)
    {
        cancellation.ThrowIfCancellationRequested();
        settings = settings with { ToolbarTools = settings.ToolbarTools?.ToArray() };
        SettingsStore.Validate(settings);
        CaptureOperations.ApplyTheme(settings);
        return await RunFrame(frame, settings, operations, "scroll", language, cancellation, false, true, null, null, show, drafts, notice, retakeSource, retakeProgress);
    }

    private static async Task<object> RunFrame(CapturedFrame frame, CaptureSettings settings, CaptureOperations operations,
        string mode, string? language, CancellationToken cancellation, bool quickSave, bool viewport,
        NativeCaptureDraft? recovered, IReadOnlyList<WindowCaptureTarget>? targets,
        Action<WinOneCaptureCanvas, NativeCaptureSession>? show, NativeCaptureDraftStore? drafts, string? notice,
        Func<CaptureSettings,IScrollingCaptureSource>? retakeSource = null, Func<Action,IDisposable>? retakeProgress = null, string? textMode = null)
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
            operations.NativeConfiguration is { } configuration ? tools => configuration.SaveToolbarAsync(tools, cancellation) : null,
            new NativeCaptureTextService(operations.NativeConfiguration), textMode);
        session.SetCaptureNotice(notice, language);
        if (recovered?.Text is { } text) session.RestoreText(text);
        // Show a recovered draft with its annotations already present. The
        // initial ordered composition also runs off the worker's UI thread.
        if (recovered?.Document.OrderedEffects == true) await canvas.PrepareRenderAsync(cancellation);
        var autosave = drafts is null || quickSave ? null : recovered is null
            ? new NativeCaptureAutosave(drafts, frame.Detach(), settings, canvas.ExportDocument(), mode,
                mode == "scroll" ? null : CapturePlacement.From(frame.Bounds, frame.Bounds), notice)
            : new NativeCaptureAutosave(drafts, recovered);
        var retakeViewFailed = false;
        void Publish(NativeCaptureSnapshot snapshot) => autosave?.Publish(snapshot);
        if (autosave is not null) session.SnapshotReady += Publish;
        using var close = autosave is null ? null : new NativeCaptureClose(canvas.Window, session, async preserve => {
            await session.RetakeCompletion;
            // If metadata committed but presentation failed, keep that new
            // image recoverable; never overwrite it with the old canvas.
            if (retakeViewFailed) { await autosave!.FinishAsync(false); return; }
            // Take the final input snapshot before the close disposes controls.
            if (preserve && session.Snapshot() is { } snapshot) autosave!.Publish(snapshot);
            var cleanup = await autosave!.FinishAsync(!preserve);
            foreach (var error in cleanup.Errors) Console.Error.WriteLine("Native draft cleanup: " + error);
        });
        if (autosave is not null && drafts is not null) {
            session.EnableRetake(async () => {
                var committed = false;
                try {
                    var next = await NativeCaptureRetake.RunAsync(canvas,session,operations,settings,drafts,autosave!,language,cancellation,
                        _=>committed=true,retakeSource,retakeProgress);
                    await autosave!.FinishAsync(false);
                    autosave = new NativeCaptureAutosave(drafts,next);
                } catch (Exception error) {
                    if (committed) {
                        retakeViewFailed = true;
                        var message = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en",StringComparison.OrdinalIgnoreCase)
                            ? "The retake was saved but could not be displayed. Right-click to close, then reopen capture to recover it."
                            : "重截已保存，但暂时无法显示。请右键关闭，再次截图以恢复。";
                        session.ReportReplacementFailure(message);
                        throw new InvalidOperationException(message,error);
                    }
                    throw;
                }
            }, mode == "scroll");
        }
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
            if (show is not null) show(canvas, session);
            else {
                // A modal ShowDialog loop ends when the form is hidden. Retake
                // temporarily hides this same editor, so its lifetime must be
                // tied to FormClosed instead, on the worker's existing STA loop.
                var ended = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
                FormClosedEventHandler closed = (_,_)=>ended.TrySetResult();
                canvas.Window.FormClosed += closed;
                try {
                    canvas.Window.Show();
                    if (cancellation.IsCancellationRequested && !canvas.Window.IsDisposed) {
                        if (close is not null) close.PreserveAndClose(); else canvas.Window.Close();
                    }
                    await ended.Task;
                } finally { canvas.Window.FormClosed -= closed; }
            }
            returned = true;
        } finally {
            operations.ActivateNativeCapture = null;
            if (autosave is not null) {
                session.SnapshotReady -= Publish;
                if (close?.Accepted != true) {
                    var cleanup = await autosave.FinishAsync(!retakeViewFailed && returned && !cancellation.IsCancellationRequested);
                    foreach (var error in cleanup.Errors) Console.Error.WriteLine("Native draft cleanup: " + error);
                }
            }
        }
        cancellation.ThrowIfCancellationRequested();
        if ((quickSave && session.Confirmed || session.Saved) && saved is not null) return saved;
        if(session.ResultPresented)return new {mode,resultWindow=true,resultWindowId=session.ResultWindowId,copied=false};
        if(session.Copied&&!session.Confirmed)return new {mode,copied=true};
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
