using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// One session owns input, editing and output on the existing capture window.
// Observers are optional; production performs no diagnostic I/O or browser work.
internal sealed class NativeCaptureSession : IDisposable
{
    private readonly WinOneCaptureCanvas capture;
    private readonly NativeToolbarControl nativeToolbar;
    private readonly HintLabel selectionHint = new() { AutoSize = true, Visible = false, Padding = new Padding(8, 4, 8, 4), TextAlign = ContentAlignment.MiddleCenter };
    private readonly CaptureCompletion confirmation;
    private readonly CaptureCompletion? pinCompletion;
    private readonly Action<Bitmap> saveImage;
    private readonly Action<Bitmap>? saveAsImage;
    private readonly FirstPaintObserver paint;
    private bool ready, disposed, applying, textTool, closing;
    private readonly bool quickSave;
    private bool completionQueued;
    private bool snapshotQueued;
    private int textSequence;
    private NativeInlineText? nativeText;
    private TextDraft? textDraft;
    private (float Scale, PointF Origin) inputView;
    private bool updatingInputView;
    private Point? toolbarPosition;
    private readonly string savingLabel;
    private readonly string savedLabel;
    private string? savedStatus;
    private string? preferencesStatus;
    private string? captureNotice;
    private WinOneToolStyle textStyle = new(Color.FromArgb(0xe2, 0x3b, 0x30), 24);
    internal sealed record TextDraft(int Id, Rectangle Bounds, int MaxHeight, Guid? Target = null, string Value = "", float FontPixels = 24, int ColorArgb = unchecked((int)0xffe23b30));

    public event Action? Updated;
    internal event Action<NativeCaptureSnapshot>? SnapshotReady;
    public bool Ready => ready;
    public string? Error { get; private set; }
    public bool Confirmed => confirmation.CloseRequested && confirmation.Committed;
    public bool Pinned => pinCompletion?.CloseRequested == true;
    public bool Copied => confirmation.Committed;
    public bool CloseRequested => confirmation.CloseRequested || Pinned;
    private bool Finalized => confirmation.Committed || pinCompletion?.Committed == true;
    internal NativeToolbarControl Toolbar => nativeToolbar;
    internal NativeInlineText? TextInput => nativeText;
    internal TextDraft? EditingText => textDraft;
    internal bool TextTool => textTool;
    internal bool HintVisible => selectionHint.Visible;
    internal void SetCaptureNotice(string? notice, string? language)
    {
        var english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        captureNotice = notice switch {
            null => null,
            "scrollStopped" => english ? "Capture stopped. Review before saving." : "已停止，请检查截图后保存。",
            "scrollPartial" => english ? "Only part of the content was captured. Review before saving." : "只捕获了部分内容，请检查后保存。",
            "scrollTargetChanged" => english ? "The target window changed. Review the captured content before saving." : "目标窗口已变化，请检查已捕获的内容后保存。",
            _ => throw new InvalidDataException("截图状态提示无效")
        };
        Refresh();
    }

    internal NativeCaptureSnapshot? Snapshot()
    {
        if (disposed) throw new ObjectDisposedException(nameof(NativeCaptureSession));
        var document = capture.SnapshotDocument();
        if (document is null) return null;
        NativeTextInputState? text = null;
        if (nativeText is { } input && textDraft is { } draft) {
            var bounds = TextImageBounds(input, draft);
            text = new(draft.Target, input.Text, bounds.X, bounds.Y, bounds.Width, bounds.Height,
                draft.MaxHeight, draft.FontPixels, draft.ColorArgb, input.SelectionStart, input.SelectionLength);
            text.Validate(document);
        }
        return new(document, text);
    }

    public NativeCaptureSession(WinOneCaptureCanvas capture, Action<Bitmap> copy, Action<Bitmap> save,
        Action<Bitmap>? saveAs = null, Action<Bitmap>? pin = null, bool dark = false, bool quickSave = false, string? language = null, IReadOnlyList<string>? toolbarTools = null, Func<string[]?, Task<string[]?>>? saveToolbar = null)
    {
        this.capture = capture; saveImage = save; saveAsImage = saveAs;
        savingLabel = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase) ? "Saving toolbar…" : "正在保存工具栏…";
        savedLabel = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase) ? "Screenshot saved" : "截图已保存";
        this.quickSave = quickSave;
        nativeToolbar = new NativeToolbarControl(dark, language, toolbarTools, saveToolbar);
        nativeToolbar.PreferencesChanged += PreferencesChanged;
        nativeToolbar.PreferencesSaving += PreferencesSaving;
        nativeToolbar.PreferencesFailed += PreferencesFailed;
        nativeToolbar.SetSaveAsAvailable(saveAs is not null);
        nativeToolbar.SetPinAvailable(pin is not null);
        nativeToolbar.Command += NativeCommand;
        nativeToolbar.ToolStyleChanged += ChangeStyle;
        nativeToolbar.MoveRequested += MoveToolbar;
        selectionHint.BackColor = dark ? Color.FromArgb(23, 23, 23) : Color.White;
        selectionHint.ForeColor = dark ? Color.White : Color.FromArgb(23, 23, 23);
        capture.Window.Controls.Add(nativeToolbar);
        capture.Window.Controls.Add(selectionHint);
        capture.Window.KeyPreview = true;
        confirmation = new CaptureCompletion(capture.RenderSelection, copy, capture.Window.Close, () => capture.Window.IsDisposed);
        if (pin is not null) pinCompletion = new CaptureCompletion(capture.RenderSelection, pin, capture.Window.Close, () => capture.Window.IsDisposed);
        capture.Window.FormClosed += Closed;
        capture.Changed += OnChanged;
        capture.DocumentChanged += DocumentChanged;
        capture.RegionCommitted += QueueSnapshot;
        capture.RegionCommitted += CompleteQuickSelection;
        capture.KeyPressed += OnKeyPressed;
        capture.Window.MouseUp += OnMouseUp;
        paint = new FirstPaintObserver(capture.Window, () => { ready = true; Refresh(); CompleteQuickSelection(); }, FilterTextInput);
        Refresh();
    }
    private void CompleteQuickSelection()
    {
        if (!quickSave || !ready || disposed || completionQueued || Finalized || capture.Selection is null) return;
        completionQueued = true;
        // Finish the engine's mouse/shape dispatch before exporting or closing
        // its window. This posts work, without timers or an extra UI transition.
        capture.Window.BeginInvoke((Action)(() => {
            completionQueued = false;
            if (disposed || capture.Window.IsDisposed || Finalized || closing) return;
            try { Confirm(); Refresh(); } catch (Exception error) { Refresh(error.Message); }
        }));
    }
    private void MoveToolbar(Point screenPoint) { toolbarPosition = capture.Window.PointToClient(screenPoint); Refresh(); }
    private void PreferencesSaving() { preferencesStatus = savingLabel; Refresh(); }
    private void PreferencesChanged() { preferencesStatus = null; Refresh(); }
    private void PreferencesFailed(string error) { preferencesStatus = null; Refresh(error); }
    private void ChangeStyle(ShareX.ScreenCaptureLib.WinOneToolStyle style)
    {
        if (disposed || Finalized) return;
        if (textTool) {
            if (style.Value < 6 || style.Value > 240 || style.Color.A != 255) return;
            if (nativeText is { } input) {
                // Use the input's existing IME gate; changing a style must not
                // end a composition or replace the native text control.
                try { input.RequestCommit(() => {
                    if (disposed || nativeText != input || textDraft is null) return;
                    input.SetTextStyle(style.Value, style.Color);
                    ResizeText(input, textDraft);
                    textDraft = textDraft with { FontPixels = style.Value, ColorArgb = style.Color.ToArgb() };
                    textStyle = style; Refresh(); QueueSnapshot();
                }, error => Refresh(error.Message)); }
                catch (Exception error) { Refresh(error.Message); }
            } else { textStyle = style; Refresh(); }
            return;
        }
        if (textDraft is not null) return;
        try { capture.SetToolStyle(style); Refresh(); } catch (Exception error) { Refresh(error.Message); }
    }
    private void Save() { capture.FinishGesture(); using var image = capture.RenderSelection(); saveImage(image); savedStatus = savedLabel; }
    private void SaveAs()
    {
        if (saveAsImage is null) throw new InvalidOperationException("另存为尚未连接");
        capture.FinishGesture();
        using var image = capture.RenderSelection();
        try { saveAsImage(image); savedStatus = savedLabel; }
        catch (OperationCanceledException) { } // Closing the picker keeps the screenshot open.
    }
    private void Confirm()
    {
        if (pinCompletion?.Committed == true) { pinCompletion.Complete(); return; }
        if (textDraft is not null) { RequestTextCommit("confirm"); return; }
        capture.FinishGesture();
        try { confirmation.Complete(); }
        catch { if (confirmation.Committed) capture.SetTextEditing(true); throw; }
    }
    private void BeginTextAt(Point point)
    {
        point = capture.ViewToImage(point);
        var selection = capture.Selection ?? throw new InvalidOperationException("请先选择截图区域");
        if (!selection.Contains(point)) throw new InvalidOperationException("请在截图选区内点击");
        var existing = capture.TextAt(point);
        var bounds = existing?.Bounds ?? NativeTextPlacement.NewBounds(selection, point);
        if (!selection.Contains(bounds)) throw new InvalidOperationException("文字超出截图选区");
        textDraft = new TextDraft(++textSequence, bounds, selection.Bottom - bounds.Top, existing?.Id, existing?.Text ?? "", existing?.FontPixels ?? textStyle.Value, existing?.ColorArgb ?? textStyle.Color.ToArgb());
        OpenTextInput();
    }

    internal void RestoreText(NativeTextInputState state)
    {
        if (nativeText is not null || textDraft is not null) throw new InvalidOperationException("已有正在编辑的文字");
        state.Validate(capture.SnapshotDocument() ?? throw new InvalidOperationException("请先完成选区调整"));
        textTool = true; capture.SelectTool("select");
        textDraft = new TextDraft(++textSequence, state.Bounds, state.MaxHeight, state.Target, state.Value, state.FontPixels, state.ColorArgb);
        OpenTextInput();
        nativeText!.Select(state.SelectionStart, state.SelectionLength);
        Refresh();
    }

    private void OpenTextInput()
    {
        if (textDraft is null) throw new InvalidOperationException("没有文字草稿");
        if (textDraft.Target is { } target) capture.HideEditedText(target, true);
        capture.SetTextEditing(true);
        {
            using var background = TextBackdrop(textDraft);
            var viewBounds = capture.ImageToView(textDraft.Bounds);
            nativeText = new NativeInlineText(capture.Window, background, viewBounds, textDraft.Value, textDraft.FontPixels, Color.FromArgb(textDraft.ColorArgb), capture.ViewScale, viewBounds.Location);
            inputView = (capture.ViewScale, capture.ViewOrigin);
            ResizeText(nativeText, textDraft);
            nativeText.TextChanged += (_, _) => {
                if (nativeText is null || textDraft is null) return;
                ResizeText(nativeText, textDraft); Refresh(); QueueSnapshot();
            };
            nativeText.KeyDown += (_, key) => {
                if (nativeText?.IsComposing == true) return;
                if (key.KeyData == Keys.Escape) { key.SuppressKeyPress = true; CancelNativeText(); }
                else if (key.KeyData is (Keys.Control | Keys.Enter) or (Keys.Control | Keys.S) or (Keys.Control | Keys.Shift | Keys.S)) {
                    key.SuppressKeyPress = true;
                    try { RequestTextCommit(key.KeyData == (Keys.Control | Keys.S) ? "save" : key.KeyData == (Keys.Control | Keys.Shift | Keys.S) ? "saveAs" : null); }
                    catch (Exception error) { Refresh(error.Message); }
                }
            };
            nativeText.MouseUp += (_, mouse) => { if (mouse.Button == MouseButtons.Right) capture.Window.Close(); };
            nativeText.BringToFront(); nativeText.Focus(); nativeText.SelectAll();
        }
    }

    private void RequestTextCommit(string? after = null)
    {
        if (nativeText is not null) {
            var input = nativeText;
            input.RequestCommit(() => CommitNativeText(input, after), error => Refresh(error.Message)); return;
        }
        throw new InvalidOperationException("没有正在编辑的文字");
    }

    private void CommitNativeText(NativeInlineText input, string? after)
    {
        if (nativeText != input || textDraft is null) return;
        if (!input.Fits) throw new InvalidOperationException("文字超出截图范围，请缩短内容");
        var bounds = TextImageBounds(input, textDraft);
        var lines = input.GetVisualLines().Select(line => new WinOneTextLine(line.Text,
            (int)Math.Round(line.X / inputView.Scale), (int)Math.Round(line.Y / inputView.Scale))).ToArray();
        if (textDraft.Target is Guid target) capture.UpdateText(target, input.Text, bounds, lines, textDraft.FontPixels, input.ForeColor);
        else if (!string.IsNullOrWhiteSpace(input.Text)) capture.AddText(input.Text, bounds, textDraft.FontPixels, lines, input.ForeColor);
        nativeText = null; textDraft = null; input.Dispose(); capture.SetTextEditing(false);
        capture.Window.Focus();
        if (after == "save") Save();
        else if (after == "saveAs") SaveAs();
        else if (after == "pin") Pin();
        else if (after == "confirm") Confirm();
        Refresh(); QueueSnapshot();
    }

    private void CancelNativeText()
    {
        if (textDraft?.Target is Guid target) capture.HideEditedText(target, false);
        var input = nativeText; nativeText = null; textDraft = null; input?.Dispose();
        capture.SetTextEditing(false); capture.Window.Focus(); Refresh(); QueueSnapshot();
    }

    private void NativeCommand(string command)
    {
        savedStatus = null;
        if (command == "pin") {
            try { Pin(); Refresh(); } catch (Exception error) { Refresh(error.Message); }
            return;
        }
        switch (command) {
            case "confirm": OnKeyPressed(Keys.Enter); return;
            case "save": OnKeyPressed(Keys.Control | Keys.S); return;
            case "saveAs": OnKeyPressed(Keys.Control | Keys.Shift | Keys.S); return;
            case "undo": OnKeyPressed(Keys.Control | Keys.Z); return;
            case "redo": OnKeyPressed(Keys.Control | Keys.Shift | Keys.Z); return;
            case "text": OnKeyPressed(Keys.T); return;
            case "cancel": capture.Window.Close(); return;
        }
        if (textDraft is not null || Finalized) return;
        textTool = false; capture.SelectTool(command); capture.Window.Focus(); Refresh(); QueueSnapshot();
    }
    private void Pin()
    {
        if (pinCompletion is null) throw new InvalidOperationException("截图置顶尚未连接");
        if (textDraft is not null) { RequestTextCommit("pin"); return; }
        capture.FinishGesture(); pinCompletion.Complete();
    }

    private void OnKeyPressed(Keys key)
    {
        if (!ready || disposed) return;
        savedStatus = null;
        try {
            applying = true;
            if (key == Keys.Escape) {
                if (textDraft is not null) {
                    CancelNativeText(); return;
                } else capture.Window.Close();
            } else if (key is Keys.Enter or (Keys.Control | Keys.C)) Confirm();
            else if (!Finalized) {
                if (key is (Keys.Control | Keys.S) or (Keys.Control | Keys.Shift | Keys.S)) {
                    var saveAs = key == (Keys.Control | Keys.Shift | Keys.S);
                    if (textDraft is not null) RequestTextCommit(saveAs ? "saveAs" : "save");
                    else if (saveAs) SaveAs(); else Save();
                } else if (textDraft is null) {
                    if (key == (Keys.Control | Keys.Z)) capture.Undo();
                    else if (key is (Keys.Control | Keys.Y) or (Keys.Control | Keys.Shift | Keys.Z)) capture.Redo();
                    else if (key == Keys.T) { textTool = true; capture.SelectTool("select"); }
                }
            }
            applying = false; Refresh();
        } catch (Exception error) { applying = false; Refresh(error.Message); }
        finally { applying = false; QueueSnapshot(); }
    }
    private bool FilterTextInput(Message message)
    {
        if (closing && message.Msg is >= 0x0100 and <= 0x020e) return true;
        if (message.Msg == 0x0205) { capture.Window.Close(); return true; }
        if (message.Msg == 0x0203) {
            try { Confirm(); Refresh(); } catch (Exception error) { Refresh(error.Message); }
            return true;
        }
        if (Finalized && message.Msg == 0x0100) {
            OnKeyPressed((Keys)message.WParam.ToInt32() | Control.ModifierKeys); return true;
        }
        if (Finalized && message.Msg is >= 0x0100 and <= 0x020e) return true;
        if (textTool && textDraft is null && message.Msg == 0x0201) {
            try { BeginTextAt(NativeTextPlacement.PointFromMessage(message.LParam)); Refresh(); }
            catch (Exception error) { Refresh(error.Message); }
            return true;
        }
        if (textDraft is null) {
            if (!textTool && message.Msg == 0x0201) capture.BeginCover(capture.ViewToImage(NativeTextPlacement.PointFromMessage(message.LParam)));
            return false;
        }
        if (message.Msg == 0x0201) {
            try { if (nativeText is not null) RequestTextCommit(); }
            catch (Exception error) { Refresh(error.Message); }
            return true;
        }
        return message.Msg is 0x0202 or 0x0203;
    }
    private void OnChanged() { if (!applying) Refresh(); }
    private void DocumentChanged()
    {
        // A success message describes the saved revision, not later edits.
        savedStatus = null;
        if (!applying) Refresh();
        QueueSnapshot();
    }
    private void Closed(object? sender, FormClosedEventArgs args) { confirmation.NotifyClosed(); pinCompletion?.NotifyClosed(); }
    internal void SetClosing(bool value)
    {
        closing = value;
        nativeToolbar.Enabled = !value;
        if (nativeText is not null) nativeText.Enabled = !value;
        capture.SetTextEditing(value || Finalized || nativeText is not null);
    }
    internal void ReportPersistenceError(string message) => Refresh(message);
    private void OnMouseUp(object? sender, MouseEventArgs e) { Refresh(); QueueSnapshot(); }
    private void QueueSnapshot()
    {
        if (disposed || snapshotQueued || SnapshotReady is null || capture.Window.IsDisposed) return;
        snapshotQueued = true;
        // Dispatch once after the engine has finished its current gesture or
        // input event; never end a drag or perform persistence from painting.
        capture.Window.BeginInvoke((Action)(() => {
            snapshotQueued = false;
            if (disposed || capture.Window.IsDisposed || Finalized || closing) return;
            try { if (Snapshot() is { } snapshot) SnapshotReady?.Invoke(snapshot); }
            catch (Exception error) { Refresh(error.Message); }
        }));
    }
    private void Refresh(string? error = null)
    {
        if (disposed) return;
        Error = error;
        try { UpdateInputView(); } catch (Exception failure) { error ??= failure.Message; Error = error; }
        var selection = capture.ViewSelection;
        nativeToolbar.Visible = !quickSave && selection is not null;
        // Errors use the bounded native hint, never compete with action buttons
        // for toolbar width. The hint does not intercept selection input.
        var message = error ?? preferencesStatus ?? savedStatus ?? captureNotice;
        selectionHint.Visible = message is not null;
        if (selectionHint.Visible) {
            var screen = Screen.FromPoint(Cursor.Position).WorkingArea;
            var area = new Rectangle(capture.Window.PointToClient(screen.Location), screen.Size);
            selectionHint.MaximumSize = new Size(Math.Max(1, area.Width - 32), 0);
            selectionHint.Text = message;
            selectionHint.Location = new Point(area.Left + (area.Width - selectionHint.Width) / 2, area.Bottom - selectionHint.Height - 24);
            selectionHint.BringToFront();
        }
        if (!quickSave && selection is { } selected) {
            var style = textTool ? textDraft is { } draft ? new WinOneToolStyle(Color.FromArgb(draft.ColorArgb), (int)draft.FontPixels) : textStyle : capture.GetToolStyle();
            nativeToolbar.SetToolStyle(textTool ? "text" : capture.SelectedTool, style, Finalized || (!textTool && textDraft is not null));
            nativeToolbar.SetState(capture.CanUndo, capture.CanRedo, textTool ? "text" : capture.SelectedTool, Finalized, textDraft is not null);
            var center = toolbarPosition ?? new Point(selected.Left + selected.Width / 2, selected.Top + selected.Height / 2);
            var screen = Screen.FromPoint(capture.Window.PointToScreen(center)).WorkingArea;
            var viewport = new Rectangle(capture.Window.PointToClient(screen.Location), screen.Size);
            nativeToolbar.Bounds = ToolbarPlacement.Place(selected, viewport, nativeToolbar.PreferredToolbarSize, toolbarPosition);
            nativeToolbar.BringToFront();
            nativeText?.BringToFront();
        }
        Updated?.Invoke();
    }

    private Rectangle TextImageBounds(NativeInlineText input, TextDraft draft) => new(draft.Bounds.X, draft.Bounds.Y, draft.Bounds.Width,
        Math.Min(draft.MaxHeight, Math.Max(1, (int)Math.Ceiling(input.Height / inputView.Scale))));
    private Bitmap TextBackdrop(TextDraft draft) => capture.RenderTextBackdrop(new Rectangle(draft.Bounds.X, draft.Bounds.Y, draft.Bounds.Width, draft.MaxHeight));
    private void ResizeText(NativeInlineText input, TextDraft draft) => input.ResizeToContent(Math.Max(1, (int)Math.Floor(draft.MaxHeight * inputView.Scale)));
    private void UpdateInputView()
    {
        if (updatingInputView || nativeText is not { } input || textDraft is null || inputView == (capture.ViewScale, capture.ViewOrigin)) return;
        input.RequestCommit(() => {
            if (disposed || nativeText != input || textDraft is null) return;
            updatingInputView = true;
            try {
                var bounds = TextImageBounds(input, textDraft);
                if (inputView.Scale == capture.ViewScale) {
                    // Panning moves the same text backdrop; no full-image render
                    // or brush allocation belongs in pointer movement.
                    input.MoveView(capture.ImageToView(bounds));
                } else {
                    using var background = TextBackdrop(textDraft);
                    input.SetView(background, capture.ImageToView(bounds), capture.ViewScale, textDraft.FontPixels, Color.FromArgb(textDraft.ColorArgb));
                }
                inputView = (capture.ViewScale, capture.ViewOrigin);
                ResizeText(input, textDraft);
            } finally { updatingInputView = false; }
        }, error => { updatingInputView = true; try { Refresh(error.Message); } finally { updatingInputView = false; } });
    }

    public void Dispose()
    {
        if (disposed) return;
        disposed = true;
        capture.Changed -= OnChanged; capture.KeyPressed -= OnKeyPressed;
        capture.Window.FormClosed -= Closed;
        capture.DocumentChanged -= DocumentChanged; capture.RegionCommitted -= QueueSnapshot;
        capture.RegionCommitted -= CompleteQuickSelection;
        capture.Window.MouseUp -= OnMouseUp;
        nativeToolbar.Command -= NativeCommand; nativeToolbar.MoveRequested -= MoveToolbar;
        nativeToolbar.ToolStyleChanged -= ChangeStyle;
        nativeToolbar.PreferencesChanged -= PreferencesChanged; nativeToolbar.PreferencesFailed -= PreferencesFailed;
        nativeToolbar.PreferencesSaving -= PreferencesSaving;
        nativeText?.Dispose(); nativeToolbar.Dispose(); selectionHint.Dispose();
        paint.Dispose();
    }
    private sealed class HintLabel : Label
    {
        // The hint is informative; pointer input must still reach the canvas.
        protected override void WndProc(ref Message message)
        {
            if (message.Msg == 0x0084) { message.Result = new IntPtr(-1); return; }
            base.WndProc(ref message);
        }
    }

    private sealed class FirstPaintObserver : NativeWindow, IDisposable
    {
        private readonly Form window;
        private readonly Action callback;
        private readonly Func<Message, bool> input;
        private bool fired;
        public FirstPaintObserver(Form window, Action callback, Func<Message, bool> input)
        {
            this.window = window; this.callback = callback; this.input = input;
            AssignHandle(window.Handle);
            // WinForms can recreate the HWND when changing window styles.
            // Keep input routing and first-paint readiness on the live handle.
            window.HandleCreated += HandleCreated; window.HandleDestroyed += HandleDestroyed;
        }
        private void HandleCreated(object? sender, EventArgs args) { fired = false; AssignHandle(window.Handle); }
        private void HandleDestroyed(object? sender, EventArgs args) => ReleaseHandle();
        public void Dispose() { window.HandleCreated -= HandleCreated; window.HandleDestroyed -= HandleDestroyed; ReleaseHandle(); }
        protected override void WndProc(ref Message message)
        {
            var painting = message.Msg == 0x000f;
            if (input(message)) return;
            base.WndProc(ref message);
            if (painting && !fired) { fired = true; callback(); }
        }
    }
}
