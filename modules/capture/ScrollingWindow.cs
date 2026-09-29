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
    private readonly PictureBox preview = new() { SizeMode = PictureBoxSizeMode.AutoSize };
    private bool closingAfterStop;
    private bool running;
    public ScrollingWindow(CaptureSettings settings)
    {
        Text = "WinOne · 滚动截图"; Width = 920; Height = 650; StartPosition = FormStartPosition.CenterScreen;
        Font = new Font("Microsoft YaHei UI", 10); BackColor = Color.FromArgb(248, 248, 246);
        manager = new(new ScrollingCaptureOptions { AutoUpload = false, ScrollDelay = settings.ScrollDelay, AutoScrollTop = settings.ScrollFromTop });
        var toolbar = new FlowLayoutPanel { Dock = DockStyle.Top, AutoSize = true, Padding = new Padding(12) };
        toolbar.Controls.AddRange([start, stop, accept, status]);
        var panel = new Panel { Dock = DockStyle.Fill, AutoScroll = true }; panel.Controls.Add(preview);
        Controls.Add(panel); Controls.Add(toolbar);
        ShareX.HelpersLib.ShareXResources.ApplyTheme(this, true);
        start.Click += async (_, _) => await CaptureScrollImage();
        stop.Click += (_, _) => manager.StopCapture();
        accept.Click += (_, _) => { DialogResult = DialogResult.OK; Close(); };
        Activated += (_, _) => { if (manager.IsCapturing) manager.StopCapture(); };
        FormClosing += (_, e) => { if (running) { closingAfterStop = true; manager.StopCapture(); e.Cancel = true; } };
    }
    public Bitmap? ResultCopy() => manager.Result is null ? null : new Bitmap(manager.Result);
    private async Task CaptureScrollImage()
    {
        if (running) return;
        running = true; start.Enabled = accept.Enabled = false;
        try
        {
            Hide(); await Task.Delay(200);
            if (!manager.SelectWindow()) { status.Text = "已取消选择"; return; }
            // Reopening this window or pressing Stop ends the capture without discarding its preview.
            WindowState = FormWindowState.Minimized; Show(); stop.Enabled = true;
            var result = await manager.StartCapture();
            preview.Image = manager.Result;
            status.Text = result switch { ScrollingCaptureStatus.Successful => "捕获完成", ScrollingCaptureStatus.PartiallySuccessful => "仅捕获部分内容，请检查预览", _ => "未能完成捕获，请重试" };
            accept.Enabled = manager.Result is not null;
        }
        catch (Exception error) { status.Text = error.Message; }
        finally
        {
            running = false; stop.Enabled = false; start.Enabled = true;
            if (closingAfterStop) { DialogResult = DialogResult.Cancel; Close(); }
            else { WindowState = FormWindowState.Normal; Show(); Activate(); }
        }
    }
    protected override void Dispose(bool disposing)
    {
        if (disposing) { preview.Image = null; manager.Dispose(); }
        base.Dispose(disposing);
    }
}
