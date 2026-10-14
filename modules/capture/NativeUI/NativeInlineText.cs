using System.Runtime.InteropServices;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// Feasibility probe: Windows EDIT owns text, selection, undo and IME.
// WM_CTLCOLOREDIT supplies the existing canvas behind the borderless input.
internal sealed class NativeInlineText : TextBox
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    [DllImport("user32.dll", EntryPoint = "SendMessageW")] private static extern IntPtr SendRect(IntPtr window, uint message, IntPtr w, ref Rect rect);
    [DllImport("gdi32.dll")] private static extern IntPtr CreatePatternBrush(IntPtr bitmap);
    [DllImport("gdi32.dll")] private static extern bool DeleteObject(IntPtr value);
    [DllImport("gdi32.dll")] private static extern int SetBkMode(IntPtr dc, int mode);
    [DllImport("gdi32.dll")] private static extern uint SetTextColor(IntPtr dc, int color);
    [DllImport("gdi32.dll")] private static extern bool SetBrushOrgEx(IntPtr dc, int x, int y, IntPtr previous);
    [StructLayout(LayoutKind.Sequential)] private struct Rect { public int Left, Top, Right, Bottom; }
    private readonly ParentPaint parentPaint;
    private IntPtr backgroundBrush;
    private Font ownedFont;
    private float viewScale;
    private Point backgroundOrigin;
    private bool composing;
    private Action? pendingCommit;

    public NativeInlineText(Control parent, Bitmap background, Rectangle bounds, string text, float fontPixels = 24, Color? color = null, float viewScale = 1, Point? backgroundOrigin = null)
    {
        Multiline = true; WordWrap = true; AcceptsReturn = true;
        BorderStyle = System.Windows.Forms.BorderStyle.None; AutoSize = false;
        Bounds = bounds; MaxLength = 4096; ScrollBars = ScrollBars.None;
        ForeColor = color ?? Color.FromArgb(0xe2, 0x3b, 0x30);
        this.viewScale = viewScale;
        this.backgroundOrigin = backgroundOrigin ?? Point.Empty;
        ownedFont = new Font("Segoe UI", fontPixels * viewScale, FontStyle.Regular, GraphicsUnit.Pixel);
        Font = ownedFont;
        ReplaceBackground(background);
        parent.Controls.Add(this);
        parentPaint = new ParentPaint(parent, this);
        Text = text;
    }

    private void ReplaceBackground(Bitmap background)
    {
        var bitmap = background.GetHbitmap();
        IntPtr replacement;
        try { replacement = CreatePatternBrush(bitmap); }
        finally { DeleteObject(bitmap); }
        if (replacement == IntPtr.Zero) throw new InvalidOperationException("Could not prepare the inline text background");
        var previous = backgroundBrush; backgroundBrush = replacement;
        if (previous != IntPtr.Zero) DeleteObject(previous);
    }

    public void SetView(Bitmap background, Rectangle bounds, float scale, float fontPixels, Color color)
    {
        if (!float.IsFinite(scale) || scale < 0.2f || scale > 6) throw new ArgumentOutOfRangeException(nameof(scale));
        ReplaceBackground(background);
        viewScale = scale;
        MoveView(bounds);
        SetTextStyle(fontPixels, color);
        Invalidate();
    }
    public void MoveView(Rectangle bounds) { backgroundOrigin = bounds.Location; Bounds = bounds; Invalidate(); }

    public void SetTextStyle(float fontPixels, Color color)
    {
        if (!float.IsFinite(fontPixels) || fontPixels < 6 || fontPixels > 256 || color.A != 255) throw new ArgumentOutOfRangeException(nameof(fontPixels));
        // WinForms retains its existing Font when the assigned font compares
        // equal. A color-only change must not dispose that retained instance.
        var displayedPixels = fontPixels * viewScale;
        if (ownedFont.Size == displayedPixels) { ForeColor = color; Invalidate(); return; }
        var next = new Font("Segoe UI", displayedPixels, FontStyle.Regular, GraphicsUnit.Pixel);
        var previous = ownedFont;
        var start = SelectionStart; var length = SelectionLength;
        Font = ownedFont = next; ForeColor = color;
        // WM_SETFONT restores EDIT's font-derived margins. Keep the same zero
        // margins used at creation so resizing type does not shift its anchor.
        SendMessageW(Handle, 0x00d3, new IntPtr(3), IntPtr.Zero);
        Select(start, length); previous.Dispose(); Invalidate();
    }

    protected override void OnHandleCreated(EventArgs e)
    {
        base.OnHandleCreated(e);
        SendMessageW(Handle, 0x00d3, new IntPtr(3), IntPtr.Zero); // EM_SETMARGINS: both margins zero.
    }

    public Rectangle FormattingRectangle
    {
        get { var rect = new Rect(); SendRect(Handle, 0x00b2, IntPtr.Zero, ref rect); return Rectangle.FromLTRB(rect.Left, rect.Top, rect.Right, rect.Bottom); }
    }

    public void RenderText(Graphics graphics)
    {
        // EDIT owns word breaking, including punctuation. Draw its actual
        // visual lines instead of asking a second layout engine to wrap again.
        var saved = graphics.Save();
        try {
            graphics.SetClip(FormattingRectangle);
            foreach (var line in GetVisualLines()) {
                TextRenderer.DrawText(graphics, line.Text, Font, new Point(line.X, line.Y), ForeColor,
                    TextFormatFlags.SingleLine | TextFormatFlags.ExpandTabs | TextFormatFlags.NoPrefix |
                    TextFormatFlags.NoPadding | TextFormatFlags.PreserveGraphicsClipping);
            }
        } finally { graphics.Restore(saved); }
    }

    public WinOneTextLine[] GetVisualLines()
    {
        var count = SendMessageW(Handle, 0x00ba, IntPtr.Zero, IntPtr.Zero).ToInt32();
        var text = Text;
        var lines = new List<WinOneTextLine>();
        for (var line = 0; line < count; line++) {
            var start = SendMessageW(Handle, 0x00bb, new IntPtr(line), IntPtr.Zero).ToInt32();
            if (start < 0 || start >= text.Length) continue;
            var length = SendMessageW(Handle, 0x00c1, new IntPtr(start), IntPtr.Zero).ToInt32();
            var position = SendMessageW(Handle, 0x00d6, new IntPtr(start), IntPtr.Zero).ToInt64();
            lines.Add(new WinOneTextLine(text.Substring(start, Math.Min(length, text.Length - start)), unchecked((short)position), unchecked((short)(position >> 16))));
        }
        return lines.ToArray();
    }

    public bool IsComposing => composing;
    public bool Fits => SendMessageW(Handle, 0x00ba, IntPtr.Zero, IntPtr.Zero).ToInt32() * Font.Height <= ClientSize.Height;
    public void ResizeToContent(int maximumHeight)
    {
        var lines = SendMessageW(Handle, 0x00ba, IntPtr.Zero, IntPtr.Zero).ToInt32();
        Height = Math.Min(maximumHeight, Math.Max(Font.Height, lines * Font.Height));
        if (Fits) {
            var first = SendMessageW(Handle, 0x00ce, IntPtr.Zero, IntPtr.Zero).ToInt32();
            if (first > 0) SendMessageW(Handle, 0x00b6, IntPtr.Zero, new IntPtr(-first));
        }
    }
    public void RequestCommit(Action commit, Action<Exception> failed)
    {
        if (composing) pendingCommit = () => { try { commit(); } catch (Exception error) { failed(error); } };
        else commit();
    }
    protected override void WndProc(ref Message message)
    {
        if (message.Msg == 0x010d) composing = true;
        base.WndProc(ref message);
        if (message.Msg == 0x010e) {
            composing = false;
            var commit = pendingCommit; pendingCommit = null;
            if (commit is not null && !IsDisposed) BeginInvoke(commit);
        }
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing) {
            parentPaint?.ReleaseHandle();
            // WinForms caches a separate HFONT for an explicitly assigned Font.
            // Reset the property to release that handle before disposing our
            // Font, instead of leaving each closed input's handle to finalization.
            Font = null;
            ownedFont?.Dispose();
        }
        if (backgroundBrush != IntPtr.Zero) {
            if (!DeleteObject(backgroundBrush)) Console.Error.WriteLine("Could not release inline text background brush");
            backgroundBrush = IntPtr.Zero;
        }
        base.Dispose(disposing);
    }

    private sealed class ParentPaint : NativeWindow
    {
        private readonly NativeInlineText input;
        public ParentPaint(Control parent, NativeInlineText input) { this.input = input; AssignHandle(parent.Handle); }
        protected override void WndProc(ref Message message)
        {
            if (message.Msg == 0x0133 && message.LParam == input.Handle) {
                SetTextColor(message.WParam, ColorTranslator.ToWin32(input.ForeColor));
                SetBkMode(message.WParam, 1);
                var origin = input.backgroundOrigin;
                SetBrushOrgEx(message.WParam, origin.X - input.Left, origin.Y - input.Top, IntPtr.Zero);
                message.Result = input.backgroundBrush; return;
            }
            base.WndProc(ref message);
        }
    }
}
