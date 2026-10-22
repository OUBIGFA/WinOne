using System.Diagnostics;
using System.Runtime.InteropServices;

namespace WinOne.Capture.NativeUI;

// A small standard WinForms tool window, owned by the existing capture worker.
// The original editor is only hidden while recording and is retained verbatim
// on cancellation/failure; no replacement selection window or UI runtime exists.
internal sealed class NativeRecordingWindow : Form
{
    private readonly NativeMenuTheme theme;
    private readonly IRecordingEngine engine;
    private readonly Func<IWin32Window, string?>? chooseOutput;
    private readonly Form capture;
    private readonly Rectangle selection;
    private readonly CaptureSettings settings;
    private readonly string format;
    private readonly bool english;
    private readonly TableLayoutPanel layout = new() { AutoSize = true, Dock = DockStyle.Fill, ColumnCount = 1, Padding = Padding.Empty };
    private readonly TableLayoutPanel options = new() { AutoSize = true, Dock = DockStyle.Top, ColumnCount = 2, Margin = new Padding(0, 12, 0, 12) };
    private readonly NumericUpDown fps = new() { Minimum = 1, Maximum = 60, Width = 100 };
    private readonly ComboBox codec = new() { DropDownStyle = ComboBoxStyle.DropDownList, Width = 160 };
    private readonly NumericUpDown quality = new() { Minimum = 1, Maximum = 100, Value = 85, Width = 100 };
    private readonly NumericUpDown limit = new() { Minimum = 1, Maximum = 600, Value = 120, Width = 100 };
    private readonly CheckBox cursor = new() { Checked = true, AutoSize = true, FlatStyle = FlatStyle.Flat };
    private readonly Label status = new() { AutoSize = true, MaximumSize = new Size(440, 0), Margin = new Padding(0, 4, 0, 8), AccessibleRole = AccessibleRole.StaticText };
    private readonly Label detail = new() { AutoSize = true, MaximumSize = new Size(440, 0), Visible = false, Margin = new Padding(0, 0, 0, 12) };
    // ShareX's existing ClickThrough preserves the first click on an inactive
    // ToolStrip instead of treating it only as window activation.
    private readonly ShareX.HelpersLib.ToolStripEx actions = new() { ClickThrough = true, ShowItemToolTips = false, AutoSize = true, GripStyle = ToolStripGripStyle.Hidden, Dock = DockStyle.Top, CanOverflow = false, Padding = Padding.Empty };
    private readonly ToolTip actionHints = new() { ShowAlways = true, UseAnimation = false, UseFading = false };
    private readonly ToolStripButton pin = new(), start = new(), pause = new(), stop = new(), cancel = new(), folder = new();
    private readonly System.Windows.Forms.Timer timer = new() { Interval = 250 };
    private RecordingStatus current = new();
    private Task operation = Task.CompletedTask;
    private bool busy, polling, closing, started;
    private Image? pauseImage, resumeImage, pinImage, pinnedImage;
    private NativeRecordingRegion? regionFrame;
    internal Form? RegionWindow => regionFrame;
    internal RecordingStatus? Result { get; private set; }
    internal Task CurrentOperation => operation;
    private string T(string zh, string en) => english ? en : zh;

    internal NativeRecordingWindow(Form capture, Rectangle selection, CaptureSettings settings, string format, string? language, IRecordingEngine? engine = null, Func<IWin32Window, string?>? chooseOutput = null)
    {
        this.chooseOutput = chooseOutput;
        this.capture = capture; this.selection = selection; this.settings = settings; this.format = format;
        this.engine = engine ?? new NativeRecordingEngine();
        english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        theme = new NativeMenuTheme(ShareX.HelpersLib.ShareXResources.Theme.IsDarkTheme);
        Font = theme.Font; BackColor = theme.Background; ForeColor = theme.Foreground;
        AutoScaleMode = AutoScaleMode.Dpi; FormBorderStyle = FormBorderStyle.FixedToolWindow;
        MaximizeBox = false; MinimizeBox = false; ShowInTaskbar = false; TopMost = true;
        StartPosition = FormStartPosition.Manual; Padding = new Padding(16);
        AutoSize = true; AutoSizeMode = AutoSizeMode.GrowAndShrink; MinimumSize = new Size(430, 0);
        Text = format == "gif" ? T("WinOne · 录制 GIF", "WinOne · Record GIF") : T("WinOne · 录制视频", "WinOne · Record video");
        status.Text = $"{format.ToUpperInvariant()} · {selection.Width & ~1} × {selection.Height & ~1}";
        layout.Controls.Add(status); layout.Controls.Add(options); layout.Controls.Add(detail); layout.Controls.Add(actions); Controls.Add(layout);
        options.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 150)); options.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        fps.Value = format == "gif" ? 15 : 30; if (format == "gif") fps.Maximum = 30;
        codec.Items.AddRange(["H.264", "HEVC"]); codec.SelectedIndex = 0;
        AddOption(T("每秒帧数", "Frames per second"), fps);
        if (format == "mp4") AddOption(T("视频编码", "Video codec"), codec);
        else { AddOption(T("画质（1–100）", "Quality (1–100)"), quality); AddOption(T("自动停止（秒）", "Stop after (seconds)"), limit); }
        cursor.Text = T("录制鼠标指针", "Record mouse pointer"); cursor.FlatAppearance.CheckedBackColor = theme.Background; cursor.FlatAppearance.BorderColor = new NativeMenuColors(dark: ShareX.HelpersLib.ShareXResources.Theme.IsDarkTheme).ToolStripBorder; AddOption("", cursor);
        var note = new Label { AutoSize = true, MaximumSize = new Size(420, 0), Text = format == "gif"
            ? T("静止画面自动合并。达到时长或 1800 帧时保存；GIF 不含声音。", "Static frames are merged. Saves at the time limit or 1800 frames; GIF has no audio.")
            : T("使用屏幕选区录制，不包含工具栏或声音。", "Records the selected screen area without the controls or audio."), Margin = new Padding(0, 8, 0, 0) };
        options.Controls.Add(note, 0, options.RowCount++); options.SetColumnSpan(note, 2);
        Configure(pin, "recording-pin", T("取消固定", "Unpin window"), "pin");
        pin.DisplayStyle = ToolStripItemDisplayStyle.Image;
        pinImage = pin.Image; pinnedImage = LoadIcon("pin-active");
        SetPinned(TopMost);
        Configure(start, "recording-start", T("开始录制", "Start recording"), format == "gif" ? "gif" : "video");
        Configure(pause, "recording-pause", T("暂停", "Pause"), "pause");
        Configure(stop, "recording-stop", T("停止并保存", "Stop and save"), "stop");
        Configure(cancel, "recording-cancel", T("取消", "Cancel"), "cancel");
        Configure(folder, "recording-folder", T("打开文件夹", "Open folder"), "saveAs");
        pauseImage = pause.Image; resumeImage = LoadIcon("resume");
        pause.Visible = stop.Visible = folder.Visible = false;
        actions.Items.AddRange([pin, new ToolStripSeparator(), start, pause, stop, folder, new ToolStripSeparator(), cancel]); theme.Apply(actions);
        // A tooltip is its own top-level HWND, not part of the protected panel.
        // Use the native ToolTip's before-show event, never a post-show timer.
        // ToolStrip sends mouse events to the item under the pointer, not its
        // own MouseMove handler (WinForms ToolStrip.OnMouseMove). Retain its
        // native hover timing and show one protected standard ToolTip.
        foreach (var item in new[] { pin, start, pause, stop, cancel, folder }) {
            item.MouseHover += (_, _) => {
                if (!busy && item.Available && item.Enabled)
                    actionHints.Show(item.ToolTipText, actions, item.Bounds.Left, actions.Height + 4, actionHints.AutoPopDelay);
            };
            item.MouseLeave += (_, _) => actionHints.Hide(actions);
        }
        actions.MouseLeave += (_, _) => actionHints.Hide(actions);
        actionHints.Popup += (_, e) => {
            try { if (!RecordingWindowProtection.ExcludeToolTips()) throw new InvalidOperationException(T("无法保护录制提示窗口。", "The recording tooltip could not be protected.")); }
            catch (Exception error) { e.Cancel = true; ShowError(error.Message); }
        };
        pin.Click += (_, _) => SetPinned(!TopMost);
        start.Click += (_, _) => Begin(StartAsync);
        pause.Click += (_, _) => Begin(() => PauseAsync());
        stop.Click += (_, _) => Begin(() => FinishAsync(false));
        cancel.Click += (_, _) => {
            if (busy) return;
            if (!current.Active) { Close(); return; }
            if (MessageBox.Show(this, T("放弃本次录制并删除未完成文件？", "Discard this recording and delete its unfinished file?"), Text,
                MessageBoxButtons.YesNo, MessageBoxIcon.Question, MessageBoxDefaultButton.Button2) == DialogResult.Yes) Begin(() => FinishAsync(true));
        };
        folder.Click += (_, _) => {
            try { if (Result?.Path is { } path) Process.Start(new ProcessStartInfo("explorer.exe") { UseShellExecute = true, Arguments = $"/select,\"{path}\"" }); }
            catch (Exception error) { ShowError(error.Message); }
        };
        timer.Tick += async (_, _) => {
            if (busy || polling || !current.Active) return;
            polling = true;
            try { Apply(await this.engine.CallAsync("status")); }
            catch (Exception error) { ShowError(error.Message); }
            finally { polling = false; }
        };
        FormClosing += OnClosing;
    }
    [DllImport("dwmapi.dll")] private static extern int DwmSetWindowAttribute(IntPtr window, int attribute, ref int value, int size);
    protected override void OnHandleCreated(EventArgs e)
    {
        base.OnHandleCreated(e);
        RecordingWindowProtection.Exclude(Handle);
        // Windows owns the frame and rounded corners; only select its native
        // dark caption so the tool window agrees with the global theme.
        var dark = ShareX.HelpersLib.ShareXResources.Theme.IsDarkTheme ? 1 : 0;
        var result = DwmSetWindowAttribute(Handle, 20, ref dark, sizeof(int));
        if (result < 0) Console.Error.WriteLine($"Recording window theme: 0x{result:X8}");
    }
    protected override void OnShown(EventArgs e)
    {
        base.OnShown(e);
        PlaceControlPanel();
        ShowRegion();
    }
    private void PlaceControlPanel()
    {
        PerformLayout();
        var monitor = Screen.FromRectangle(selection);
        var areas = new[] { monitor.WorkingArea }.Concat(Screen.AllScreens.Where(screen => screen.DeviceName != monitor.DeviceName).Select(screen => screen.WorkingArea)).ToArray();
        Bounds = ToolbarPlacement.PlaceRecording(selection, areas, Size);
    }
    private void ShowRegion()
    {
        regionFrame ??= new NativeRecordingRegion(new Rectangle(selection.X, selection.Y, selection.Width & ~1, selection.Height & ~1));
        if (!regionFrame.Visible) regionFrame.Show();
    }
    private void SetPinned(bool pinned)
    {
        // Same Lucide Pin and 45-degree pinned state as the shared result UI.
        TopMost = pinned; pin.Checked = pinned; pin.Image = pinned ? pinnedImage : pinImage;
        pin.Text = pin.AccessibleName = pin.ToolTipText = pinned ? T("取消固定", "Unpin window") : T("固定窗口", "Pin window");
    }
    [DllImport("user32.dll")] private static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] private static extern IntPtr GetAncestor(IntPtr window, uint flags);
    protected override void OnDeactivate(EventArgs e)
    {
        base.OnDeactivate(e);
        if (TopMost || busy || closing || IsDisposed || !IsHandleCreated) return;
        // Deactivation can be an owned picker/menu. Check the settled owner
        // once after native dispatch, never with a focus timer or global hook.
        BeginInvoke((Action)(() => {
            if (TopMost || busy || closing || IsDisposed) return;
            var foreground = GetForegroundWindow();
            if (foreground == Handle || GetAncestor(foreground, 3) == Handle) return;
            Close(); // OnClosing stops and saves an active recording, not discard.
        }));
    }
    private void AddOption(string title, Control control)
    {
        var row = options.RowCount++; var label = new Label { AutoSize = true, Text = title, Margin = new Padding(0, 6, 16, 8) };
        control.AccessibleName = title.Length > 0 ? title : cursor.Text; control.Margin = new Padding(0, 3, 0, 8);
        control.BackColor = theme.Background; control.ForeColor = theme.Foreground;
        options.Controls.Add(label, 0, row); options.Controls.Add(control, 1, row);
    }
    private void Configure(ToolStripButton button, string name, string text, string icon)
    {
        button.Name = name; button.Text = button.AccessibleName = button.ToolTipText = text;
        button.DisplayStyle = ToolStripItemDisplayStyle.ImageAndText; button.Image = LoadIcon(icon);
        button.Padding = new Padding(6); button.Margin = new Padding(0, 0, 4, 0);
    }
    private Image LoadIcon(string name)
    {
        var suffix = ShareX.HelpersLib.ShareXResources.Theme.IsDarkTheme ? "-dark" : "";
        using var stream = typeof(NativeRecordingWindow).Assembly.GetManifestResourceStream($"WinOne.Capture.Icons.{name}{suffix}-36.png") ?? throw new InvalidOperationException("Missing recording icon: " + name);
        using var image = Image.FromStream(stream); return new Bitmap(image);
    }
    private void Begin(Func<Task> action)
    {
        if (busy || closing) return;
        actionHints.Hide(actions);
        busy = true; actions.Enabled = false; options.Enabled = false;
        var completion = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        operation = completion.Task; // Publish before entering a modal file picker.
        _ = RunAction(action, completion);
    }
    private async Task RunAction(Func<Task> action, TaskCompletionSource completion)
    {
        try { await action(); }
        catch (Exception error) {
            ShowError(error.Message);
            if (!started) ClearRegion();
        } finally {
            busy = false;
            if (!IsDisposed) { actions.Enabled = true; options.Enabled = !started; }
            completion.TrySetResult();
        }
    }
    private async Task StartAsync()
    {
        var monitor = Screen.FromRectangle(selection);
        if (!monitor.Bounds.Contains(selection)) throw new InvalidOperationException(T("请选择同一显示器内的录制区域。", "Select an area within a single display."));
        var suggested = CaptureFileName.RenderRecording(settings.FileNameTemplate, DateTime.Now, format);
        using var picker = new SaveFileDialog { Title = Text, Filter = format == "gif" ? "GIF (*.gif)|*.gif" : "MP4 (*.mp4)|*.mp4", DefaultExt = format,
            FileName = suggested, AddExtension = true, CheckPathExists = true, OverwritePrompt = false,
            InitialDirectory = Directory.Exists(settings.OutputDirectory) ? settings.OutputDirectory : Environment.GetFolderPath(Environment.SpecialFolder.MyVideos) };
        var path = chooseOutput is null ? (picker.ShowDialog(this) == DialogResult.OK ? picker.FileName : null) : chooseOutput(this);
        if (path is null) return;
        if (File.Exists(path)) throw new InvalidOperationException(T("文件已存在，请选择其他名称。原文件未改动。", "The file exists. Choose another name; the original is unchanged."));
        // The border encloses the exact even-pixel encoder crop, and stays
        // visible through pause. Its HWND is excluded just like the controls.
        ShowRegion();
        // Also protect an existing screenshot-toolbar tooltip before WGC starts.
        RecordingWindowProtection.ExcludeToolTips();
        var config = RecordingConfig.FromSelection(selection, monitor.DeviceName, path, format, capture.DeviceDpi / 96d) with {
            Fps = (int)fps.Value, Codec = codec.SelectedIndex == 1 ? "hevc" : "h264", Quality = (int)quality.Value,
            GifMaxSeconds = (int)limit.Value, Cursor = cursor.Checked, ExcludeWindows = [unchecked((ulong)Handle.ToInt64()), unchecked((ulong)capture.Handle.ToInt64()), unchecked((ulong)regionFrame!.Handle.ToInt64())],
        };
        status.Text = T("正在准备录制…", "Preparing recording…"); detail.Visible = false;
        capture.Hide();
        regionFrame.Show();
        var value = await engine.CallAsync("start_recording", config);
        started = value.Active; Apply(value);
        if (started) { PlaceControlPanel(); timer.Start(); }
    }
    private async Task PauseAsync() => Apply(await engine.CallAsync(current.Phase == "paused" ? "resume_recording" : "pause_recording"));
    private async Task FinishAsync(bool discard)
    {
        timer.Stop(); status.Text = discard ? T("正在取消…", "Cancelling…") : T("正在保存…", "Saving…");
        Apply(await engine.CallAsync(discard ? "cancel_recording" : "stop_recording"));
        if (discard && current.Phase == "cancelled") { closing = true; Close(); }
    }
    private void Apply(RecordingStatus value)
    {
        if (IsDisposed) return;
        current = value;
        if (value.Phase == "failed") started = false;
        var active = value.Active; var elapsed = TimeSpan.FromMilliseconds(value.DurationMs);
        start.Visible = !started && !active; options.Visible = !started && !active;
        pause.Visible = active; stop.Visible = active;
        pause.Text = pause.AccessibleName = pause.ToolTipText = value.Phase == "paused" ? T("继续", "Resume") : T("暂停", "Pause");
        pause.Image = value.Phase == "paused" ? resumeImage : pauseImage;
        status.Text = $"{format.ToUpperInvariant()} · {(int)elapsed.TotalHours:00}:{elapsed.Minutes:00}:{elapsed.Seconds:00}" + (value.Phase == "paused" ? T(" · 已暂停", " · Paused") : "");
        if (!active) {
            timer.Stop(); ClearRegion();
            if (value.Phase == "completed") {
                Result = value; start.Visible = false; options.Visible = false; folder.Visible = true;
                status.Text = T("录制已保存", "Recording saved"); cancel.Text = cancel.AccessibleName = T("关闭", "Close");
                var reason = value.Reason switch { "diskLow" => T("磁盘空间不足 200 MB，已自动保存。", "Saved automatically: less than 200 MB remains."),
                    "gifLimit" => T("已达到 GIF 录制限制，自动保存。", "Saved automatically at the GIF limit."),
                    "sourceClosed" => T("录制源已关闭，已保存收到的画面。", "The source closed; received frames were saved."), _ => "" };
                detail.Text = $"{value.Path}\n{(int)elapsed.TotalHours:00}:{elapsed.Minutes:00}:{elapsed.Seconds:00} · {value.Bytes / 1048576d:0.00} MB · {value.EncodedFrames} " + T("帧", "frames") + (reason.Length > 0 ? "\n" + reason : ""); detail.Visible = true;
            } else if (value.Phase == "failed") {
                ShowError((value.Error ?? T("录制失败，请重试。", "Recording failed. Try again.")) + (value.Path is { } path ? "\n" + T("未完成文件：", "Unfinished file: ") + path : ""));
                // Keep the live panel and retry entry above the desktop. The
                // caller restores the editor only after this panel closes;
                // showing its topmost frozen canvas here can cover the panel.
            }
        }
        PerformLayout();
    }
    private void ShowError(string message) { if (IsDisposed) return; detail.Text = message; detail.Visible = true; }
    private void ClearRegion() { regionFrame?.Dispose(); regionFrame = null; }
    private void OnClosing(object? sender, FormClosingEventArgs e)
    {
        if (closing) return;
        if (busy) { e.Cancel = true; return; }
        if (current.Active) { e.Cancel = true; Begin(async () => { await FinishAsync(false); if (current.Phase == "completed") { closing = true; Close(); } }); }
    }
    internal async Task ShutdownAsync()
    {
        // Host cancellation preserves a recording by stopping, not discarding.
        await operation;
        if (IsDisposed) return;
        if (current.Active) {
            busy = true; actions.Enabled = false;
            try { await FinishAsync(false); } catch (Exception error) { ShowError(error.Message); }
            finally { busy = false; if (!IsDisposed) actions.Enabled = true; }
            if (current.Active) return; // A failed stop must not discard a live recording.
        }
        closing = true; Close();
    }
    internal static async Task<RecordingStatus?> RunAsync(Form capture, Rectangle selection, CaptureSettings settings, string format, string? language, CancellationToken cancellation, IRecordingEngine? engine = null, Action<Form>? shown = null)
    {
        cancellation.ThrowIfCancellationRequested();
        using var window = new NativeRecordingWindow(capture, selection, settings, format, language, engine);
        var finished = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        window.FormClosed += (_, _) => finished.TrySetResult();
        try {
            // The caller owns restoration even if layout, HWND protection or
            // first Show fails. Do not leave a disabled topmost screenshot over
            // a new/failed recording panel during the first cold launch.
            capture.Hide();
            window.Show(); shown?.Invoke(window);
            using var cancel = cancellation.Register(() => {
                if (!window.IsDisposed && window.IsHandleCreated) window.BeginInvoke((Action)(async () => { await window.ShutdownAsync(); }));
            });
            await finished.Task; return window.Result;
        } finally { if (window.Result is null && !capture.IsDisposed) capture.Show(); }
    }
    protected override void Dispose(bool disposing)
    {
        if (disposing) {
            timer.Dispose(); actionHints.Dispose(); ClearRegion();
            pin.Image = null; pinImage?.Dispose(); pinnedImage?.Dispose();
            foreach (var item in new[] { start, stop, cancel, folder }) item.Image?.Dispose();
            pause.Image = null; pauseImage?.Dispose(); resumeImage?.Dispose();
        }
        base.Dispose(disposing);
        if (disposing) theme.Dispose();
    }
}
