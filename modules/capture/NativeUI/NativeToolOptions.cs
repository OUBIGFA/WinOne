using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

// A native menu hosts standard controls. It stays attached to the existing
// toolbar and exposes only properties used by the active drawing tool.
internal sealed class NativeToolOptions : ToolStripDropDownButton
{
    private readonly ToolStripLabel valueLabel = new();
    private readonly NumericUpDown value = new() { AutoSize = false, Size = new Size(160, 28), BorderStyle = System.Windows.Forms.BorderStyle.FixedSingle, DecimalPlaces = 0, Minimum = 1, Maximum = 20 };
    private readonly ToolStripControlHost valueHost;
    private readonly List<ToolStripMenuItem> colors = new();
    private readonly List<Image> swatches = new();
    private readonly bool english;
    private bool updating;
    private WinOneToolStyle style = new(Color.Red, 2);
    public event Action<WinOneToolStyle>? StyleChanged;

    public NativeToolOptions(bool dark, string? language)
    {
        english = (language ?? System.Globalization.CultureInfo.CurrentUICulture.Name).StartsWith("en", StringComparison.OrdinalIgnoreCase);
        Text = T("样式", "Style"); AccessibleName = Text;
        Name = "capture-style"; Overflow = ToolStripItemOverflow.Never; Enabled = false;
        DisplayStyle = ToolStripItemDisplayStyle.Text;
        Padding = new Padding(8, 4, 8, 4);
        value.AccessibleName = T("线宽", "Stroke width");
        value.BackColor = dark ? Color.FromArgb(23, 23, 23) : Color.White;
        value.ForeColor = dark ? Color.White : Color.FromArgb(23, 23, 23);
        DropDown = new ToolStripDropDownMenu { ShowImageMargin = true, ShowCheckMargin = true, Padding = new Padding(8), MinimumSize = new Size(200, 0) };
        valueHost = new ToolStripControlHost(value) { AutoSize = false, Size = new Size(176, 40), Padding = new Padding(8, 4, 8, 8) };
        DropDownItems.Add(new ToolStripLabel(T("颜色", "Color")));
        foreach (var entry in new[] {
            ("红色", "Red", Color.FromArgb(226,59,48)), ("黄色", "Yellow", Color.Gold),
            ("绿色", "Green", Color.ForestGreen), ("蓝色", "Blue", Color.RoyalBlue),
            ("黑色", "Black", Color.Black), ("白色", "White", Color.White) }) {
            // A bordered swatch remains visible for white and black, while the
            // name and native check mark identify it without relying on color.
            var swatch = new Bitmap(18, 18);
            using (var graphics = Graphics.FromImage(swatch)) {
                graphics.Clear(entry.Item3);
                using var border = new Pen(SystemColors.GrayText);
                graphics.DrawRectangle(border, 0, 0, 17, 17);
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
}
