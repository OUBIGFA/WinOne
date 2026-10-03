namespace WinOne.Capture;

// Paints the same frame across the native/WebView handoff. It has no controls
// and does not activate; the editor explicitly releases it after its first paint.
internal sealed class CaptureHoldWindow : Form
{
    private CapturedFrame? borrowed;
    private Bitmap? owned;
    private readonly Rectangle desktop, selection;
    private readonly System.Windows.Forms.Timer deadline = new() { Interval = 15000 };
    public string? DraftId { get; private set; }
    public CaptureHoldWindow(CapturedFrame frame, Rectangle selected)
    {
        borrowed = frame; desktop = frame.Bounds; selection = selected;
        AutoScaleMode = AutoScaleMode.None; FormBorderStyle = FormBorderStyle.None;
        StartPosition = FormStartPosition.Manual; Bounds = desktop;
        ShowInTaskbar = false; TopMost = true; DoubleBuffered = true; Cursor = Cursors.Cross;
        deadline.Tick += (_, _) => { Console.Error.WriteLine("Capture handoff display timed out"); Close(); };
        Shown += (_, _) => deadline.Start();
    }
    protected override bool ShowWithoutActivation => true;
    protected override CreateParams CreateParams { get { var value = base.CreateParams; value.ExStyle |= 0x08000080; return value; } }
    public void Adopt(CapturedFrame frame, string draftId)
    {
        owned = frame.Detach(); borrowed = null; DraftId = draftId;
        deadline.Stop(); deadline.Start();
    }
    protected override void OnPaint(PaintEventArgs e)
    {
        if (owned is not null) e.Graphics.DrawImageUnscaled(owned, Point.Empty);
        else borrowed?.DrawUnscaled(e.Graphics);
        if (selection == desktop) return;
        var local = selection; local.Offset(-desktop.X, -desktop.Y);
        using var shade = new Region(ClientRectangle); shade.Exclude(local);
        using var brush = new SolidBrush(Color.FromArgb(100, 0, 0, 0)); e.Graphics.FillRegion(brush, shade);
        using var light = new Pen(Color.FromArgb(231, 231, 226), 3);
        using var dark = new Pen(Color.FromArgb(44, 44, 42), 1);
        e.Graphics.DrawRectangle(light, local.X, local.Y, local.Width - 1, local.Height - 1);
        e.Graphics.DrawRectangle(dark, local.X, local.Y, local.Width - 1, local.Height - 1);
    }
    protected override void Dispose(bool disposing)
    {
        if (disposing) { deadline.Dispose(); owned?.Dispose(); owned = null; borrowed = null; }
        base.Dispose(disposing);
    }
}
