namespace WinOne.Capture.NativeUI;

internal static class NativeTextPlacement
{
    // WM_LBUTTONDOWN carries signed client coordinates in the two words.
    public static Point PointFromMessage(IntPtr value)
    {
        var packed = unchecked((long)value);
        return new Point(unchecked((short)(packed & 0xffff)), unchecked((short)((packed >> 16) & 0xffff)));
    }

    public static Rectangle NewBounds(Rectangle selection, Point point)
    {
        if (!selection.Contains(point)) throw new InvalidOperationException("请在截图选区内点击");
        var bounds = new Rectangle(point.X, point.Y, Math.Min(400, selection.Right - point.X), Math.Min(50, selection.Bottom - point.Y));
        if (bounds.Width < 32 || bounds.Height < 34) throw new InvalidOperationException("文字位置太靠近选区边缘");
        return bounds;
    }
}
