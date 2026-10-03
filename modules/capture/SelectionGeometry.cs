namespace WinOne.Capture;

[Flags]
internal enum SelectionEdge { None = 0, Left = 1, Right = 2, Top = 4, Bottom = 8, TopLeft = Left | Top, TopRight = Right | Top, BottomLeft = Left | Bottom, BottomRight = Right | Bottom }

internal static class SelectionGeometry
{
    internal static SelectionEdge HitEdge(Rectangle selection, Point point, int tolerance, Rectangle desktop)
    {
        if (selection.IsEmpty || selection == desktop) return SelectionEdge.None;
        if ((long)point.X < (long)selection.Left - tolerance || (long)point.X > (long)selection.Right + tolerance ||
            (long)point.Y < (long)selection.Top - tolerance || (long)point.Y > (long)selection.Bottom + tolerance) return SelectionEdge.None;
        var edge = SelectionEdge.None;
        // For very small selections, prefer the nearest edge instead of selecting both.
        var left = Math.Abs((long)point.X - selection.Left);
        var right = Math.Abs((long)point.X - selection.Right);
        if (Math.Min(left, right) <= tolerance) edge |= left <= right ? SelectionEdge.Left : SelectionEdge.Right;
        var top = Math.Abs((long)point.Y - selection.Top);
        var bottom = Math.Abs((long)point.Y - selection.Bottom);
        if (Math.Min(top, bottom) <= tolerance) edge |= top <= bottom ? SelectionEdge.Top : SelectionEdge.Bottom;
        return edge;
    }

    internal static Rectangle Resize(Rectangle original, SelectionEdge edge, Point point, Rectangle desktop)
    {
        // Keep the opposite edge fixed and at least one pixel selected. Clamping
        // at the opposite edge avoids reversing the handle while dragging.
        var left = original.Left; var right = original.Right;
        var top = original.Top; var bottom = original.Bottom;
        if (edge.HasFlag(SelectionEdge.Left)) left = Math.Clamp(point.X, desktop.Left, right - 1);
        if (edge.HasFlag(SelectionEdge.Right)) right = Math.Clamp(point.X, left + 1, desktop.Right);
        if (edge.HasFlag(SelectionEdge.Top)) top = Math.Clamp(point.Y, desktop.Top, bottom - 1);
        if (edge.HasFlag(SelectionEdge.Bottom)) bottom = Math.Clamp(point.Y, top + 1, desktop.Bottom);
        return Rectangle.FromLTRB(left, top, right, bottom);
    }

    internal static Rectangle WindowAtPoint(IReadOnlyList<Rectangle> windows, Point point, Rectangle bounds)
    {
        foreach (var window in windows)
        {
            var visible = Rectangle.Intersect(window, bounds);
            if (visible.Contains(point)) return visible;
        }
        return Rectangle.Empty;
    }
    internal static Rectangle ToolbarBounds(Rectangle selection, Rectangle viewport, Size preferred, int margin)
    {
        if (viewport.Width < 1 || viewport.Height < 1 || preferred.Width < 1 || preferred.Height < 1) throw new ArgumentOutOfRangeException(nameof(viewport));
        margin = Math.Clamp(margin, 0, (Math.Min(viewport.Width, viewport.Height) - 1) / 2);
        var width = Math.Min(preferred.Width, viewport.Width - margin * 2);
        var height = Math.Min(preferred.Height, viewport.Height - margin * 2);
        var x = Math.Clamp((long)selection.Right - width, (long)viewport.Left + margin, (long)viewport.Right - margin - width);
        var y = (long)selection.Bottom + margin;
        if (y + height > (long)viewport.Bottom - margin) y = (long)selection.Top - margin - height;
        y = Math.Clamp(y, (long)viewport.Top + margin, (long)viewport.Bottom - margin - height);
        return new Rectangle((int)x, (int)y, width, height);
    }
    // Inputs are pixel edges, not inclusive pixel indices. The UI can drag in
    // either direction and may leave the captured desktop while holding capture.
    public static Rectangle FromEdges(Point start, Point end, Rectangle bounds)
    {
        var right = (long)bounds.X + bounds.Width;
        var bottom = (long)bounds.Y + bounds.Height;
        if (bounds.Width <= 0 || bounds.Height <= 0 || right > int.MaxValue || bottom > int.MaxValue)
            throw new ArgumentOutOfRangeException(nameof(bounds));
        var x1 = (int)Math.Clamp((long)start.X, bounds.X, right);
        var x2 = (int)Math.Clamp((long)end.X, bounds.X, right);
        var y1 = (int)Math.Clamp((long)start.Y, bounds.Y, bottom);
        var y2 = (int)Math.Clamp((long)end.Y, bounds.Y, bottom);
        return Rectangle.FromLTRB(Math.Min(x1, x2), Math.Min(y1, y2), Math.Max(x1, x2), Math.Max(y1, y2));
    }
}
