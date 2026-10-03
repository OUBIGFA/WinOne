using System;
using System.Drawing;
using System.Runtime.InteropServices;
using System.Windows.Forms;

namespace ShareX.ScreenCaptureLib
{
    public sealed record WinOneTextLine(string Text, int X, int Y);

    // A real bitmap-backed HDC lets native text blend with the actual image.
    // Keep effects in the upstream bitmap pass, then draw all drawing layers
    // in their original order on this surface, rather than moving text on top.
    internal static class NativeDrawingSurface
    {
        [DllImport("gdi32.dll")] private static extern IntPtr CreateCompatibleDC(IntPtr dc);
        [DllImport("gdi32.dll")] private static extern IntPtr SelectObject(IntPtr dc, IntPtr value);
        [DllImport("gdi32.dll")] private static extern bool DeleteDC(IntPtr dc);
        [DllImport("gdi32.dll")] private static extern bool DeleteObject(IntPtr value);
        internal static Bitmap Draw(Bitmap background, Action<Graphics> draw)
        {
            var bitmap = background.GetHbitmap();
            var dc = CreateCompatibleDC(IntPtr.Zero);
            if (dc == IntPtr.Zero) { DeleteObject(bitmap); throw new InvalidOperationException("Could not create drawing surface"); }
            var previous = SelectObject(dc, bitmap);
            try {
                using (var graphics = Graphics.FromHdc(dc)) draw(graphics);
                return Image.FromHbitmap(bitmap);
            } finally { SelectObject(dc, previous); DeleteDC(dc); DeleteObject(bitmap); }
        }
        internal static void DrawText(Graphics graphics, WinOneTextLine[] lines, Rectangle bounds, float fontPixels, Color color)
        {
            var saved = graphics.Save();
            try {
                // GDI TextRenderer does not apply Graphics' world transform.
                // Convert explicitly so zoomed canvas text matches native EDIT.
                using var transform = graphics.Transform;
                var corners = new[] { new PointF(bounds.Left, bounds.Top), new PointF(bounds.Right, bounds.Bottom) };
                transform.TransformPoints(corners);
                var scale = transform.Elements[0];
                graphics.ResetTransform();
                graphics.SetClip(RectangleF.FromLTRB(corners[0].X, corners[0].Y, corners[1].X, corners[1].Y), System.Drawing.Drawing2D.CombineMode.Intersect);
                using var font = new Font("Segoe UI", fontPixels * scale, FontStyle.Regular, GraphicsUnit.Pixel);
                foreach (var line in lines) {
                    var position = new[] { new PointF(bounds.X + line.X, bounds.Y + line.Y) };
                    transform.TransformPoints(position);
                    TextRenderer.DrawText(graphics, line.Text, font, Point.Round(position[0]), color,
                        TextFormatFlags.SingleLine | TextFormatFlags.ExpandTabs | TextFormatFlags.NoPrefix |
                        TextFormatFlags.NoPadding | TextFormatFlags.PreserveGraphicsClipping);
                }
            } finally { graphics.Restore(saved); }
        }
    }
}
