using System.ComponentModel;
using System.Runtime.InteropServices;

namespace WinOne.Capture.NativeUI;

// This belongs to the existing native capture runtime. It neither activates
// the target window nor starts a separate UI process during scrolling.
internal sealed class NativeScrollProgress : Form
{
    private const int EscapeId = 0x574f;
    private readonly NativeMenuTheme theme;
    private readonly Label status = new() { AutoSize = true };
    private readonly Button stop = new() { AutoSize = true, FlatStyle = FlatStyle.Flat };
    private readonly Action requestStop;
    private readonly bool english;
    private readonly Color border;
    private bool requested, registered;
    [DllImport("user32.dll", SetLastError = true)] private static extern bool SetWindowDisplayAffinity(IntPtr window, uint affinity);
    [DllImport("user32.dll", SetLastError = true)] private static extern bool RegisterHotKey(IntPtr window, int id, uint modifiers, uint key);
    [DllImport("user32.dll", SetLastError = true)] private static extern bool UnregisterHotKey(IntPtr window, int id);

    public NativeScrollProgress(Action requestStop, bool dark, string? language)
    {
        this.requestStop = requestStop;
        english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        theme = new NativeMenuTheme(dark);
        Font = theme.Font; BackColor = theme.Background; ForeColor = theme.Foreground;
        border = SystemInformation.HighContrast ? SystemColors.WindowFrame : new NativeMenuColors(dark: dark).ToolStripBorder;
        AutoScaleMode = AutoScaleMode.Dpi; FormBorderStyle = FormBorderStyle.None;
        ShowInTaskbar = false; TopMost = true; StartPosition = FormStartPosition.Manual;
        Text = "WinOne"; Padding = new Padding(12); AutoSize = true; AutoSizeMode = AutoSizeMode.GrowAndShrink;
        var layout = new FlowLayoutPanel { AutoSize = true, Dock = DockStyle.Fill, WrapContents = false, Padding = Padding.Empty };
        status.Text = english ? "Scrolling capture · Esc to stop" : "滚动截图中 · Esc 停止";
        status.Margin = new Padding(0, 8, 14, 8);
        stop.Name = "scroll-stop"; stop.Text = english ? "Stop" : "停止";
        stop.AccessibleName = stop.Text; stop.Padding = new Padding(12, 3, 12, 3); stop.Margin = Padding.Empty;
        stop.BackColor = BackColor; stop.ForeColor = ForeColor; stop.UseVisualStyleBackColor = false;
        stop.FlatAppearance.BorderColor = border; stop.Click += (_, _) => RequestStop();
        layout.Controls.Add(status); layout.Controls.Add(stop); Controls.Add(layout);
        SetStyle(ControlStyles.ResizeRedraw, true);
    }
    protected override bool ShowWithoutActivation => true;
    protected override CreateParams CreateParams {
        get { var value = base.CreateParams; value.ExStyle |= 0x08000000 | 0x00000080; return value; }
    }
    public static NativeScrollProgress Open(Action requestStop, bool dark, string? language)
    {
        var window = new NativeScrollProgress(requestStop, dark, language);
        try { window.Start(); return window; }
        catch { window.Dispose(); throw; }
    }
    private void Start()
    {
        // Windows 11 supports WDA_EXCLUDEFROMCAPTURE. Do not continue with a
        // black rectangle or a visible control panel in the captured content.
        if (!SetWindowDisplayAffinity(Handle, 0x11))
            throw new Win32Exception(Marshal.GetLastWin32Error(), english ? "The capture controls could not be excluded. Try again." : "无法隐藏截图中的操作面板，请重试。");
        if (!RegisterHotKey(Handle, EscapeId, 0x4000, (uint)Keys.Escape))
            throw new Win32Exception(Marshal.GetLastWin32Error(), english ? "Esc is in use. Close the conflicting shortcut and try again." : "Esc 快捷键被占用，请关闭冲突快捷键后重试。");
        registered = true;
        PerformLayout();
        var screen = Screen.FromPoint(Cursor.Position).WorkingArea;
        Location = new Point(Math.Max(screen.Left, screen.Right - Width - 16), Math.Max(screen.Top, screen.Bottom - Height - 16));
        Show();
    }
    private void RequestStop()
    {
        if (requested) return;
        requested = true; stop.Enabled = false;
        // Reserve the measured status area for the remainder of this capture,
        // so changing its text does not move Stop away from the pointer.
        var measured = status.Size;
        status.AutoSize = false; // WinForms restores the pre-AutoSize width here.
        status.Size = measured;
        status.Text = english ? "Stopping…" : "正在停止…";
        requestStop();
    }
    protected override void WndProc(ref Message message)
    {
        if (message.Msg == 0x0021) { message.Result = new IntPtr(3); return; } // MA_NOACTIVATE still delivers the click.
        if (message.Msg == 0x0312 && message.WParam.ToInt32() == EscapeId) { RequestStop(); return; }
        base.WndProc(ref message);
    }
    protected override void OnPaint(PaintEventArgs e) { base.OnPaint(e); ControlPaint.DrawBorder(e.Graphics, ClientRectangle, border, ButtonBorderStyle.Solid); }
    protected override void Dispose(bool disposing)
    {
        if (disposing) {
            if (registered) {
                if (!UnregisterHotKey(Handle, EscapeId))
                    Console.Error.WriteLine("Scrolling stop shortcut cleanup: " + new Win32Exception(Marshal.GetLastWin32Error()).Message);
                // Destroying the owning HWND also releases its registration.
                registered = false;
            }
            theme.Dispose();
        }
        base.Dispose(disposing);
    }
}
