using System;
using System.Drawing;

namespace ShareX.ScreenCaptureLib
{
    public sealed partial class WinOneCaptureCanvas
    {
        // The session owns this image. Previewing never replaces the original
        // canvas, crop, annotations or undo history.
        public void SetTranslationPreview(Bitmap image, Rectangle region)
        {
            if (image != null && (Selection != region || image.Size != region.Size))
                throw new InvalidOperationException("译图与当前选区不匹配");
            form.WinOneTranslationPreview = image;
            form.WinOneTranslationRegion = region;
            form.Invalidate();
        }

        public Bitmap RenderOriginalSelection()
        {
            var region = Selection ?? throw new InvalidOperationException("请先选择截图区域");
            using var rendered = Render();
            region = Rectangle.Intersect(region, new Rectangle(Point.Empty, rendered.Size));
            if (region.Width < 1 || region.Height < 1) throw new InvalidOperationException("Invalid selection");
            return rendered.Clone(region, rendered.PixelFormat);
        }
    }

    public sealed partial class RegionCaptureForm
    {
        internal Bitmap WinOneTranslationPreview;
        internal Rectangle WinOneTranslationRegion;

        private void DrawWinOneTranslation(Graphics graphics)
        {
            if (WinOneTranslationPreview == null) return;
            var bounds = new RectangleF(CanvasRectangle.X + WinOneTranslationRegion.X,
                CanvasRectangle.Y + WinOneTranslationRegion.Y,
                WinOneTranslationRegion.Width, WinOneTranslationRegion.Height);
            // OnPaint already applies the viewport transform. Keep native image
            // coordinates here so long captures also track zoom and panning.
            graphics.DrawImage(WinOneTranslationPreview, bounds);
            graphics.DrawRectangle(borderPen, bounds.X, bounds.Y, bounds.Width, bounds.Height);
        }
    }
}
