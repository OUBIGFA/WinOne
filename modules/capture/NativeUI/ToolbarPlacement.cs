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
        if (requested is null && y + size.Height > area.Bottom) y = selection.Top - size.Height - 8;
        return new Rectangle(Math.Clamp(requested?.X ?? selection.Left, area.Left, area.Right - size.Width),
            Math.Clamp(y, area.Top, area.Bottom - size.Height), size.Width, size.Height);
    }
}
