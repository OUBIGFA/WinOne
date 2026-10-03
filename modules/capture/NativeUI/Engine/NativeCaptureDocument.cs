using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using ShareX.HelpersLib;

namespace ShareX.ScreenCaptureLib
{
    // This is a bounded data format, not serialized engine objects or type names.
    public sealed record WinOneCaptureDocument(int Version, int Width, int Height, Rectangle? Selection, string Tool, WinOneCaptureLayer[] Layers)
    {
        public void Validate(Size image)
        {
            if (Version != 1 || Width != image.Width || Height != image.Height || Width < 1 || Height < 1 || Layers == null || Layers.Length > 512)
                throw new InvalidDataException("原生截图文档尺寸或版本无效");
            if (Tool is not ("region" or "select" or "rectangle" or "ellipse" or "arrow" or "pen" or "cover" or "mosaic" or "blur")) throw new InvalidDataException("原生截图工具无效");
            if (Selection is { } crop && (crop.X < 0 || crop.Y < 0 || crop.Width < 1 || crop.Height < 1 || (long)crop.X + crop.Width > Width || (long)crop.Y + crop.Height > Height))
                throw new InvalidDataException("原生截图选区无效");
            var points = 0; var ids = new HashSet<Guid>();
            foreach (var layer in Layers) {
                if (layer == null || layer.Kind is not ("rectangle" or "ellipse" or "arrow" or "pen" or "cover" or "mosaic" or "blur" or "text")) throw new InvalidDataException("原生截图图层无效");
                var bounds = layer.Bounds;
                // The engine allows layers to move outside the image. Preserve
                // those edits while bounding coordinates before GDI conversion;
                // output is clipped to the strictly validated image crop.
                if (!Finite(bounds.X, bounds.Y, bounds.Width, bounds.Height) || bounds.Width < 0 || bounds.Height < 0 || !Coordinate(bounds.X) || !Coordinate(bounds.Y) || !Coordinate(bounds.Right) || !Coordinate(bounds.Bottom) || bounds.Width > 1000000 || bounds.Height > 1000000)
                    throw new InvalidDataException("原生截图图层超出范围");
                if (layer.Points == null || layer.Points.Length > 4096 || (points += layer.Points.Length) > 16384 || layer.Points.Any(p => !Finite(p.X, p.Y) || !Coordinate(p.X) || !Coordinate(p.Y)))
                    throw new InvalidDataException("原生截图笔画无效");
                if (layer.Kind == "pen" && layer.Points.Length == 0 || layer.Kind == "arrow" && (layer.Points.Length < 2 || layer.Points.Length > 7)) throw new InvalidDataException("原生截图笔画点数无效");
                if (layer.Smoothing < 0 || layer.Smoothing > 10) throw new InvalidDataException("笔画平滑参数无效");
                if (layer.StrokeWidth < 0 || layer.StrokeWidth > 64 || layer.Dash < 0 || layer.Dash > 4 || layer.CornerRadius < 0 || layer.CornerRadius > 150 || Math.Abs((long)layer.ShadowOffset.X) > 256 || Math.Abs((long)layer.ShadowOffset.Y) > 256)
                    throw new InvalidDataException("原生截图样式无效");
                if (layer.Kind == "mosaic" && (layer.EffectSize < 2 || layer.EffectSize > 10000) || layer.Kind == "blur" && (layer.EffectSize < 3 || layer.EffectSize > 199)) throw new InvalidDataException("原生截图效果无效");
                if (layer.Kind == "arrow" && !Enum.IsDefined(typeof(ArrowHeadDirection), layer.ArrowHead)) throw new InvalidDataException("箭头方向无效");
                if (layer.Kind == "text") {
                    if (layer.Text == null || layer.Text.Length > 4096 || layer.TextId == Guid.Empty || !ids.Add(layer.TextId) || !Finite(layer.FontPixels) || layer.FontPixels < 6 || layer.FontPixels > 240 || Color.FromArgb(layer.TextColor).A != 255)
                        throw new InvalidDataException("原生截图文字无效");
                    if (layer.Lines != null && (layer.Lines.Length > 4097 || layer.Lines.Any(line => line == null || line.Text == null || line.Text.Length > 4096 || line.X < 0 || line.Y < 0 || line.X > bounds.Width || line.Y > bounds.Height) || layer.Lines.Sum(line => line.Text.Length) > 4096))
                        throw new InvalidDataException("原生截图文字排版无效");
                }
            }
        }
        private static bool Finite(params float[] values) => values.All(float.IsFinite);
        private static bool Coordinate(float value) => value >= -1000000 && value <= 1000000;
    }

    public sealed record WinOneCaptureLayer
    {
        public string Kind { get; init; }
        public RectangleF Bounds { get; init; }
        public int StrokeColor { get; init; }
        public int FillColor { get; init; }
        public int StrokeWidth { get; init; }
        public int Dash { get; init; }
        public int CornerRadius { get; init; }
        public bool Shadow { get; init; }
        public int ShadowColor { get; init; }
        public Point ShadowOffset { get; init; }
        public PointF[] Points { get; init; } = Array.Empty<PointF>();
        public bool CurveInterpolation { get; init; }
        public int Smoothing { get; init; }
        public int ArrowHead { get; init; }
        public bool CenterNodeActive { get; init; }
        public string Text { get; init; } = "";
        public Guid TextId { get; init; }
        public float FontPixels { get; init; } = 24;
        public int TextColor { get; init; } = unchecked((int)0xffe23b30);
        public WinOneTextLine[] Lines { get; init; }
        public int EffectSize { get; init; }
    }

    public partial class FreehandDrawingShape
    {
        internal PointF[] WinOnePoints() => positions.ToArray();
        internal void WinOneRestorePoints(PointF[] value) => positions = value.ToList();
    }
    public partial class LineDrawingShape
    {
        internal void WinOneRestorePoints(PointF[] value, bool manualCenter)
        { Points = value.ToArray(); CenterPointCount = value.Length - 2; CenterNodeActive = manualCenter; }
    }

    internal partial class ShapeManager
    {
        internal WinOneCaptureDocument ExportWinOneDocument(string tool)
        {
            var origin = Form.CanvasRectangle.Location;
            var layers = Shapes.Where(shape => shape.ShapeCategory != ShapeCategory.Region).Select(shape => {
                var kind = shape switch { WinOneInlineTextShape _ => "text", WinOneCoverShape _ => "cover", RectangleDrawingShape _ => "rectangle", EllipseDrawingShape _ => "ellipse", ArrowDrawingShape _ => "arrow", FreehandDrawingShape _ => "pen", PixelateEffectShape _ => "mosaic", BlurEffectShape _ => "blur", _ => throw new InvalidDataException("无法保存该原生图层") };
                var layer = new WinOneCaptureLayer { Kind = kind, Bounds = shape.Rectangle };
                if (shape is BaseDrawingShape drawing) layer = layer with { StrokeColor = drawing.BorderColor.ToArgb(), FillColor = drawing.FillColor.ToArgb(), StrokeWidth = drawing.BorderSize, Dash = (int)drawing.BorderStyle, Shadow = drawing.Shadow, ShadowColor = drawing.ShadowColor.ToArgb(), ShadowOffset = drawing.ShadowOffset };
                if (shape is RectangleDrawingShape rectangle) layer = layer with { CornerRadius = rectangle.CornerRadius };
                if (shape is ArrowDrawingShape arrow) layer = layer with { Points = arrow.Points.ToArray(), ArrowHead = (int)arrow.ArrowHeadDirection, CenterNodeActive = arrow.CenterNodeActive };
                if (shape is FreehandDrawingShape pen) layer = layer with { Points = pen.WinOnePoints(), Smoothing = pen.Smoothing, CurveInterpolation = pen.CurveInterpolation };
                if (shape is PixelateEffectShape pixel) layer = layer with { EffectSize = pixel.PixelSize };
                if (shape is BlurEffectShape blur) layer = layer with { EffectSize = blur.BlurRadius };
                if (shape is WinOneInlineTextShape text) layer = layer with { Text = text.Text, TextId = text.Id, FontPixels = text.FontPixels, TextColor = text.TextColor.ToArgb(), Lines = text.NativeLines?.ToArray() };
                return layer with {
                    Bounds = new RectangleF(layer.Bounds.X - origin.X, layer.Bounds.Y - origin.Y, layer.Bounds.Width, layer.Bounds.Height),
                    Points = layer.Points.Select(point => new PointF(point.X - origin.X, point.Y - origin.Y)).ToArray()
                };
            }).ToArray();
            var document = new WinOneCaptureDocument(1, Form.Canvas.Width, Form.Canvas.Height, ProbeCommittedRegion is { } region
                ? Rectangle.Round(new RectangleF(region.Rectangle.X - origin.X, region.Rectangle.Y - origin.Y, region.Rectangle.Width, region.Rectangle.Height)) : null, tool, layers);
            document.Validate(Form.Canvas.Size); return document;
        }

        internal void RestoreWinOneDocument(WinOneCaptureDocument document)
        {
            document.Validate(Form.Canvas.Size);
            // Recovery initializes a new canvas before admitting editing input.
            // Never overwrite a live session or recycle a disposed history object.
            if (Shapes.Count != 0 || IsCreating || IsMoving || IsResizing || history.CanUndo || history.CanRedo)
                throw new InvalidOperationException("原生恢复需要空白截图会话");
            var restored = new List<BaseShape>();
            try {
                foreach (var layer in document.Layers) {
                    BaseShape shape = layer.Kind switch { "text" => new WinOneInlineTextShape(), "cover" => new WinOneCoverShape(), "rectangle" => new RectangleDrawingShape(), "ellipse" => new EllipseDrawingShape(), "arrow" => new ArrowDrawingShape(), "pen" => new FreehandDrawingShape(), "mosaic" => new PixelateEffectShape(), "blur" => new BlurEffectShape(), _ => throw new InvalidDataException("未知原生图层") };
                    restored.Add(shape); shape.Manager = this; shape.OnConfigLoad(); shape.Rectangle = layer.Bounds;
                    if (shape is BaseDrawingShape drawing) { drawing.BorderColor = Color.FromArgb(layer.StrokeColor); drawing.FillColor = Color.FromArgb(layer.FillColor); drawing.BorderSize = layer.StrokeWidth; drawing.BorderStyle = (BorderStyle)layer.Dash; drawing.Shadow = layer.Shadow; drawing.ShadowColor = Color.FromArgb(layer.ShadowColor); drawing.ShadowOffset = layer.ShadowOffset; }
                    if (shape is RectangleDrawingShape rectangle) rectangle.CornerRadius = layer.CornerRadius;
                    if (shape is ArrowDrawingShape arrow) { arrow.WinOneRestorePoints(layer.Points, layer.CenterNodeActive); arrow.ArrowHeadDirection = (ArrowHeadDirection)layer.ArrowHead; }
                    if (shape is FreehandDrawingShape pen) { pen.WinOneRestorePoints(layer.Points); pen.Smoothing = layer.Smoothing; pen.CurveInterpolation = layer.CurveInterpolation; }
                    if (shape is PixelateEffectShape pixel) pixel.PixelSize = layer.EffectSize;
                    if (shape is BlurEffectShape blur) blur.BlurRadius = layer.EffectSize;
                    if (shape is WinOneInlineTextShape text) { text.Id = layer.TextId; text.Text = layer.Text; text.FontPixels = layer.FontPixels; text.TextColor = Color.FromArgb(layer.TextColor); text.NativeLines = layer.Lines?.ToArray(); }
                }
            } catch { foreach (var shape in restored) shape.Dispose(); throw; }
            // Validate and construct before replacing any current editable state.
            ClearTools(); DeselectCurrentShape();
            Shapes = restored;
            ProbeRestoreSelection(document.Selection);
            MoveAll(Form.CanvasRectangle.Location);
            foreach (var shape in Shapes) shape.OnCreated();
            OnImageModified(); Form.Invalidate();
        }
    }
}
