using System.Drawing.Drawing2D;
using System.Drawing.Imaging;
using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;

namespace WinOne.Capture.NativeUI;

// The desktop window's design tokens (modules/nativeui/design/theme.go). The
// capture toolbar and its menus take the same neutral palette, radii and
// font as the MyGo settings window, so capturing reads as the same product.
// Change both places together.
internal sealed class NativeStyle
{
    public static readonly NativeStyle Light = new(panel: 0xffffff, surface: 0xf0f0ee, border: 0xcececa, text: 0x292927, muted: 0x60605c,
        hover: 0xe8e8e5, pressed: 0xdededa, selected: 0xe4e4e0, accent: 0x30302e);
    public static readonly NativeStyle Dark = new(panel: 0x242423, surface: 0x292928, border: 0x454541, text: 0xe7e7e2, muted: 0xaaa9a3,
        hover: 0x333331, pressed: 0x3b3b38, selected: 0x343432, accent: 0xe4e4e0);
    public static NativeStyle For(bool dark) => dark ? Dark : Light;

    // kit's Radius (controls, menu items) and PanelRadius (panels, popups), in DIP.
    public const int Radius = 8, PanelRadius = 10;

    // Panel is the face of the toolbar and its menus (kit Palette.Panel).
    public Color Panel { get; }
    // Surface fills fields, as kit's number field.
    public Color Surface { get; }
    public Color Border { get; }
    public Color Text { get; }
    public Color TextMuted { get; }
    public Color Hover { get; }
    public Color Pressed { get; }
    // Selected fills the chosen tool, as kit fills the chosen page.
    public Color Selected { get; }
    // Accent marks the chosen tool and the focused field's border.
    public Color Accent { get; }

    private NativeStyle(int panel, int surface, int border, int text, int muted, int hover, int pressed, int selected, int accent)
    {
        Panel = Rgb(panel); Surface = Rgb(surface); Border = Rgb(border); Text = Rgb(text); TextMuted = Rgb(muted);
        Hover = Rgb(hover); Pressed = Rgb(pressed); Selected = Rgb(selected); Accent = Rgb(accent);
    }
    private static Color Rgb(int value) => Color.FromArgb(255, Color.FromArgb(value));

    // MyGo paints a disabled element at half opacity; on the opaque face that
    // is the color halfway to the panel.
    public Color Faded(Color color) => Color.FromArgb((color.R + Panel.R) / 2, (color.G + Panel.G) / 2, (color.B + Panel.B) / 2);

    // Match the shared UI font at 14 px with explicit Chinese coverage.
    // GDI+ silently substitutes a missing family, so check the selected name.
    public static Font CreateFont()
    {
        var font = new Font("Microsoft YaHei UI", 10.5f, FontStyle.Regular, GraphicsUnit.Point);
        if (font.Name == "Microsoft YaHei UI") return font;
        font.Dispose();
        return new Font("Segoe UI", 10.5f, FontStyle.Regular, GraphicsUnit.Point);
    }

    public static int Scale(int value, int dpi) => (int)Math.Round(value * dpi / 96d);

    public static GraphicsPath Rounded(RectangleF bounds, float radius)
    {
        var path = new GraphicsPath();
        var diameter = Math.Min(radius * 2, Math.Min(bounds.Width, bounds.Height));
        if (diameter <= 0) { path.AddRectangle(bounds); return path; }
        path.AddArc(bounds.Left, bounds.Top, diameter, diameter, 180, 90);
        path.AddArc(bounds.Right - diameter, bounds.Top, diameter, diameter, 270, 90);
        path.AddArc(bounds.Right - diameter, bounds.Bottom - diameter, diameter, diameter, 0, 90);
        path.AddArc(bounds.Left, bounds.Bottom - diameter, diameter, diameter, 90, 90);
        path.CloseFigure();
        return path;
    }

    // Fills a rounded rectangle with smooth corners and straight edges on
    // whole pixels, as MyGo draws a control's face.
    public static void FillRounded(Graphics graphics, RectangleF bounds, float radius, Color color)
    {
        var smoothing = graphics.SmoothingMode; var offset = graphics.PixelOffsetMode;
        graphics.SmoothingMode = SmoothingMode.AntiAlias; graphics.PixelOffsetMode = PixelOffsetMode.Half;
        try {
            using var path = Rounded(bounds, radius);
            using var brush = new SolidBrush(color);
            graphics.FillPath(brush, path);
        } finally { graphics.SmoothingMode = smoothing; graphics.PixelOffsetMode = offset; }
    }

    // Packaged Lucide masks take the theme's text colors, as MyGo colors its
    // icons; only the alpha of the mask is kept.
    public static Bitmap Tint(Image mask, Color color)
    {
        var image = new Bitmap(mask.Width, mask.Height, PixelFormat.Format32bppArgb);
        using (var graphics = Graphics.FromImage(image)) {
            graphics.CompositingMode = CompositingMode.SourceCopy;
            graphics.DrawImage(mask, new Rectangle(0, 0, mask.Width, mask.Height));
        }
        var data = image.LockBits(new Rectangle(0, 0, image.Width, image.Height), ImageLockMode.ReadWrite, PixelFormat.Format32bppArgb);
        try {
            var pixels = new int[data.Stride / 4 * image.Height];
            Marshal.Copy(data.Scan0, pixels, 0, pixels.Length);
            var rgb = color.ToArgb() & 0xffffff;
            for (var i = 0; i < pixels.Length; i++) pixels[i] = pixels[i] & unchecked((int)0xff000000) | rgb;
            Marshal.Copy(pixels, 0, data.Scan0, pixels.Length);
        } finally { image.UnlockBits(data); }
        return image;
    }

    // Draws an icon, at half opacity while its item is disabled.
    public static void DrawIcon(Graphics graphics, Image image, Rectangle bounds, bool enabled)
    {
        if (enabled) { graphics.DrawImage(image, bounds); return; }
        using var attributes = new ImageAttributes();
        attributes.SetColorMatrix(new ColorMatrix { Matrix33 = 0.5f });
        graphics.DrawImage(image, bounds, 0, 0, image.Width, image.Height, GraphicsUnit.Pixel, attributes);
    }

    [DllImport("dwmapi.dll")] private static extern int DwmSetWindowAttribute(IntPtr window, int attribute, ref int value, int size);
    private static readonly ConditionalWeakTable<ToolStripDropDown, NativeStyle> roundedMenus = new();

    // Windows 11 rounds a popup that asks, as kit rounds its drop-downs, and
    // draws the popup's 1 px border over its edge in the color given. The
    // renderer still draws its own square border underneath: Windows 10 and
    // virtual machines do not round, and keep that border.
    public void Round(ToolStripDropDown menu)
    {
        if (roundedMenus.TryGetValue(menu, out _)) return;
        roundedMenus.Add(menu, this);
        void Apply()
        {
            var corner = 2; // DWMWCP_ROUND
            DwmSetWindowAttribute(menu.Handle, 33, ref corner, sizeof(int));
            var border = ColorTranslator.ToWin32(Border);
            DwmSetWindowAttribute(menu.Handle, 34, ref border, sizeof(int));
        }
        if (menu.IsHandleCreated) Apply();
        menu.HandleCreated += (_, _) => Apply();
    }
}
