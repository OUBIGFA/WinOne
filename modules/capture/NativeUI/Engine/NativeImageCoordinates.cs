using System;
using System.Drawing;
using System.Linq;

namespace ShareX.ScreenCaptureLib
{
    // History copies these image coordinates together with each editable shape.
    // Translating a canvas must not quantize old fractional positions through
    // large screen coordinates before exporting or applying an effect.
    public sealed record WinOneShapeCoordinates(RectangleF ImageBounds, RectangleF WorldBounds,
        PointF[] ImagePoints, PointF[] WorldPoints);

    public abstract partial class BaseShape
    {
        public WinOneShapeCoordinates WinOneCoordinates { get; set; }
    }

    internal partial class ShapeManager
    {
        private static PointF[] WinOneShapePoints(BaseShape shape) => shape switch {
            FreehandDrawingShape pen => pen.WinOnePoints(),
            ArrowDrawingShape arrow => arrow.Points.ToArray(),
            _ => Array.Empty<PointF>()
        };
        private static (RectangleF Bounds, PointF[] Points) WinOneImageCoordinates(BaseShape shape, PointF origin, PointF[] points)
        {
            var bounds = shape.Rectangle;
            var saved = shape.WinOneCoordinates;
            if (saved is null) return (new RectangleF(bounds.X-origin.X,bounds.Y-origin.Y,bounds.Width,bounds.Height),
                points.Select(p=>new PointF(p.X-origin.X,p.Y-origin.Y)).ToArray());
            // Actual user movement is a delta from the remembered world position;
            // viewport translation refreshes that position without changing pixels.
            var local = new RectangleF((float)(saved.ImageBounds.X+(double)bounds.X-saved.WorldBounds.X),
                (float)(saved.ImageBounds.Y+(double)bounds.Y-saved.WorldBounds.Y), bounds.Width, bounds.Height);
            var localPoints = points.Length == saved.WorldPoints.Length
                ? points.Select((p,i)=>new PointF((float)(saved.ImagePoints[i].X+(double)p.X-saved.WorldPoints[i].X),
                    (float)(saved.ImagePoints[i].Y+(double)p.Y-saved.WorldPoints[i].Y))).ToArray()
                : points.Select(p=>new PointF(p.X-origin.X,p.Y-origin.Y)).ToArray();
            return (local,localPoints);
        }
        internal void TranslateWinOneCanvas(float x, float y)
        {
            var origin = new PointF(Form.CanvasRectangle.X-x,Form.CanvasRectangle.Y-y);
            foreach (var shape in Shapes) {
                if (shape.ShapeCategory == ShapeCategory.Region) { shape.Move(x,y); continue; }
                var local = WinOneImageCoordinates(shape,origin,WinOneShapePoints(shape));
                shape.Move(x,y);
                shape.WinOneCoordinates = new(local.Bounds,shape.Rectangle,local.Points,WinOneShapePoints(shape));
            }
        }
    }
}
