namespace WinOne.Capture;

// Physical desktop pixels from the same immutable frame as the captured image.
// This is local session data; it never belongs in portable preferences.
internal record CapturePlacement(int X, int Y, int DesktopX, int DesktopY, int DesktopWidth, int DesktopHeight)
{
    public void Validate(Size image)
    {
        var right = (long)DesktopX + DesktopWidth;
        var bottom = (long)DesktopY + DesktopHeight;
        if (DesktopWidth <= 0 || DesktopHeight <= 0 || right > int.MaxValue || bottom > int.MaxValue ||
            image.Width <= 0 || image.Height <= 0 || X < DesktopX || Y < DesktopY ||
            (long)X + image.Width > right || (long)Y + image.Height > bottom)
            throw new InvalidDataException("截图位置与桌面范围不一致");
    }

    public static CapturePlacement From(Rectangle selected, Rectangle desktop)
    {
        var placement = new CapturePlacement(selected.X, selected.Y, desktop.X, desktop.Y, desktop.Width, desktop.Height);
        placement.Validate(selected.Size);
        return placement;
    }
}
