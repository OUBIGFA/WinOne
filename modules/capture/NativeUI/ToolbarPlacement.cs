namespace WinOne.Capture.NativeUI;

internal static class ToolbarPlacement
{
    // Recording must prefer every available side of the selection. Do not
    // change the annotation toolbar's deliberately inside-bottom fallback.
    // On a nearly full monitor there may be no rectangle large enough; choose
    // the least overlapping visible position, then retry after controls shrink.
    public static Rectangle PlaceRecording(Rectangle selection, IReadOnlyList<Rectangle> viewports, Size preferred)
    {
        if (viewports.Count == 0) throw new ArgumentException("No recording monitor is available");
        if (selection.Contains(viewports[0])) return Place(selection, viewports[0], preferred);
        Rectangle? best = null; long overlap = long.MaxValue;
        foreach (var viewport in viewports) {
            var area = Rectangle.Inflate(viewport, -8, -8);
            if (area.Width < preferred.Width || area.Height < preferred.Height) continue;
            var x = Math.Clamp(selection.Left + (selection.Width - preferred.Width) / 2, area.Left, area.Right - preferred.Width);
            var y = Math.Clamp(selection.Top + (selection.Height - preferred.Height) / 2, area.Top, area.Bottom - preferred.Height);
            foreach (var point in new[] {
                new Point(x, selection.Bottom + 8), new Point(x, selection.Top - preferred.Height - 8),
                new Point(selection.Right + 8, y), new Point(selection.Left - preferred.Width - 8, y),
            }) {
                var box = new Rectangle(new Point(Math.Clamp(point.X, area.Left, area.Right - preferred.Width), Math.Clamp(point.Y, area.Top, area.Bottom - preferred.Height)), preferred);
                if (!box.IntersectsWith(selection)) return box;
                var intersection = Rectangle.Intersect(box, selection);
                var pixels = (long)intersection.Width * intersection.Height;
                if (pixels < overlap) { best = box; overlap = pixels; }
            }
        }
        return best ?? Place(selection, viewports[0], preferred);
    }

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
