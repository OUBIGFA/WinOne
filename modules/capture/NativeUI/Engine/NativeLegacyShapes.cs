using System;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Drawing.Imaging;
using ShareX.HelpersLib;

namespace ShareX.ScreenCaptureLib
{
    public interface IWinOneLegacyStroke { float LegacyStrokeWidth { get; set; } }
    public interface IWinOneLegacyEffect { void ApplyLegacy(Bitmap image); }

    internal static class WinOneLegacyDrawing
    {
        internal static void Stroke(Graphics graphics,Color color,float width,Action<Pen> draw)
        {
            var state=graphics.Save();
            try {
                graphics.PageUnit=GraphicsUnit.Pixel;graphics.SmoothingMode=SmoothingMode.AntiAlias;
                using var pen=new Pen(color,width){LineJoin=LineJoin.Round,StartCap=LineCap.Round,EndCap=LineCap.Round};
                draw(pen);
            } finally {graphics.Restore(state);}
        }
        internal static void Effect(Bitmap image,RectangleF bounds,Action<Bitmap> apply)
        {
            // Legacy bounds added width/height in double precision before ceil.
            // RectangleF.Right rounds first and can omit a boundary pixel.
            var area=Rectangle.Intersect(new Rectangle(Point.Empty,image.Size),Rectangle.FromLTRB((int)Math.Floor(bounds.Left),(int)Math.Floor(bounds.Top),(int)Math.Ceiling((double)bounds.X+bounds.Width),(int)Math.Ceiling((double)bounds.Y+bounds.Height)));
            if(area.Width<=0||area.Height<=0)return;
            using var patch=image.Clone(area,PixelFormat.Format32bppArgb);apply(patch);
            using var graphics=Graphics.FromImage(image);graphics.CompositingMode=CompositingMode.SourceCopy;graphics.DrawImageUnscaled(patch,area.Location);
        }
    }
    public sealed class WinOneLegacyRectangleShape:RectangleDrawingShape,IWinOneLegacyStroke
    {
        public float LegacyStrokeWidth{get;set;}=3;
        public override void OnDraw(Graphics graphics)=>WinOneLegacyDrawing.Stroke(graphics,BorderColor,LegacyStrokeWidth,pen=>graphics.DrawRectangle(pen,Rectangle.X,Rectangle.Y,Rectangle.Width,Rectangle.Height));
    }
    public sealed class WinOneLegacyEllipseShape:EllipseDrawingShape,IWinOneLegacyStroke
    {
        public float LegacyStrokeWidth{get;set;}=3;
        public override void OnDraw(Graphics graphics)=>WinOneLegacyDrawing.Stroke(graphics,BorderColor,LegacyStrokeWidth,pen=>graphics.DrawEllipse(pen,Rectangle));
    }
    public sealed class WinOneLegacyPenShape:FreehandDrawingShape,IWinOneLegacyStroke
    {
        public float LegacyStrokeWidth{get;set;}=3;
        public override void OnDraw(Graphics graphics){var points=WinOnePoints();if(points.Length>=2)WinOneLegacyDrawing.Stroke(graphics,BorderColor,LegacyStrokeWidth,pen=>graphics.DrawLines(pen,points));}
    }
    public sealed class WinOneLegacyArrowShape:ArrowDrawingShape,IWinOneLegacyStroke
    {
        public float LegacyStrokeWidth{get;set;}=3;
        public override void OnDraw(Graphics graphics)
        {
            if(Points.Length<2)return;
            WinOneLegacyDrawing.Stroke(graphics,BorderColor,LegacyStrokeWidth,pen=>{
                using var cap=new AdjustableArrowCap(4,5,true);pen.CustomEndCap=cap;graphics.DrawLine(pen,Points[0],Points[Points.Length-1]);
            });
        }
    }
    public sealed class WinOneLegacyPixelateShape:PixelateEffectShape,IWinOneLegacyEffect
    {
        public void ApplyLegacy(Bitmap image)=>WinOneLegacyDrawing.Effect(image,Rectangle,patch=>ImageHelpers.Pixelate(patch,PixelSize));
        public override void OnCreated(){InitialSize=Rectangle.Size;Manager.OnImageModified();}
        public override void OnMoved(){} public override void OnResized(){}
    }
    public sealed class WinOneLegacyBlurShape:BlurEffectShape,IWinOneLegacyEffect
    {
        public void ApplyLegacy(Bitmap image)=>WinOneLegacyDrawing.Effect(image,Rectangle,patch=>ImageHelpers.BoxBlur(patch,BlurRadius));
        public override void OnCreated(){InitialSize=Rectangle.Size;Manager.OnImageModified();}
        public override void OnMoved(){} public override void OnResized(){}
    }
}
