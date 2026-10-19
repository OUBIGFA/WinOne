using System.Drawing.Imaging;
using System.Text.Json;
using ShareX;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture;

internal record CaptureOutput(string Path, int Width, int Height, bool Copied, bool Pinned, string Id = "", bool PreviewReady = false, string? PreviewWarning = null);
internal record CaptureEditorRequest(string DraftId);

internal sealed partial class CaptureOperations
{
    private readonly SettingsStore store;
    private readonly string lastPath;
    private readonly CapturePreviews previews;
    private readonly CaptureDraftStore drafts;
    private readonly Action<Bitmap> copyImage;
    private readonly Func<CaptureSettings, string?, string?> chooseSavePath;
    private volatile CaptureOutput? last;
    public CaptureOutput? Last => last;
    public CaptureOperations(SettingsStore store, string directory, Action<Bitmap>? copyImage = null, Func<CaptureSettings, string?, string?>? chooseSavePath = null)
    {
        // Tests can verify exact exported pixels without changing the user's clipboard.
        this.copyImage = copyImage ?? CopyToClipboard;
        this.chooseSavePath = chooseSavePath ?? SelectPath;
        this.store = store; lastPath = Path.Combine(directory, "last-capture.json"); previews = new CapturePreviews(directory); drafts = new CaptureDraftStore(directory);
        if (File.Exists(lastPath))
        {
            try {
                last = JsonSerializer.Deserialize<CaptureOutput>(File.ReadAllBytes(lastPath), SettingsStore.Json);
                if (last is not null && string.IsNullOrEmpty(last.Id)) { last = last with { Id = Guid.NewGuid().ToString("N") }; PersistLast(); }
            }
            catch (JsonException error) { Console.Error.WriteLine("Last capture metadata: " + error.Message); }
        }
    }
    public object Capture(string mode, CancellationToken cancellation, bool unifiedEditor = false, string? language = null)
    {
        if (drafts.Read() is not null) throw new InvalidOperationException("请先完成或放弃当前截图草稿");
        if (unifiedEditor && (mode is "window" or "region" or "region-native" or "fullscreen") && (mode != "fullscreen" || store.Snapshot().OpenEditor))
            return BeginUnifiedSelection(mode == "region-native" ? "region" : mode);
        var settings = store.Snapshot(); ApplyTheme(settings);
        var quickSave = unifiedEditor && !settings.OpenEditor;
        if (quickSave) settings = settings with { AskWhereToSave = false };
        if (unifiedEditor && mode == "region") mode = "region-native";
        if (unifiedEditor && mode == "window") mode = "window-native";
        Bitmap? image = null;
        CapturePlacement? placement = null;
        Bitmap? desktopPreview = null;
        try
        {
        var editRequested = false;
        switch (mode)
        {
            // Selection and direct actions share one immutable desktop frame.
            case "region-native":
            case "window-native":
                var targets = mode == "window-native" ? WindowCaptureTargets.Snapshot() : null;
                var regionGroups = targets?.ToDictionary(target => target.Handle, target => (IReadOnlyList<Rectangle>)new[] { target.Bounds });
                var regions = targets?.Select(target => target.Bounds).ToArray();
                using (var frame = CaptureDesktopFrame())
                {
                    if (targets is not null && !targets.SequenceEqual(WindowCaptureTargets.Snapshot())) throw new InvalidOperationException("窗口布局已变化，请重新截图");
                    object? directResult = null;
                    bool ExecuteSelection(NativeSelectionAction action, Rectangle region)
                    {
                        cancellation.ThrowIfCancellationRequested();
                        using var selected = frame.Crop(region);
                        switch (action)
                        {
                            case NativeSelectionAction.Copy:
                                if (!ClipboardHelpers.CopyImage(selected)) throw new InvalidOperationException((language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase) ? "Clipboard unavailable. Try again." : "剪贴板暂不可用，请重试。");
                                directResult = new { copied = true }; break;
                            case NativeSelectionAction.Save:
                                if (quickSave) { directResult = Export(selected, settings, cancellation, language: language); break; }
                                var path = SelectPath(settings, language);
                                if (path is null) return false;
                                directResult = SaveLocalImage(selected, path); break;
                            case NativeSelectionAction.SaveAndCopy:
                                directResult = Export(selected, settings with { AskWhereToSave = false, CopyToClipboard = true, PinAfterCapture = false }, cancellation, language: language); break;
                            case NativeSelectionAction.Pin:
                                PinToScreenForm.OpenManaged(new Bitmap(selected), PinOptions(), language, settings.FileNameTemplate);
                                directResult = new { pinned = true }; break;
                        }
                        return true;
                    }
                    var directEdit = unifiedEditor && settings.OpenEditor;
                    using var selector = new NativeSelectionWindow(frame, language ?? System.Globalization.CultureInfo.CurrentUICulture.Name, ShareXResources.Theme.IsDarkTheme, unifiedEditor, regions, unifiedEditor ? ExecuteSelection : null, quickSave, directEdit);
                    using var refinement = targets is null ? null : new WindowRegionRefinement(targets, regionGroups!, selector.UpdateWindowRegions,
                        readNative: target => {
                            if (CaptureHelpers.GetWindowRectangle(target.Handle) != target.Bounds) throw new InvalidOperationException("窗口布局已变化，请重新截图");
                            return WindowCaptureTargets.Regions([target]);
                        });
                    if (refinement is not null) selector.WindowHovered += refinement.Hover;
                    if (directEdit) selector.BeforeEditorHandoff = selection => HoldCapture(frame, selection);
                    var selectionResult = selector.ShowDialog();
                    if (directResult is not null) return directResult;
                    if (directEdit && selectionResult == DialogResult.OK)
                        return new CaptureEditorRequest(PrepareHeldCapture(frame, selector.Selection, settings).Id);
                    image = selectionResult == DialogResult.OK ? frame.Crop(selector.Selection) : null;
                    if (image is not null) placement = CapturePlacement.From(selector.Selection, frame.Bounds);
                    editRequested = selector.EditRequested;
                    if (selector.CancellationReason is "display_changed" or "dpi_changed" or "bounds_changed")
                        throw new InvalidOperationException("显示设置已变化，请重新截图");
                    if (image is not null && unifiedEditor && (settings.OpenEditor || editRequested)) desktopPreview = frame.CreatePreview();
                }
                break;
            case "region":
            case "window":
                using (var selector = new RegionCaptureForm(mode == "window" ? RegionCaptureMode.OneClick : RegionCaptureMode.Annotation, Options()))
                { selector.ShowDialog(); image = selector.GetResultImage(); }
                break;
            case "fullscreen":
                if (unifiedEditor && settings.OpenEditor)
                {
                    using var frame = CaptureDesktopFrame();
                    return new CaptureEditorRequest(PrepareHeldCapture(frame, frame.Bounds, settings, fullscreen: true).Id);
                }
                using (var frame = CaptureDesktopFrame()) { placement = CapturePlacement.From(frame.Bounds, frame.Bounds); if (unifiedEditor && settings.OpenEditor) desktopPreview = frame.CreatePreview(); image = frame.Detach(); }
                break;
            case "scroll":
                using (var scrolling = new ScrollingWindow(settings, language, manager => SelectScrollingTarget(manager, language), unifiedEditor))
                {
                    image = scrolling.ShowDialog() == DialogResult.OK ? scrolling.TakeResult() : null;
                    editRequested = scrolling.EditRequested;
                }
                break;
            default: throw new InvalidOperationException("不支持的截图方式");
        }
        if (image is null) throw new OperationCanceledException("截图已取消");
        // Both allocations belong to this operation, including failure paths
        // before a complete draft has been committed.
        {
            cancellation.ThrowIfCancellationRequested();
            if (settings.OpenEditor || editRequested) return unifiedEditor ? new CaptureEditorRequest(PrepareEditor(image, settings, placement, desktopPreview).Id) : Edit(image, settings, cancellation);
            return Export(image, settings, cancellation, language: language);
        }
        }
        finally { desktopPreview?.Dispose(); image?.Dispose(); }
    }
    private bool SelectScrollingTarget(ScrollingCaptureManager manager, string? language, bool directConfirm = false)
    {
        var targets = WindowCaptureTargets.Snapshot();
        using var frame = CaptureDesktopFrame();
        using var selector = new NativeSelectionWindow(frame, language ?? System.Globalization.CultureInfo.CurrentUICulture.Name, ShareXResources.Theme.IsDarkTheme, allowEdit: false, directConfirm: directConfirm);
        if (selector.ShowDialog() != DialogResult.OK) return false;
        return SelectScrollingTarget(manager, selector.Selection, targets, language);
    }
    // Both the explicit retake selector and the screenshot toolbar use this
    // same validation. A committed physical selection needs no second picker.
    internal static bool SelectScrollingTarget(ScrollingCaptureManager manager, Rectangle region,
        IReadOnlyList<WindowCaptureTarget> targets, string? language)
    {
        var english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        var target = WindowCaptureTargets.ForRegion(targets, region);
        var current = WindowCaptureTargets.ForRegion(WindowCaptureTargets.Snapshot(), region);
        if (target is null || current != target)
            throw new InvalidOperationException(english ? "Select an unobstructed area inside one window." : "请在同一窗口内选择未被遮挡的区域。");
        manager.SelectTarget(new WindowInfo(target.Handle), region);
        // Wheel scrolling follows the pointer. Toolbar confirmation may leave
        // it outside the selected content, so move it back before capturing.
        Cursor.Position = new Point(region.Left + region.Width / 2, region.Top + region.Height / 2);
        return true;
    }
    internal CaptureDraft PrepareEditor(Bitmap image, CaptureSettings settings, CapturePlacement? placement = null, Bitmap? desktopPreview = null) => drafts.Begin(image, settings, placement, desktopPreview);
    internal CaptureDraft PrepareSelectionEditor(CapturedFrame frame, Rectangle selected, CaptureSettings settings)
    {
        // Keep one immutable desktop frame so resizing can reveal adjacent pixels.
        // Every output remains cropped; a missing selection is rejected by the store.
        if (selected.Width < 1 || selected.Height < 1 || !frame.Bounds.Contains(selected)) throw new InvalidDataException("截图选区无效");
        var crop = new AnnotationCrop(selected.X - frame.Bounds.X, selected.Y - frame.Bounds.Y, selected.Width, selected.Height);
        using var background = frame.CreatePreview();
        return frame.WithImage(image => drafts.Begin(image, settings, CapturePlacement.From(frame.Bounds, frame.Bounds), background, crop));
    }
    public CaptureEditorRequest PrepareLastDraft(string expectedId)
    {
        using var image = LoadLast(expectedId);
        // Editing a saved image is explicit Save-as work, not a new capture's
        // automatic clipboard/pinning task list.
        var settings = store.Snapshot() with { AskWhereToSave = true, CopyToClipboard = false, PinAfterCapture = false };
        return new CaptureEditorRequest(PrepareEditor(image, settings).Id);
    }
    public CaptureDraft? GetDraft(string? expectedId = null)
    {
        var draft = drafts.Read();
        if (expectedId is not null && draft?.Id != expectedId) throw new InvalidOperationException("截图草稿已变化，请重新加载");
        return draft;
    }
    public CaptureDraft UpdateDraft(string id, long revision, AnnotationDocument document) => drafts.Update(id, revision, document);
    public object CancelDraft(string id, long revision) { _ = RequireDraft(id, revision); try { drafts.Cancel(id, revision); return new { cancelled = true }; } finally { if (selectionCandidates?.Id == id) { selectionCandidates.Dispose(); selectionCandidates = null; } ReleaseCaptureHold(id); } }
    public CaptureOutput CompleteDraft(string id, long revision, string? path, CancellationToken cancellation, string? language = null, bool saveCopy = false)
    {
        var draft = RequireDraft(id, revision);
        var settings = saveCopy ? draft.Settings with { AskWhereToSave = false, CopyToClipboard = true, PinAfterCapture = false } : draft.Settings;
        if (saveCopy) path = null;
        if (string.IsNullOrEmpty(path) && settings.AskWhereToSave) throw new InvalidDataException("请先选择保存位置");
        using var rendered = RenderDraft(draft);
        var output = Export(rendered, settings, cancellation, path, language);
        drafts.Complete(id, revision);
        return output;
    }
    private CaptureDraft RequireDraft(string id, long revision)
    {
        var draft = GetDraft(id)!;
        if (draft.Revision != revision) throw new InvalidOperationException("截图草稿已变化，请重新加载");
        return draft;
    }
    internal Bitmap RenderDraft(CaptureDraft draft)
    {
        if (draft.SelectionPending) throw new InvalidOperationException("请先选择截图区域");
        using var original = drafts.Load(draft.Id);
        return AnnotationRenderer.Render(original, draft.Document);
    }
    public object CopyDraft(string id, long revision)
    {
        using var rendered = RenderDraft(RequireDraft(id, revision));
        Copy(rendered);
        // Explicit image actions keep the draft available for further editing.
        return new { draftId = id, revision, copied = true };
    }
    public object ConfirmDraft(string id, long revision)
    {
        // Confirm means copy and finish. Only an explicit Save writes a file;
        // a failed clipboard write leaves the full editing session recoverable.
        using var rendered = RenderDraft(RequireDraft(id, revision));
        Copy(rendered);
        drafts.Complete(id, revision);
        return new { draftId = id, revision, copied = true };
    }
    public object PinDraft(string id, long revision, string? language = null)
    {
        var draft = RequireDraft(id, revision);
        using var rendered = RenderDraft(draft);
        ApplyTheme(store.Snapshot());
        PinToScreenForm.OpenManaged(new Bitmap(rendered), PinOptions(), language, draft.Settings.FileNameTemplate);
        return new { draftId = id, revision, pinned = true, pins = PinToScreenForm.ActiveCount };
    }
    // Reuses ShareX's desktop acquisition and bounds calculation. Selection UI
    // can consume this frame without constructing an upstream editor window.
    internal CapturedFrame CaptureDesktopFrame()
    {
        var bounds = CaptureHelpers.GetScreenBounds();
        var image = new Screenshot().CaptureRectangle(bounds) ?? throw new InvalidOperationException("无法读取桌面图像");
        try { return CapturedFrame.Own(image, bounds); }
        catch { image.Dispose(); throw; }
    }
    public object EditLast(CancellationToken cancellation, string? expectedId = null)
    {
        var settings = store.Snapshot(); ApplyTheme(settings);
        using var image = LoadLast(expectedId); return Edit(image, settings, cancellation);
    }
    private object Edit(Bitmap image, CaptureSettings settings, CancellationToken cancellation)
    {
        CaptureOutput? savedDuringEdit = null;
        using var editor = CreateEditor(image, saved => savedDuringEdit = saved, settings);
        editor.ShowDialog();
        if (editor.Result is not (RegionResult.AnnotateContinueTask or RegionResult.AnnotateRunAfterCaptureTasks))
        {
            if (savedDuringEdit is not null) return savedDuringEdit;
            throw new OperationCanceledException("编辑已取消");
        }
        using var result = editor.Result == RegionResult.AnnotateContinueTask ? new Bitmap(editor.Canvas) : editor.GetResultImage();
        if (result is null) throw new InvalidOperationException("未生成编辑结果");
        return Export(result, settings, cancellation, editor.ImageFilePath);
    }
    // Direct editor saves are committed outputs, even if the user later closes the editor.
    internal RegionCaptureForm CreateEditor(Bitmap image, Action<CaptureOutput> onSaved, CaptureSettings? sessionSettings = null)
    {
        var settings = sessionSettings ?? store.Snapshot();
        var editor = new RegionCaptureForm(RegionCaptureMode.TaskEditor, Options(), new Bitmap(image));
        string Save(Bitmap output, string? path, bool ask)
        {
            using (output)
            {
                var saved = SaveEditorOutput(output, path, ask, settings);
                if (saved is null) return "";
                onSaved(saved); return saved.Path;
            }
        }
        editor.SaveImageRequested += (output, path) => Save(output, path, false);
        editor.SaveImageAsRequested += (output, path) => Save(output, path, true);
        editor.TryCopyImageRequested += output =>
        {
            try { Copy(output); return true; }
            catch (Exception error)
            {
                MessageBox.Show(editor, error.Message, "复制失败", MessageBoxButtons.OK, MessageBoxIcon.Error);
                return false;
            }
        };
        return editor;
    }
    private CaptureOutput? SaveEditorOutput(Bitmap image, string? path, bool ask, CaptureSettings settings)
    {
        try
        {
            if (ask || string.IsNullOrEmpty(path)) path = SelectPath(settings);
            if (string.IsNullOrEmpty(path)) return null;
            return SaveLocalImage(image, path);
        }
        catch (Exception error) { MessageBox.Show(error.Message, "保存失败", MessageBoxButtons.OK, MessageBoxIcon.Error); return null; }
    }
    internal CaptureOutput Export(Bitmap image, CaptureSettings settings, CancellationToken cancellation, string? path = null, string? language = null)
    {
        cancellation.ThrowIfCancellationRequested();
        var automatic = string.IsNullOrEmpty(path) && !settings.AskWhereToSave;
        if (string.IsNullOrEmpty(path)) path = automatic ? settings.OutputDirectory : chooseSavePath(settings, language);
        if (string.IsNullOrEmpty(path)) throw new OperationCanceledException("保存已取消");
        var output = SaveLocalImage(image, path, automatic, settings.FileNameTemplate);
        path = output.Path;
        if (settings.CopyToClipboard)
        {
            try { Copy(image); output = output with { Copied = true }; last = output; }
            catch (Exception error) { throw new InvalidOperationException($"截图已保存到 {path}，复制失败：{error.Message}", error); }
        }
        if (settings.PinAfterCapture) { PinToScreenForm.OpenManaged(new Bitmap(image), PinOptions(), language, settings.FileNameTemplate); output = output with { Pinned = true }; last = output; }
        PersistLast(); return output;
    }
    private CaptureOutput SaveLocalImage(Bitmap image, string path, bool automatic = false, string? template = null)
    {
        var previous = last;
        if (automatic) path = PngFiles.WriteUnique(image, path, DateTime.Now, template);
        else WriteImage(image, path);
        last = new(path, image.Width, image.Height, false, false, Guid.NewGuid().ToString("N"));
        try { previews.Write(image, last.Id); last = last with { PreviewReady = true }; }
        catch (Exception error) { last = last with { PreviewWarning = "截图已保存，预览生成失败：" + error.Message }; }
        PersistLast();
        if (previous is not null)
        {
            try { previews.Remove(previous.Id); }
            catch (Exception error) { last = last with { PreviewWarning = (last.PreviewWarning is null ? "" : last.PreviewWarning + "\n") + "旧预览清理失败：" + error.Message }; PersistLast(); }
        }
        return last;
    }
    public object CopyLast(string? expectedId = null) { using var image = LoadLast(expectedId); Copy(image); last = last! with { Copied = true }; PersistLast(); return last; }
    // Save-as is an explicit toolbar action. It does not repeat clipboard or
    // pin-after-capture preferences and leaves the original file intact.
    public CaptureOutput SaveLastAs(string path, string? expectedId = null)
    {
        using var image = LoadLast(expectedId);
        return SaveLocalImage(image, path);
    }
    public CaptureOutput ApplyAnnotations(string path, string expectedId, AnnotationDocument document)
    {
        if (string.IsNullOrEmpty(expectedId)) throw new InvalidDataException("缺少待编辑图像标识");
        using var original = LoadLast(expectedId);
        using var rendered = AnnotationRenderer.Render(original, document);
        return SaveLocalImage(rendered, path);
    }
    public CaptureEditPreview PreviewAnnotations(string expectedId, AnnotationDocument document)
    {
        using var original = last?.Id == expectedId ? LoadLast(expectedId) : drafts.Read()?.Id == expectedId ? drafts.Load(expectedId) : LoadLast(expectedId);
        using var rendered = AnnotationRenderer.Render(original, document);
        // Preview never changes last-capture metadata, clipboard, or output.
        return previews.WriteEdit(rendered, expectedId);
    }
    public object PinLast(string? expectedId = null, string? language = null) { var settings = store.Snapshot(); ApplyTheme(settings); using var image = LoadLast(expectedId); PinToScreenForm.OpenManaged(new Bitmap(image), PinOptions(), language, settings.FileNameTemplate); return new { pins = PinToScreenForm.ActiveCount }; }
    private Bitmap LoadLast(string? expectedId = null)
    {
        var output = last ?? throw new InvalidOperationException("还没有截图");
        if (expectedId is not null && output.Id != expectedId) throw new InvalidOperationException("截图已变化，请刷新后重试");
        using var source = new Bitmap(output.Path); return new Bitmap(source);
    }
    private void PersistLast() => SettingsStore.AtomicWrite(lastPath, JsonSerializer.SerializeToUtf8Bytes(last, SettingsStore.Json));
    private static RegionCaptureOptions Options() => new() { EnableSharing = false, ShowEditorPanTip = false, EditorAutoCopyImage = false };
    private static PinToScreenOptions PinOptions() => new() { Shadow = false, BorderColor = Color.FromArgb(100, 100, 100), BorderSize = 1 };
    private void Copy(Bitmap image) => copyImage(image);
    private static void CopyToClipboard(Bitmap image)
    {
        if (!ClipboardHelpers.CopyImage(image)) throw new InvalidOperationException("剪贴板暂不可用，请重试复制");
    }
    private static void WriteImage(Bitmap image, string path)
    {
        PngFiles.Write(image, path);
    }
    private static string? SelectPath(CaptureSettings settings, string? language = null)
    {
        var english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        using var dialog = new SaveFileDialog { Title = english ? "Save capture" : "保存截图", Filter = english ? "PNG image|*.png" : "PNG 图像|*.png", DefaultExt = "png", InitialDirectory = settings.OutputDirectory, FileName = PngFiles.SuggestedName(DateTime.Now, template: settings.FileNameTemplate), OverwritePrompt = true };
        return dialog.ShowDialog() == DialogResult.OK ? dialog.FileName : null;
    }
    internal static void ApplyTheme(CaptureSettings settings)
    {
        var dark = settings.Theme == "dark";
        if (settings.Theme == "system")
        {
            using var key = Microsoft.Win32.Registry.CurrentUser.OpenSubKey(@"Software\Microsoft\Windows\CurrentVersion\Themes\Personalize");
            dark = key?.GetValue("AppsUseLightTheme") is int value && value == 0;
        }
        ShareXResources.Theme = dark ? ShareXTheme.DarkTheme : ShareXTheme.LightTheme;
    }
}
