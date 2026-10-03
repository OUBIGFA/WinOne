using System;
using System.Drawing;
using ShareX.HelpersLib;

namespace ShareX.ScreenCaptureLib
{
    // Image coordinates remain the persistence/output contract. Only native
    // input and overlay controls use viewport coordinates after zoom and pan.
    public sealed partial class WinOneCaptureCanvas
    {
        public float ViewScale => form.ZoomFactor;
        public bool HasViewport => form.IsEditorMode;
        public PointF ViewOrigin => form.CanvasRectangle.Location;
        public Bitmap RenderTextBackdrop(Rectangle imageArea)
        {
            using var source = Render();
            var size = new Size(Math.Max(1, (int)Math.Round(imageArea.Width * ViewScale)), Math.Max(1, (int)Math.Ceiling(imageArea.Height * ViewScale)));
            var result = new Bitmap(size.Width, size.Height);
            try {
                using var graphics = Graphics.FromImage(result);
                graphics.DrawImage(source, new Rectangle(Point.Empty, size), imageArea, GraphicsUnit.Pixel);
                return result;
            } catch { result.Dispose(); throw; }
        }
        public Rectangle? ViewSelection => Selection is { } selection ? ImageToView(selection) : (Rectangle?)null;
        public Point ViewToImage(Point point) => Point.Round(new PointF(point.X / ViewScale - form.CanvasRectangle.X, point.Y / ViewScale - form.CanvasRectangle.Y));
        public Rectangle ImageToView(Rectangle rectangle) => Rectangle.Round(new RectangleF(
            (rectangle.X + form.CanvasRectangle.X) * ViewScale, (rectangle.Y + form.CanvasRectangle.Y) * ViewScale,
            rectangle.Width * ViewScale, rectangle.Height * ViewScale));
        public Rectangle ViewToImage(Rectangle rectangle) => Rectangle.Round(new RectangleF(
            rectangle.X / ViewScale - form.CanvasRectangle.X, rectangle.Y / ViewScale - form.CanvasRectangle.Y,
            rectangle.Width / ViewScale, rectangle.Height / ViewScale));
        private RectangleF ToWorld(RectangleF bounds) => new RectangleF(bounds.X + form.CanvasRectangle.X, bounds.Y + form.CanvasRectangle.Y, bounds.Width, bounds.Height);
        private PointF ToWorld(PointF point) => new PointF(point.X + form.CanvasRectangle.X, point.Y + form.CanvasRectangle.Y);
        private Rectangle FromWorld(RectangleF bounds) => Rectangle.Round(new RectangleF(bounds.X - form.CanvasRectangle.X, bounds.Y - form.CanvasRectangle.Y, bounds.Width, bounds.Height));

        public void SetViewport(float scale, PointF imageOrigin)
        {
            if (!HasViewport || !float.IsFinite(scale) || scale < 0.2f || scale > 6 ||
                !float.IsFinite(imageOrigin.X) || !float.IsFinite(imageOrigin.Y)) throw new ArgumentOutOfRangeException(nameof(scale));
            FinishGesture();
            var previous = form.CanvasRectangle;
            form.ZoomFactor = scale;
            form.CanvasRectangle = new RectangleF(imageOrigin, form.Canvas.Size);
            form.ShapeManager.MoveAll(imageOrigin.X - previous.X, imageOrigin.Y - previous.Y);
            form.CanvasCenterOffset = new Vector2((imageOrigin.X + form.Canvas.Width / 2f) * scale - form.ClientSize.Width / 2f,
                (imageOrigin.Y + form.Canvas.Height / 2f) * scale - form.ClientSize.Height / 2f);
            form.Invalidate(); Changed?.Invoke();
        }
    }
}
