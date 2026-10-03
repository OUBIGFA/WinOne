using WinOne.Capture;

internal static class CapturedFrameTests
{
    private static void Check(bool value, string message) { if (!value) throw new Exception(message); }

    public static void Run()
    {
        var source = new Bitmap(4, 3);
        for (var y = 0; y < source.Height; y++)
            for (var x = 0; x < source.Width; x++)
                source.SetPixel(x, y, Color.FromArgb(150, x * 11, y * 13, x + y));
        var expected = source.GetPixel(1, 1);
        var lastPixel = source.GetPixel(3, 2);
        using var frame = CapturedFrame.Own(source, new Rectangle(-5, -2, 4, 3));
        using var cropped = frame.Crop(new Rectangle(-4, -1, 2, 2));
        Check(cropped.Size == new Size(2, 2) && cropped.GetPixel(0, 0).ToArgb() == expected.ToArgb(), "negative desktop coordinates changed crop pixels");
        using (var edge = frame.Crop(new Rectangle(-2, 0, 1, 1)))
            Check(edge.GetPixel(0, 0).ToArgb() == lastPixel.ToArgb(), "right/bottom edge pixel was omitted");
        foreach (var rectangle in new[] { new Rectangle(-6, -2, 2, 1), new Rectangle(-5, -2, 0, 1), new Rectangle(-2, 0, 2, 1), new Rectangle(int.MaxValue, 0, 2, 1) })
        {
            try { using var invalid = frame.Crop(rectangle); throw new Exception("out-of-bounds crop accepted"); }
            catch (ArgumentOutOfRangeException) { }
        }
        frame.Dispose();
        Check(cropped.GetPixel(0, 0).ToArgb() == expected.ToArgb(), "disposing the source invalidated an exported crop");
        try { using var invalid = frame.Crop(new Rectangle(-5, -2, 1, 1)); throw new Exception("disposed frame remained usable"); }
        catch (ObjectDisposedException) { }

        using var rejected = new Bitmap(1, 1);
        try { using var invalid = CapturedFrame.Own(rejected, new Rectangle(0, 0, 2, 2)); throw new Exception("mismatched desktop bounds accepted"); }
        catch (ArgumentException) { }
        Check(rejected.Width == 1, "failed ownership transfer disposed the producer's image");

        var full = new Bitmap(2, 2);
        full.SetPixel(1, 1, Color.Coral);
        using var owner = CapturedFrame.Own(full, new Rectangle(0, 0, 2, 2));
        using var detached = owner.Detach();
        Check(ReferenceEquals(full, detached), "full-frame handoff copied the bitmap");
        owner.Dispose();
        Check(detached.GetPixel(1, 1).ToArgb() == Color.Coral.ToArgb(), "frame disposal destroyed transferred ownership");
        try { _ = owner.Detach(); throw new Exception("frame ownership transferred twice"); }
        catch (ObjectDisposedException) { }
    }
}
