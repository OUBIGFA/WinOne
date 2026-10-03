using System.Drawing.Drawing2D;
using ShareX.HelpersLib;

internal static class BackgroundRenderingTests
{
    public static void Run()
    {
        using var source = new Bitmap(96, 64);
        for (var y = 0; y < source.Height; y++) for (var x = 0; x < source.Width; x++) source.SetPixel(x, y, Color.FromArgb(x * 2, y * 3, (x + y) % 256));
        foreach (var scale in new[] { 1f, 1.5f, 0.75f }) foreach (var origin in new[] { Point.Empty, new Point(13, 7), new Point(-17, -9) }) {
            using var brush = new TextureBrush(source) { WrapMode = WrapMode.Clamp };
            brush.TranslateTransform(origin.X, origin.Y);
            using var old = new Bitmap(180, 120); using var direct = new Bitmap(180, 120);
            using (var graphics = Graphics.FromImage(old)) {
                graphics.ScaleTransform(scale, scale);
                using var quality = new GraphicsQualityManager(graphics, false);
                graphics.CompositingMode = CompositingMode.SourceCopy;
                using var extracted = brush.Image;
                graphics.DrawImage(extracted, new Rectangle(origin, source.Size));
            }
            using (var graphics = Graphics.FromImage(direct)) {
                graphics.ScaleTransform(scale, scale);
                using var quality = new GraphicsQualityManager(graphics, false);
                graphics.CompositingMode = CompositingMode.SourceCopy;
                using var owned = (Bitmap)source.Clone();
                graphics.DrawImage(owned, new Rectangle(origin, source.Size));
            }
            for (var y = 0; y < old.Height; y++) for (var x = 0; x < old.Width; x++)
                if (old.GetPixel(x, y) != direct.GetPixel(x, y)) throw new Exception($"Background rendering differs at scale {scale}, origin {origin}, pixel {x},{y}");
        }
    }
}
