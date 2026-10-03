using ShareX.ScreenCaptureLib;

namespace WinOne.Capture;

// The capture/stitching engine stays upstream; this window only presents local preview actions.
internal sealed class ScrollingWindow : Form
{
    private readonly ScrollingCaptureManager manager;
    private readonly Label status = new() { AutoSize = true, Text = "选择窗口后开始滚动捕获" };
    private readonly Button start = new() { Text = "选择窗口", AutoSize = true };
    private readonly Button stop = new() { Text = "停止", AutoSize = true, Enabled = false };
    private readonly Button accept = new() { Text = "使用截图", AutoSize = true, Enabled = false };
    private readonly Button edit = new() { AutoSize = true, Enabled = false };
    private readonly PictureBox preview = new() { SizeMode = PictureBoxSizeMode.AutoSize };
    private readonly Func<ScrollingCaptureManager, bool>? selectTarget;
    private readonly bool english;
    private readonly Font uiFont = new("Segoe UI", 10);
    private Bitmap? result;
    private bool closingAfterStop;
    private bool running;
    private bool userStopped;
    public bool EditRequested { get; private set; }
    private string T(string zh, string en) => english ? en : zh;
    public ScrollingWindow(CaptureSettings settings, string? language = null, Func<ScrollingCaptureManager, bool>? selectTarget = null, bool allowEdit = true)
    {
        this.selectTarget = selectTarget;
        english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        Text = T("WinOne · 滚动截图", "WinOne · Scrolling capture"); Width = 920; Height = 650; MinimumSize = new Size(640, 420); StartPosition = FormStartPosition.CenterScreen;
        Font = uiFont; KeyPreview = true;
        var dark = ShareX.HelpersLib.ShareXResources.Theme.IsDarkTheme;
        BackColor = SystemInformation.HighContrast ? SystemColors.Window : dark ? Color.FromArgb(44, 44, 42) : Color.White;
        ForeColor = SystemInformation.HighContrast ? SystemColors.WindowText : dark ? Color.FromArgb(238, 238, 238) : Color.FromArgb(36, 36, 36);
        status.Text = T("选择窗口内需要滚动的区域。", "Select the scrolling area inside a window.");
        start.Text = T("选择区域", "Select area"); stop.Text = T("停止", "Stop"); accept.Text = T("使用截图", "Use capture"); edit.Text = T("编辑", "Edit"); edit.Visible = allowEdit;
        var cancel = new Button { Text = T("取消", "Cancel"), AutoSize = true };
        manager = new(new ScrollingCaptureOptions { AutoUpload = false, ScrollDelay = settings.ScrollDelay, AutoScrollTop = settings.ScrollFromTop });
        var toolbar = new FlowLayoutPanel { Dock = DockStyle.Top, AutoSize = true, Padding = new Padding(12) };
        foreach (var button in new[] { start, stop, accept, edit, cancel })
        {
            button.Name = button == start ? "scroll-start" : button == stop ? "scroll-stop" : button == accept ? "scroll-use" : button == edit ? "scroll-edit" : "scroll-cancel";
            button.AccessibleName = button.Text; button.FlatStyle = FlatStyle.Flat; button.Padding = new Padding(10, 4, 10, 4); button.Margin = new Padding(0, 0, 8, 0);
            button.UseVisualStyleBackColor = false; button.BackColor = button == accept ? ForeColor : BackColor; button.ForeColor = button == accept ? BackColor : ForeColor;
            button.FlatAppearance.BorderColor = SystemInformation.HighContrast ? SystemColors.WindowFrame : dark ? Color.FromArgb(69, 69, 65) : Color.FromArgb(215, 215, 212);
        }
        toolbar.Controls.AddRange([start, stop, accept, edit, cancel]);
        status.Dock = DockStyle.Bottom; status.Padding = new Padding(16, 8, 16, 12);
        var panel = new Panel { Dock = DockStyle.Fill, AutoScroll = true }; panel.Controls.Add(preview);
        Controls.Add(panel); Controls.Add(toolbar); Controls.Add(status);
        start.Click += async (_, _) => await CaptureScrollImage();
        stop.Click += (_, _) => StopCapture();
        accept.Click += (_, _) => { DialogResult = DialogResult.OK; Close(); };
        edit.Click += (_, _) => { EditRequested = true; DialogResult = DialogResult.OK; Close(); };
        cancel.Click += (_, _) => Close();
        KeyDown += (_, e) => { if (e.KeyCode == Keys.Escape) { e.Handled = true; Close(); } };
        Activated += (_, _) => { if (manager.IsCapturing) StopCapture(); };
        FormClosing += (_, e) => { if (running) { closingAfterStop = true; manager.StopCapture(); e.Cancel = true; } };
    }
    // Pass the same bitmap to export/editing. Closing the preview must not
    // dispose it after ownership has moved to the caller.
    internal Bitmap? TakeResult()
    {
        if (running) throw new InvalidOperationException("Capture is running");
        preview.Image = null;
        var image = result;
        result = null;
        accept.Enabled = edit.Enabled = false;
        return image;
    }
    private void StopCapture() { userStopped = true; manager.StopCapture(); }
    // Takes ownership even on failure. The previous result survives capture
    // retries until a new result has actually reached the preview.
    internal void OwnPreview(Bitmap image)
    {
        if (ReferenceEquals(result, image)) return;
        if (IsDisposed) { image.Dispose(); throw new ObjectDisposedException(nameof(ScrollingWindow)); }
        var previous = result;
        try { preview.Image = image; }
        catch { image.Dispose(); throw; }
        result = image;
        previous?.Dispose();
        accept.Enabled = edit.Enabled = !running;
    }
    private async Task CaptureScrollImage()
    {
        if (running) return;
        running = true; userStopped = false; start.Enabled = accept.Enabled = edit.Enabled = false;
        try
        {
            // Hiding a modal Form ends ShowDialog with Cancel. Minimize instead
            // so the asynchronous selector keeps the capture session alive.
            WindowState = FormWindowState.Minimized; await Task.Delay(200);
            if (closingAfterStop) return;
            if (selectTarget is null) throw new InvalidOperationException(T("选区工具不可用", "Selection tool unavailable"));
            if (!selectTarget(manager)) { status.Text = result is null ? T("已取消选择", "Selection cancelled") : T("已取消选择，保留上次截图", "Selection cancelled. Previous capture retained."); return; }
            // Reopening this window or pressing Stop ends the capture without discarding its preview.
            WindowState = FormWindowState.Minimized; stop.Enabled = true;
            status.Text = T("滚动捕获中，切回此窗口可停止。", "Capturing. Return to this window to stop.");
            var outcome = await manager.StartCapture();
            var captured = manager.TakeResult();
            if (captured is not null) OwnPreview(captured);
            status.Text = outcome switch { ScrollingCaptureStatus.Successful => T("捕获完成", "Capture complete"), ScrollingCaptureStatus.PartiallySuccessful => T("仅捕获部分内容，请检查预览", "Partial capture. Check the preview."), _ => result is null ? T("捕获失败，请重试", "Capture failed. Try again.") : T("捕获未完成，请检查预览", "Capture incomplete. Check the preview.") };
            if (userStopped) status.Text = result is null ? T("已停止，未生成截图", "Stopped. No capture available.") : T("已停止，请检查预览", "Stopped. Check the preview.");
        }
        catch (Exception error)
        {
            // Failed stitching can leave an unusable partial bitmap. Keep only
            // the previous accepted preview while reporting the failure.
            using var failed = manager.TakeResult();
            Console.Error.WriteLine(error);
            var message = error.Message == "WinOne.ScrollingTargetChanged" ? T("目标窗口已变化，请重新选择区域。", "Target window changed. Select the area again.") : error.Message;
            status.Text = result is null ? message : T("本次捕获失败，保留上次截图：", "Capture failed. Previous capture retained: ") + message;
        }
        finally
        {
            running = false; stop.Enabled = false; start.Enabled = true; accept.Enabled = edit.Enabled = result is not null;
            if (result is not null) { start.Text = T("重新截取", "Capture again"); start.AccessibleName = start.Text; }
            if (closingAfterStop) { DialogResult = DialogResult.Cancel; Close(); }
            else { WindowState = FormWindowState.Normal; Activate(); }
        }
    }
    protected override void Dispose(bool disposing)
    {
        if (disposing) { preview.Image = null; result?.Dispose(); result = null; manager.Dispose(); uiFont.Dispose(); }
        base.Dispose(disposing);
    }
}
