using WinOne.Capture.NativeUI;

namespace ShareX;

// WinOne owns the switch and user gestures. The unchanged source image also remains
// the source for Save original, even while the translated preview is visible.
public partial class PinToScreenForm
{
    private Func<bool>? pinEnabled;
    private NativeHostConfiguration? pinHost;
    private ToolStripButton? translateButton;
    private ToolStripMenuItem? translateAction, retranslateAction, toggleAction, copyTextAction, saveTranslated;
    private Button? comparison;
    private ToolTip? pinTip;
    private CancellationTokenSource? pinRequest;
    private PinTranslationResult? translated;
    private PinOcrLayout? pinLayout;
    private bool showTranslation;
    private Rectangle? translationArea;
    private Point? areaStart;
    private Image DisplayedPin => showTranslation && translated is not null ? translated.Picture : Image;

    private void SetUpTranslation(Func<bool> enabled, NativeHostConfiguration? host)
    {
        pinEnabled = enabled;
        pinHost = host;
        translateButton = new ToolStripButton(PinText("翻译", "Translate")) { DisplayStyle = ToolStripItemDisplayStyle.Text };
        translateButton.AccessibleName = translateButton.Text;
        translateButton.ToolTipText = PinText("翻译贴图。按住 Shift 拖动可选取局部。", "Translate the image. Shift-drag to select part of it.");
        translateButton.Click += (_, _) => TranslatePin(false);
        tsMain.Items.Insert(tsMain.Items.IndexOf(tsbOptions), translateButton);
        var menu = ContextMenuStrip!;
        ToolStripMenuItem Add(string zh, string en, Action click)
        {
            var item = new ToolStripMenuItem(PinText(zh, en));
            item.Click += (_, _) => click();
            menu.Items.Insert(menu.Items.Count - 2, item);
            return item;
        }
        translateAction = Add("翻译贴图", "Translate image", () => TranslatePin(false));
        retranslateAction = Add("重新翻译整张贴图", "Retranslate entire image", () => { translationArea = null; TranslatePin(true); });
        toggleAction = Add("切换原图与译图", "Switch original / translated", TogglePin);
        toggleAction.ShortcutKeyDisplayString = "Tab";
        copyTextAction = Add("复制全部译文", "Copy translated text", () => {
            if (translated is null) return;
            try { Clipboard.SetText(translated.Text); }
            catch (System.Runtime.InteropServices.ExternalException) { Status(PinText("剪贴板暂不可用", "Clipboard unavailable")); }
        });
        saveTranslated = Add("另存译图", "Save translated image as", () => {
            if (translated is not null) { using var snapshot = new Bitmap(translated.Picture); SavePinPicture(snapshot); }
        });
        comparison = new Button { AutoSize = true, TabStop = false, FlatStyle = FlatStyle.Popup, Visible = false };
        comparison.Click += (_, _) => TogglePin();
        Controls.Add(comparison);
        Resize += (_, _) => PositionComparison();
        pinTip = new ToolTip(components!);
        Disposed += (_, _) => { CancelPin(); translated?.Picture.Dispose(); };
        RefreshTranslation();
    }

    internal static void RefreshManagedTranslations()
    {
        PinToScreenForm[] open;
        lock (syncLock) open = forms.ToArray();
        foreach (var form in open)
        {
            if (form.IsDisposed || !form.IsHandleCreated) continue;
            try { form.BeginInvoke(new Action(form.RefreshTranslation)); }
            catch (InvalidOperationException) when (form.IsDisposed || !form.IsHandleCreated) { }
        }
    }

    private void RefreshTranslation()
    {
        if (translateButton is null || IsDisposed) return;
        bool enabled = pinEnabled?.Invoke() == true;
        if (!enabled)
        {
            CancelPin();
            showTranslation = false;
            translationArea = null;
            areaStart = null;
            pinLayout = null;
            translated?.Picture.Dispose();
            translated = null;
        }
        translateButton.Visible = translateAction!.Visible = enabled;
        translateButton.Enabled = translateAction.Enabled = pinRequest is null;
        translateButton.Text = pinRequest is not null ? PinText("翻译中…", "Translating…") : PinText("翻译", "Translate");
        translateAction.Text = translationArea.HasValue ? PinText("翻译选中文字", "Translate selection") : PinText("翻译贴图", "Translate image");
        retranslateAction!.Visible = toggleAction!.Visible = copyTextAction!.Visible = saveTranslated!.Visible = enabled && translated is not null;
        comparison!.Visible = enabled && translated is not null && !Minimized;
        comparison.Text = showTranslation ? PinText("查看原图", "Show original") : PinText("查看译图", "Show translation");
        if (translated is not null && !translationArea.HasValue && pinRequest is null)
            translateButton.Text = translateAction.Text = comparison.Text;
        comparison.AccessibleName = comparison.Text;
        PositionComparison();
        UpdateControls();
        Invalidate();
    }

    private void PositionComparison()
    {
        if (comparison is null) return;
        comparison.Location = new Point(Math.Max(0, (ClientSize.Width-comparison.Width)/2), Math.Max(0, ClientSize.Height-comparison.Height-4));
    }
    private void Status(string message) => pinTip?.Show(message, this, Math.Max(0,Width/2-30), 10, 3500);
    private void CancelPin()
    {
        var request = pinRequest;
        pinRequest = null;
        request?.Cancel();
    }
    private void TogglePin()
    {
        if (pinEnabled?.Invoke() != true || translated is null) return;
        showTranslation = !showTranslation;
        RefreshTranslation();
    }
    protected override bool ProcessCmdKey(ref Message msg, Keys keyData)
    {
        if (keyData == Keys.Tab && translated is not null && pinEnabled?.Invoke() == true) { TogglePin(); return true; }
        return base.ProcessCmdKey(ref msg,keyData);
    }

    private async void TranslatePin(bool force)
    {
        if (pinEnabled?.Invoke() != true || IsDisposed) return;
        if (pinRequest is not null && !force) return;
        if (!force && !translationArea.HasValue && translated is not null) { TogglePin(); return; }
        if (pinHost is not { } host) { Status(PinText("翻译服务不可用", "Translation service unavailable")); return; }
        CancelPin();
        using var request = new CancellationTokenSource();
        pinRequest = request;
        RefreshTranslation();
        var selection = translationArea;
        try
        {
            using var source = new Bitmap(Image);
            var token = request.Token;
            var layout = pinLayout;
            if (layout is null)
            {
                layout = await Task.Run(() => new NativeOcrEngine().PinLayout(source, token), token);
                if (IsDisposed || token.IsCancellationRequested || pinRequest != request || pinEnabled?.Invoke() != true) return;
                pinLayout = layout;
            }
            var result = await Task.Run(() => PinTranslationEngine.Translate(source, layout, selection,
                host.PinTargetAsync, host.TranslateAsync, token), token);
            if (IsDisposed || token.IsCancellationRequested || pinRequest != request || pinEnabled?.Invoke() != true) { result.Picture.Dispose(); return; }
            translated?.Picture.Dispose();
            translated = result;
            translationArea = null;
            showTranslation = true;
        }
        catch (OperationCanceledException) when (request.IsCancellationRequested) { }
        catch (Exception error)
        {
            if (!IsDisposed && pinRequest == request) Status(error.Message switch {
                "ocrEmpty" => PinText("此区域没有可翻译的文字", "No translatable text in this area"),
                "pinUnchanged" => PinText("译文与原文相同", "The translation matches the original"),
                "pinRenderFailed" => PinText("译图生成失败，请重试", "Could not render the translation. Try again."),
                _ => PinText("翻译失败，请检查识别与翻译设置后重试。", "Translation failed. Check recognition and translation settings, then retry.") });
        }
        finally {
            if (pinRequest == request) pinRequest = null;
            if (!IsDisposed) RefreshTranslation();
        }
    }

    private Point SourcePoint(Point client)
    {
        int margin = Options.Border ? Options.BorderSize : 0;
        return new(Math.Clamp((client.X-margin)*100/ImageScale, 0, Image.Width),
                   Math.Clamp((client.Y-margin)*100/ImageScale, 0, Image.Height));
    }
    private bool BeginPinSelection(MouseEventArgs e)
    {
        if (pinEnabled?.Invoke() != true || Minimized || e.Button != MouseButtons.Left || ModifierKeys != Keys.Shift) return false;
        areaStart = SourcePoint(e.Location);
        translationArea = null;
        Capture = true;
        return true;
    }
    private bool UpdatePinSelection(MouseEventArgs e, bool complete)
    {
        if (areaStart is not { } start) return false;
        var end = SourcePoint(e.Location);
        var rect = Rectangle.FromLTRB(Math.Min(start.X,end.X),Math.Min(start.Y,end.Y),Math.Max(start.X,end.X),Math.Max(start.Y,end.Y));
        translationArea = rect.Width > 0 && rect.Height > 0 ? rect : null;
        if (complete) { areaStart = null; Capture = false; RefreshTranslation(); }
        Invalidate();
        return true;
    }
    private void DrawPinSelection(Graphics graphics)
    {
        if (Minimized || translationArea is not { } r) return;
        int margin = Options.Border ? Options.BorderSize : 0;
        using var pen = new Pen(SystemColors.Highlight, 2) { DashStyle = System.Drawing.Drawing2D.DashStyle.Dash };
        graphics.DrawRectangle(pen, margin + r.X*ImageScale/100, margin + r.Y*ImageScale/100, r.Width*ImageScale/100,r.Height*ImageScale/100);
    }
}
