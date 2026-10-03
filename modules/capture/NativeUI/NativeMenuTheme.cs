namespace WinOne.Capture.NativeUI;

// One owned font and renderer serve a complete menu tree. Menu input, focus,
// placement and accessibility remain provided by the native controls.
internal sealed class NativeMenuTheme : IDisposable
{
    private readonly Font font = new("Segoe UI", 10, FontStyle.Regular, GraphicsUnit.Point);
    private readonly bool contrast = SystemInformation.HighContrast;
    public Color Background { get; }
    public Color Foreground { get; }
    public ToolStripRenderer Renderer { get; }
    public Font Font => font;

    public NativeMenuTheme(bool dark)
    {
        Background = contrast ? SystemColors.Menu : dark ? Color.FromArgb(23, 23, 23) : Color.White;
        Foreground = contrast ? SystemColors.MenuText : dark ? Color.FromArgb(250, 250, 250) : Color.FromArgb(23, 23, 23);
        Renderer = contrast ? new ToolStripSystemRenderer() : new MenuRenderer(new NativeMenuColors(dark: dark), Foreground) { RoundedEdges = false };
    }
    public void Apply(ToolStrip menu)
    {
        menu.Font = font; menu.BackColor = Background; menu.ForeColor = Foreground; menu.Renderer = Renderer;
        if (menu is ToolStripDropDown) {
            menu.Padding = new Padding(6);
            menu.MinimumSize = new Size(200, 0);
            menu.ImageScalingSize = new Size(18, 18);
        }
        foreach (ToolStripItem item in menu.Items) {
            item.ForeColor = Foreground; item.BackColor = Background; item.Font = font;
            if (item is ToolStripMenuItem entry) {
                entry.Padding = new Padding(8, 5, 8, 5);
                if (entry.HasDropDownItems) Apply(entry.DropDown);
            } else if (item is ToolStripControlHost host) {
                host.Control.Font = font; host.Control.BackColor = Background; host.Control.ForeColor = Foreground;
            }
        }
    }
    public void Dispose() => font.Dispose();

    private sealed class MenuRenderer(ProfessionalColorTable colors, Color foreground) : ToolStripProfessionalRenderer(colors)
    {
        protected override void OnRenderArrow(ToolStripArrowRenderEventArgs e)
        {
            e.ArrowColor = e.Item?.Enabled != false ? foreground : SystemColors.GrayText;
            base.OnRenderArrow(e);
        }
    }
}
