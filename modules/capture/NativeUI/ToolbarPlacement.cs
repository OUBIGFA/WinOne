namespace WinOne.Capture.NativeUI;

internal static class ToolbarPlacement
{
    // Coordinates are native client pixels. Clamp to the chosen monitor, not
    // the bounding rectangle spanning several monitors and their gaps.
    public static Rectangle Place(Rectangle selection, Rectangle viewport, Size preferred, Point? requested = null)
    {
        var area = Rectangle.Inflate(viewport, -8, -8);
        if (area.Width < 1 || area.Height < 1) throw new ArgumentException("Monitor area is too small");
        var size = new Size(Math.Min(preferred.Width, area.Width), Math.Min(preferred.Height, area.Height));
        var y = requested?.Y ?? selection.Bottom + 8;
        var x = requested?.X ?? selection.Left + (selection.Width - size.Width) / 2;
        // Keep the default near the lower edge. When there is no room below,
        // anchor inside the bottom-right instead of jumping to the top-left.
        if (requested is null && y + size.Height > area.Bottom) {
            y = selection.Bottom - size.Height - 8;
            x = selection.Right - size.Width - 8;
        }
        return new Rectangle(Math.Clamp(x, area.Left, area.Right - size.Width),
            Math.Clamp(y, area.Top, area.Bottom - size.Height), size.Width, size.Height);
    }
}
