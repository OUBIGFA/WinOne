namespace WinOne.Capture.NativeUI;

// Draws native ToolStrip controls as the settings window's MyGo controls:
// rounded hover and pressed faces, the chosen tool's fill and selection
// bar, half-opacity disabled items, and menus as kit's drop-downs. The
// controls keep their hit testing, keyboard navigation, accessibility,
// menu lifetime and screen placement.
internal sealed class NativeRenderer : ToolStripProfessionalRenderer
{
    private readonly NativeStyle style;
    private readonly Action<ToolStripArrowRenderEventArgs> drawArrow;
    private readonly Func<Image?>? overflowIcon;

    public NativeRenderer(NativeStyle style, Action<ToolStripArrowRenderEventArgs> drawArrow, Func<Image?>? overflowIcon = null)
        : base(new NativeMenuColors(style))
    {
        this.style = style; this.drawArrow = drawArrow; this.overflowIcon = overflowIcon;
        RoundedEdges = false;
    }

    private static int Scale(ToolStripItem item, int value) => NativeStyle.Scale(value, item.Owner?.DeviceDpi ?? 96);

    protected override void OnRenderToolStripBackground(ToolStripRenderEventArgs e)
    {
        using var brush = new SolidBrush(style.Panel);
        e.Graphics.FillRectangle(brush, e.AffectedBounds);
    }

    // Toolbar rows sit on the toolbar's own face; only popups have a border.
    protected override void OnRenderToolStripBorder(ToolStripRenderEventArgs e)
    {
        if (e.ToolStrip is not ToolStripDropDown) return;
        using var pen = new Pen(SystemInformation.HighContrast ? SystemColors.WindowFrame : style.Border);
        e.Graphics.DrawRectangle(pen, 0, 0, e.ToolStrip.Width - 1, e.ToolStrip.Height - 1);
    }

    // kit's drop-downs have no image column of their own color.
    protected override void OnRenderImageMargin(ToolStripRenderEventArgs e) { }

    protected override void OnRenderButtonBackground(ToolStripItemRenderEventArgs e) => Face(e.Graphics, e.Item, (e.Item as ToolStripButton)?.Checked == true);
    protected override void OnRenderDropDownButtonBackground(ToolStripItemRenderEventArgs e) => Face(e.Graphics, e.Item, false);
    protected override void OnRenderOverflowButtonBackground(ToolStripItemRenderEventArgs e)
    {
        Face(e.Graphics, e.Item, false);
        if (overflowIcon?.Invoke() is not { } image) return;
        var size = e.ToolStrip?.ImageScalingSize ?? new Size(18, 18);
        NativeStyle.DrawIcon(e.Graphics, image, new Rectangle((e.Item.Width - size.Width) / 2, (e.Item.Height - size.Height) / 2, size.Width, size.Height), e.Item.Enabled);
    }

    // kit's ghost and icon buttons: no face until hovered or pressed. The
    // chosen tool keeps its fill, with no accent line beneath it.
    private void Face(Graphics graphics, ToolStripItem item, bool chosen)
    {
        var fill = item.Pressed ? style.Pressed : chosen ? style.Selected : item.Selected && item.Enabled ? style.Hover : Color.Empty;
        if (fill.IsEmpty) return;
        if (!item.Enabled) fill = style.Faded(fill);
        var bounds = new Rectangle(Point.Empty, item.Size);
        NativeStyle.FillRounded(graphics, bounds, Scale(item, NativeStyle.Radius), fill);
    }

    // A menu item shows kit's hover face, 4 DIP inside the panel's sides,
    // while the pointer or the keys are on it.
    protected override void OnRenderMenuItemBackground(ToolStripItemRenderEventArgs e)
    {
        var item = e.Item;
        if (!item.Enabled || !(item.Selected || item.Pressed)) return;
        var inset = Scale(item, 4);
        // A drop-down trims its items' unused arrow column at its right edge.
        var right = item.Owner is { } owner ? Math.Min(item.Width, owner.Width - item.Bounds.X) : item.Width;
        NativeStyle.FillRounded(e.Graphics, new Rectangle(inset, 0, right - inset * 2, item.Height), Scale(item, NativeStyle.Radius), style.Hover);
    }

    // A menu's image column starts at its edge; kit places a row's icon 10
    // DIP inside the row's face, which leaves kit's 8 DIP before the text.
    private static Rectangle MenuImage(ToolStripItem item, Rectangle bounds) =>
        item.Owner is ToolStripDropDownMenu { ShowCheckMargin: false } ? bounds with { X = Math.Max(bounds.X, Scale(item, 14)) } : bounds;

    // Without a check column the check of an item with an image is a chosen
    // fill behind the image, as WinForms draws it; otherwise it is MyGo's
    // menu check mark.
    private static bool CheckBehindImage(ToolStripItem item) =>
        item.Image is not null && item.Owner is ToolStripDropDownMenu { ShowCheckMargin: false };

    protected override void OnRenderItemCheck(ToolStripItemImageRenderEventArgs e)
    {
        if (CheckBehindImage(e.Item)) { ChosenImage(e); return; }
        var r = e.ImageRectangle;
        var size = Math.Min(Math.Min(r.Width, r.Height), Scale(e.Item, 16));
        if (size <= 0) return;
        float x = r.X + (r.Width - size) / 2f, y = r.Y + (r.Height - size) / 2f;
        var graphics = e.Graphics; var smoothing = graphics.SmoothingMode;
        graphics.SmoothingMode = System.Drawing.Drawing2D.SmoothingMode.AntiAlias;
        try {
            using var pen = new Pen(e.Item.Enabled ? style.Text : style.Faded(style.Text), 1.5f * (e.Item.Owner?.DeviceDpi ?? 96) / 96f) {
                StartCap = System.Drawing.Drawing2D.LineCap.Round, EndCap = System.Drawing.Drawing2D.LineCap.Round, LineJoin = System.Drawing.Drawing2D.LineJoin.Round };
            graphics.DrawLines(pen, new[] { new PointF(x + size * .22f, y + size * .52f), new PointF(x + size * .42f, y + size * .71f), new PointF(x + size * .78f, y + size * .31f) });
        } finally { graphics.SmoothingMode = smoothing; }
    }

    private void ChosenImage(ToolStripItemImageRenderEventArgs e)
    {
        var inset = Scale(e.Item, 3);
        var r = Rectangle.Inflate(MenuImage(e.Item, e.ImageRectangle), inset, inset);
        NativeStyle.FillRounded(e.Graphics, r, Scale(e.Item, NativeStyle.Radius) - 2, e.Item.Enabled ? style.Selected : style.Faded(style.Selected));
    }

    protected override void OnRenderItemImage(ToolStripItemImageRenderEventArgs e)
    {
        if (e.Image is null || e.ImageRectangle.IsEmpty) return;
        if (e.Item is ToolStripMenuItem { Checked: true } && CheckBehindImage(e.Item)) ChosenImage(e);
        NativeStyle.DrawIcon(e.Graphics, e.Image, MenuImage(e.Item!, e.ImageRectangle), e.Item?.Enabled != false);
    }

    // Labels heading a menu's groups and shortcut keys are secondary text, as
    // in kit and MyGo's menus; disabled text fades like the rest of the item
    // rather than turning to the system's gray.
    protected override void OnRenderItemText(ToolStripItemTextRenderEventArgs e)
    {
        var item = e.Item;
        var secondary = item is ToolStripLabel && item.Owner is ToolStripDropDown || item is ToolStripMenuItem && e.Text != item.Text;
        var color = SystemInformation.HighContrast ? (item.Enabled ? e.TextColor : SystemColors.GrayText)
            : !item.Enabled ? style.Faded(style.Text)
            : secondary ? style.TextMuted : style.Text;
        TextRenderer.DrawText(e.Graphics, e.Text, e.TextFont, e.TextRectangle, color, e.TextFormat);
    }

    // Group dividers: kit's border color, inset from the ends.
    protected override void OnRenderSeparator(ToolStripSeparatorRenderEventArgs e)
    {
        var item = e.Item;
        using var pen = new Pen(SystemInformation.HighContrast ? SystemColors.MenuText : style.Border);
        if (e.Vertical) {
            var x = item.Width / 2; var inset = Scale(item, 2);
            e.Graphics.DrawLine(pen, x, inset, x, item.Height - inset - 1);
        } else {
            var y = item.Height / 2; var inset = Scale(item, 8);
            var right = item.Owner is { } owner ? Math.Min(item.Width, owner.Width - item.Bounds.X) : item.Width;
            e.Graphics.DrawLine(pen, inset, y, right - inset - 1, y);
        }
    }

    protected override void OnRenderArrow(ToolStripArrowRenderEventArgs e) => drawArrow(e);
}
