using System.Drawing.Drawing2D;
using System.Drawing.Imaging;
using System.Drawing.Text;
using System.Globalization;
using ShareX.HelpersLib;

namespace WinOne.Capture;

internal record EditPoint(float X, float Y);
internal record AnnotationCrop(int X, int Y, int Width, int Height);
internal record AnnotationMark
{
    public string Kind { get; init; } = "rectangle";
    public float X { get; init; }
    public float Y { get; init; }
    public float Width { get; init; }
    public float Height { get; init; }
    public string Color { get; init; } = "#ff0000";
    public float StrokeWidth { get; init; } = 3;
    public string Text { get; init; } = "";
    public float FontSize { get; init; } = 24;
    public int EffectSize { get; init; } = 12;
    public IReadOnlyList<EditPoint> Points { get; init; } = [];
}
internal record AnnotationDocument
{
    public int Version { get; init; } = 1;
    public int Width { get; init; }
    public int Height { get; init; }
    public IReadOnlyList<AnnotationMark> Marks { get; init; } = [];
    public AnnotationCrop? Crop { get; init; }
}

internal static class AnnotationRenderer
{
    // Coordinates are original-image pixels, independent of the preview scale.
    // Validate the entire document before allocating or painting an output image.
    public static Bitmap Render(Bitmap source, AnnotationDocument document)
    {
        var cropBounds = ValidateDocument(source.Size, document);
        // Crop-only saves avoid an intermediate full-size bitmap. With marks,
        // render first so effects and lines retain their original coordinates.
        if (document.Marks.Count == 0) return source.Clone(cropBounds, PixelFormat.Format32bppArgb);
        var output = source.Clone(new Rectangle(Point.Empty, source.Size), PixelFormat.Format32bppArgb);
        try
        {
            foreach (var mark in document.Marks) Draw(output, mark);
            if (cropBounds == new Rectangle(Point.Empty, source.Size)) return output;
            var cropped = output.Clone(cropBounds, PixelFormat.Format32bppArgb);
            output.Dispose();
            return cropped;
        }
        catch { output.Dispose(); throw; }
    }

    internal static Rectangle ValidateDocument(Size size, AnnotationDocument document)
    {
        if (size.Width < 1 || size.Height < 1 || document.Version is not (1 or 2) || document.Width != size.Width || document.Height != size.Height || document.Marks is null || document.Marks.Count > 512)
            throw new InvalidDataException("标注文档版本、尺寸或数量无效");
        var cropBounds = new Rectangle(Point.Empty, size);
        if (document.Crop is { } crop)
        {
            if (document.Version != 2 || crop.X < 0 || crop.Y < 0 || crop.Width < 1 || crop.Height < 1 || (long)crop.X + crop.Width > size.Width || (long)crop.Y + crop.Height > size.Height)
                throw new InvalidDataException("裁剪范围无效");
            cropBounds = new Rectangle(crop.X, crop.Y, crop.Width, crop.Height);
        }
        var points = 0;
        foreach (var mark in document.Marks)
        {
            if (mark is null) throw new InvalidDataException("标注为空");
            Validate(mark, size);
            points += mark.Points.Count;
            if (points > 16384) throw new InvalidDataException("笔画点数过多");
        }
        return cropBounds;
    }

    private static void Validate(AnnotationMark mark, Size size)
    {
        if (mark.Kind is not ("rectangle" or "ellipse" or "arrow" or "pen" or "text" or "mosaic" or "blur" or "cover")) throw new InvalidDataException("不支持的标注工具");
        if (!float.IsFinite(mark.StrokeWidth) || mark.StrokeWidth < 1 || mark.StrokeWidth > 64 || mark.Points is null) throw new InvalidDataException("笔画参数无效");
        _ = ParseColor(mark.Color);
        if (mark.Kind is "pen" or "arrow")
        {
            if (mark.Points.Count < 2 || mark.Points.Count > 4096 || mark.Kind == "arrow" && mark.Points.Count != 2) throw new InvalidDataException("笔画点数无效");
            foreach (var point in mark.Points)
                if (point is null || !float.IsFinite(point.X) || !float.IsFinite(point.Y) || point.X < 0 || point.Y < 0 || point.X > size.Width || point.Y > size.Height) throw new InvalidDataException("笔画超出图像范围");
            if (mark.Kind == "arrow" && mark.Points[0] == mark.Points[1]) throw new InvalidDataException("箭头长度为零");
            return;
        }
        if (!float.IsFinite(mark.X) || !float.IsFinite(mark.Y) || !float.IsFinite(mark.Width) || !float.IsFinite(mark.Height) || mark.X < 0 || mark.Y < 0 || mark.Width < 1 || mark.Height < 1 || (double)mark.X + mark.Width > size.Width || (double)mark.Y + mark.Height > size.Height) throw new InvalidDataException("标注超出图像范围");
        if (mark.Kind == "text" && (mark.Text is null || mark.Text.Length > 4096 || !float.IsFinite(mark.FontSize) || mark.FontSize < 8 || mark.FontSize > 256)) throw new InvalidDataException("文字参数无效");
        if (mark.Kind is "mosaic" or "blur" && (mark.EffectSize < 2 || mark.EffectSize > 64)) throw new InvalidDataException("图像效果参数无效");
    }

    private static Color ParseColor(string value)
    {
        if (value is null || value.Length != 7 || value[0] != '#' || !uint.TryParse(value.AsSpan(1), NumberStyles.HexNumber, CultureInfo.InvariantCulture, out var color)) throw new InvalidDataException("标注颜色无效");
        return Color.FromArgb(255, (int)(color >> 16), (int)((color >> 8) & 255), (int)(color & 255));
    }
    private static Rectangle Pixels(AnnotationMark mark) => Rectangle.FromLTRB((int)Math.Floor(mark.X), (int)Math.Floor(mark.Y), (int)Math.Ceiling((double)mark.X + mark.Width), (int)Math.Ceiling((double)mark.Y + mark.Height));

    private static void Draw(Bitmap image, AnnotationMark mark)
    {
        if (mark.Kind is "mosaic" or "blur")
        {
            var area = Pixels(mark);
            using var patch = image.Clone(area, PixelFormat.Format32bppArgb);
            if (mark.Kind == "mosaic") ImageHelpers.Pixelate(patch, mark.EffectSize);
            // The bounded box blur handles one-pixel/narrow regions; the fast
            // variant assumes a radius smaller than the bitmap dimensions.
            else ImageHelpers.BoxBlur(patch, mark.EffectSize);
            using var target = Graphics.FromImage(image);
            target.CompositingMode = CompositingMode.SourceCopy;
            target.DrawImageUnscaled(patch, area.Location);
            return;
        }
        using var graphics = Graphics.FromImage(image);
        graphics.PageUnit = GraphicsUnit.Pixel;
        graphics.SmoothingMode = SmoothingMode.AntiAlias;
        graphics.TextRenderingHint = TextRenderingHint.AntiAliasGridFit;
        using var pen = new Pen(ParseColor(mark.Color), mark.StrokeWidth) { LineJoin = LineJoin.Round, StartCap = LineCap.Round, EndCap = LineCap.Round };
        using var brush = new SolidBrush(pen.Color);
        var bounds = new RectangleF(mark.X, mark.Y, mark.Width, mark.Height);
        switch (mark.Kind)
        {
            case "rectangle": graphics.DrawRectangle(pen, bounds.X, bounds.Y, bounds.Width, bounds.Height); break;
            case "ellipse": graphics.DrawEllipse(pen, bounds); break;
            case "pen": graphics.DrawLines(pen, mark.Points.Select(point => new PointF(point.X, point.Y)).ToArray()); break;
            case "arrow":
                using (var cap = new AdjustableArrowCap(4, 5, true))
                { pen.CustomEndCap = cap; graphics.DrawLine(pen, mark.Points[0].X, mark.Points[0].Y, mark.Points[1].X, mark.Points[1].Y); }
                break;
            case "text":
                using (var font = new Font("Segoe UI", mark.FontSize, FontStyle.Regular, GraphicsUnit.Pixel))
                { graphics.SetClip(bounds); graphics.DrawString(mark.Text, font, brush, bounds, StringFormat.GenericTypographic); }
                break;
            case "cover":
                graphics.SmoothingMode = SmoothingMode.None;
                graphics.FillRectangle(brush, Pixels(mark));
                break;
        }
    }
}
