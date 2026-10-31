using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// A native menu hosts standard controls. It stays attached to the existing
// toolbar and exposes only properties used by the active drawing tool.
internal sealed class NativeToolOptions : ToolStripDropDownButton
{
    private readonly ToolStripLabel valueLabel = new();
    private readonly NumericUpDown value = new() { AutoSize = false, BorderStyle = System.Windows.Forms.BorderStyle.None, DecimalPlaces = 0, Minimum = 1, Maximum = 20 };
    private readonly ToolStripControlHost valueHost;
    private readonly List<ToolStripMenuItem> colors = new();
    private readonly List<Image> swatches = new();
    private readonly bool english;
    private readonly NativeStyle palette;
    private bool updating;
    private WinOneToolStyle style = new(Color.Red, 2);
    public event Action<WinOneToolStyle>? StyleChanged;

    public NativeToolOptions(bool dark, string? language)
    {
        english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        Text = T("样式", "Style"); AccessibleName = Text;
        Name = "capture-style"; Overflow = ToolStripItemOverflow.Never; Enabled = false;
        // kit's select: the label, then a small arrow, both drawn in OnPaint
        // at kit's spacing. The toolbar sizes it like the icon buttons.
        DisplayStyle = ToolStripItemDisplayStyle.None;
        ShowDropDownArrow = false; AutoSize = false; AutoToolTip = false;
        value.AccessibleName = T("线宽", "Stroke width");
        palette = NativeStyle.For(dark);
        value.BackColor = SystemInformation.HighContrast ? SystemColors.Window : palette.Surface;
        value.ForeColor = SystemInformation.HighContrast ? SystemColors.WindowText : palette.Text;
        DropDown = new ToolStripDropDownMenu { ShowImageMargin = true, ShowCheckMargin = true };
        valueHost = new ToolStripControlHost(new NumberBox(value, palette)) { AutoSize = false, Size = new Size(176, 44), Padding = new Padding(8, 4, 8, 8) };
        DropDownItems.Add(new ToolStripLabel(T("颜色", "Color")));
        foreach (var entry in new[] {
            ("红色", "Red", Color.FromArgb(226,59,48)), ("黄色", "Yellow", Color.Gold),
            ("绿色", "Green", Color.ForestGreen), ("蓝色", "Blue", Color.RoyalBlue),
            ("黑色", "Black", Color.Black), ("白色", "White", Color.White) }) {
            // A bordered swatch remains visible for white and black, while the
            // name and native check mark identify it without relying on color.
            // Its corners follow kit's small radius.
            var swatch = new Bitmap(18, 18);
            using (var graphics = Graphics.FromImage(swatch))
            using (var shape = NativeStyle.Rounded(new RectangleF(1, 1, 16, 16), 4))
            using (var fill = new SolidBrush(entry.Item3))
            using (var border = new Pen(SystemInformation.HighContrast ? SystemColors.GrayText : palette.Border)) {
                graphics.SmoothingMode = System.Drawing.Drawing2D.SmoothingMode.AntiAlias;
                graphics.PixelOffsetMode = System.Drawing.Drawing2D.PixelOffsetMode.Half;
                graphics.FillPath(fill, shape);
                graphics.DrawPath(border, shape);
            }
            swatches.Add(swatch);
            var item = new ToolStripMenuItem(T(entry.Item1, entry.Item2), swatch) { Tag = entry.Item3 };
            item.Click += (_, _) => StyleChanged?.Invoke(style with { Color = (Color)item.Tag! });
            colors.Add(item); DropDownItems.Add(item);
        }
        DropDownItems.Add(new ToolStripSeparator()); DropDownItems.Add(valueLabel); DropDownItems.Add(valueHost);
        value.ValueChanged += (_, _) => { if (!updating) StyleChanged?.Invoke(style with { Value = (int)Math.Round(value.Value), Precision=value.DecimalPlaces>0?(float)value.Value:null }); };
    }
    private string T(string chinese, string translated) => english ? translated : chinese;

    // The arrow after the label, at the toolbar's DPI. The toolbar owns and
    // disposes it.
    private Image? arrow;
    internal Image? ReplaceArrow(Image? next) { var previous = arrow; arrow = next; Invalidate(); return previous; }

    private const TextFormatFlags LabelFormat = TextFormatFlags.Left | TextFormatFlags.VerticalCenter | TextFormatFlags.SingleLine | TextFormatFlags.NoPadding | TextFormatFlags.NoPrefix;
    private int LabelWidth => TextRenderer.MeasureText(Text, Font, Size.Empty, LabelFormat).Width;

    // kit's select spacing: 10 DIP before the label, 6 to the arrow, 8 after.
    internal int PreferredWidth(int dpi) => NativeStyle.Scale(10 + 6 + 8, dpi) + LabelWidth + (arrow?.Width ?? 0);

    protected override void OnPaint(PaintEventArgs e)
    {
        // The renderer draws the face; the label and arrow follow it.
        base.OnPaint(e);
        var dpi = Owner?.DeviceDpi ?? 96;
        var left = NativeStyle.Scale(10, dpi); var width = LabelWidth;
        var color = SystemInformation.HighContrast ? (Enabled ? ForeColor : SystemColors.GrayText) : Enabled ? palette.Text : palette.Faded(palette.Text);
        TextRenderer.DrawText(e.Graphics, Text, Font, new Rectangle(left, 0, width, Height), color, LabelFormat);
        if (arrow is not null)
            NativeStyle.DrawIcon(e.Graphics, arrow, new Rectangle(left + width + NativeStyle.Scale(6, dpi), (Height - arrow.Height) / 2, arrow.Width, arrow.Height), Enabled);
    }

    public void Configure(string tool, WinOneToolStyle current, bool editing)
    {
        var supported = tool is "rectangle" or "ellipse" or "arrow" or "pen" or "cover" or "mosaic" or "blur" or "text";
        // The same button stays in the row for every tool. Switching selection
        // to a drawing tool must not change the toolbar's width or placement.
        Enabled = supported && !editing;
        // Visibility also depends on the parent window. Configure values before
        // first paint, even while the toolbar is still hidden.
        if (!supported) return;
        style = current; updating = true;
        try {
            var effect = tool is "mosaic" or "blur";
            foreach (var item in colors) item.Enabled = !effect;
            valueLabel.Enabled = valueHost.Enabled = tool != "cover";
            valueLabel.Text = tool == "text" ? T("字号", "Font size") : tool == "mosaic" ? T("像素大小", "Pixel size") : tool == "blur" ? T("模糊强度", "Blur strength") : T("线宽", "Stroke width");
            value.AccessibleName = valueLabel.Text;
            value.DecimalPlaces = tool=="text"||current.Precision is not null?2:0;
            value.Minimum = tool == "text" ? 6 : tool == "mosaic" ? 2 : tool == "blur" ? 2 : 1;
            value.Maximum = tool == "text" ? 256 : tool == "mosaic" ? 10000 : tool == "blur" ? 199 : 64;
            value.Increment = tool == "blur" ? 2 : 1;
            value.Value = Math.Clamp((decimal)current.EffectiveValue,value.Minimum,value.Maximum);
            foreach (var item in colors) item.Checked = ((Color)item.Tag!).ToArgb() == current.Color.ToArgb();
        } finally { updating = false; }
    }
    protected override void Dispose(bool disposing)
    {
        base.Dispose(disposing);
        if (disposing) { foreach (var image in swatches) image.Dispose(); swatches.Clear(); }
    }

    // kit's number field: one rounded field without spin buttons, as in the
    // settings window. Typing sets the value; Up, Down and the mouse wheel
    // still step it.
    private sealed class NumberBox : Control
    {
        private readonly NumericUpDown value;
        private readonly NativeStyle palette;

        public NumberBox(NumericUpDown value, NativeStyle palette)
        {
            this.value = value; this.palette = palette;
            SetStyle(ControlStyles.AllPaintingInWmPaint | ControlStyles.OptimizedDoubleBuffer | ControlStyles.ResizeRedraw | ControlStyles.UserPaint, true);
            foreach (var buttons in value.Controls.Cast<Control>().Where(child => child is not TextBoxBase).ToArray()) buttons.Visible = false;
            value.Enter += (_, _) => Invalidate();
            value.Leave += (_, _) => Invalidate();
            Controls.Add(value);
        }

        protected override void OnLayout(LayoutEventArgs e)
        {
            base.OnLayout(e);
            // The hidden spin buttons keep their column at the right, which
            // serves as the field's right padding.
            var inset = NativeStyle.Scale(10, DeviceDpi);
            var height = value.PreferredHeight;
            value.SetBounds(inset, (Height - height) / 2, Math.Max(1, Width - inset - 2), height);
        }

        protected override void OnEnabledChanged(EventArgs e) { base.OnEnabledChanged(e); Invalidate(); }

        protected override void OnPaint(PaintEventArgs e)
        {
            var contrast = SystemInformation.HighContrast;
            var edge = value.ContainsFocus ? (contrast ? SystemColors.Highlight : palette.Accent) : (contrast ? SystemColors.WindowFrame : palette.Border);
            if (!Enabled && !contrast) edge = palette.Faded(edge);
            var radius = NativeStyle.Scale(NativeStyle.Radius, DeviceDpi);
            e.Graphics.Clear(BackColor);
            NativeStyle.FillRounded(e.Graphics, new Rectangle(0, 0, Width, Height), radius, edge);
            NativeStyle.FillRounded(e.Graphics, new Rectangle(1, 1, Width - 2, Height - 2), radius - 1, value.BackColor);
        }
    }
}
