namespace WinOne.Capture.NativeUI;

// One owned font and renderer serve a complete menu tree. Menu input, focus,
// placement and accessibility remain provided by the native controls.
internal sealed class NativeMenuTheme : IDisposable
{
    private readonly Font font = new("Segoe UI", 10.5f, FontStyle.Regular, GraphicsUnit.Point);
    private readonly bool contrast = SystemInformation.HighContrast;
    private readonly Dictionary<ArrowDirection, Bitmap> arrows = LoadArrows();
    public Color Background { get; }
    public Color Foreground { get; }
    public ToolStripRenderer Renderer { get; }
    public Font Font => font;

    public NativeMenuTheme(bool dark)
    {
        Background = contrast ? SystemColors.Menu : dark ? Color.FromArgb(35, 35, 35) : Color.White;
        Foreground = contrast ? SystemColors.MenuText : dark ? Color.FromArgb(233, 233, 233) : Color.FromArgb(38, 38, 38);
        Renderer = contrast ? new SystemMenuRenderer(DrawArrow) : new MenuRenderer(new NativeMenuColors(dark: dark), DrawArrow) { RoundedEdges = false };
    }
    public void Apply(ToolStrip menu)
    {
        menu.Font = font; menu.BackColor = Background; menu.ForeColor = Foreground; menu.Renderer = Renderer;
        if (menu is ToolStripDropDown) {
            menu.Padding = new Padding(6);
            menu.MinimumSize = new Size(224, 0);
            menu.ImageScalingSize = new Size(18, 18);
        }
        foreach (ToolStripItem item in menu.Items) {
            item.ForeColor = Foreground; item.BackColor = Background; item.Font = font;
            if (item is ToolStripMenuItem entry) {
                entry.Padding = new Padding(8, 7, 8, 7);
                if (entry.HasDropDownItems) Apply(entry.DropDown);
            } else if (item is ToolStripControlHost host) {
                host.Control.Font = font; host.Control.BackColor = Background; host.Control.ForeColor = Foreground;
            }
        }
    }
    public void Dispose()
    {
        foreach (var arrow in arrows.Values) arrow.Dispose();
        font.Dispose();
    }

    // Use the packaged Lucide outline at every native submenu, including
    // overflow arrows. The platform renderer's filled triangle is not reused.
    private static Dictionary<ArrowDirection, Bitmap> LoadArrows()
    {
        using var stream = typeof(NativeMenuTheme).Assembly.GetManifestResourceStream("WinOne.Capture.Icons.chevron-72.png")
            ?? throw new InvalidOperationException("Missing local menu chevron");
        using var original = Image.FromStream(stream);
        var result = new Dictionary<ArrowDirection, Bitmap>();
        foreach (var (direction, rotation) in new[] {
            (ArrowDirection.Right, RotateFlipType.RotateNoneFlipNone),
            (ArrowDirection.Down, RotateFlipType.Rotate90FlipNone),
            (ArrowDirection.Left, RotateFlipType.Rotate180FlipNone),
            (ArrowDirection.Up, RotateFlipType.Rotate270FlipNone) }) {
            var image = new Bitmap(original); image.RotateFlip(rotation); result.Add(direction, image);
        }
        return result;
    }
    // Toolbar renderers delegate here too; none may fall back to the native
    // filled triangle while the rest of the menu uses the packaged chevron.
    internal void DrawArrow(ToolStripArrowRenderEventArgs e)
    {
        var colour = e.Item?.Enabled != false ? Foreground : SystemColors.GrayText;
        var size = Math.Min(Math.Min(e.ArrowRectangle.Width, e.ArrowRectangle.Height), (int)Math.Round(16 * (e.Item?.Owner?.DeviceDpi ?? 96) / 96d));
        if (size <= 0) return;
        var rect = new Rectangle(e.ArrowRectangle.X + (e.ArrowRectangle.Width - size) / 2,
            e.ArrowRectangle.Y + (e.ArrowRectangle.Height - size) / 2, size, size);
        var image = arrows[e.Direction];
        using var attributes = new System.Drawing.Imaging.ImageAttributes();
        // Tint only the supplied alpha mask; system colours remain visible in
        // forced/high contrast and disabled states without a second icon set.
        attributes.SetColorMatrix(new System.Drawing.Imaging.ColorMatrix(new[] {
            new float[] {0,0,0,0,0}, new float[] {0,0,0,0,0}, new float[] {0,0,0,0,0},
            new float[] {0,0,0,1,0}, new float[] {colour.R/255f,colour.G/255f,colour.B/255f,0,1} }));
        e.Graphics.DrawImage(image, rect, 0, 0, image.Width, image.Height, GraphicsUnit.Pixel, attributes);
    }
    private sealed class MenuRenderer(ProfessionalColorTable colors, Action<ToolStripArrowRenderEventArgs> drawArrow) : ToolStripProfessionalRenderer(colors)
    {
        protected override void OnRenderArrow(ToolStripArrowRenderEventArgs e) => drawArrow(e);
    }
    private sealed class SystemMenuRenderer(Action<ToolStripArrowRenderEventArgs> drawArrow) : ToolStripSystemRenderer
    {
        protected override void OnRenderArrow(ToolStripArrowRenderEventArgs e) => drawArrow(e);
    }
}
