using WinOne.Capture;

internal static class AnnotationTests
{
    public static void Run()
    {
        using var source = new Bitmap(100, 80);
        using (var drawing = Graphics.FromImage(source)) drawing.Clear(Color.SlateBlue);
        var document = new AnnotationDocument { Width = 100, Height = 80, Marks = [new AnnotationMark { Kind = "rectangle", X = 10, Y = 10, Width = 30, Height = 30, Color = "#ff0000", StrokeWidth = 4 }] };
        using var output = AnnotationRenderer.Render(source, document);
        if (output.GetPixel(10, 20).ToArgb() != Color.Red.ToArgb() || output.GetPixel(70, 60).ToArgb() != Color.SlateBlue.ToArgb()) throw new Exception("annotation rendering changed the wrong pixels");
        if (source.GetPixel(10, 20).ToArgb() != Color.SlateBlue.ToArgb()) throw new Exception("annotation rendering changed the original");
        foreach (var mark in new[] {
            new AnnotationMark { Kind = "rectangle", X = float.NaN, Width = 10, Height = 10 },
            new AnnotationMark { Kind = "rectangle", X = 99, Width = 10, Height = 10 },
            new AnnotationMark { Kind = "unknown", Width = 10, Height = 10 },
        })
        {
            try { using var invalid = AnnotationRenderer.Render(source, document with { Marks = [mark] }); throw new Exception("invalid annotation accepted"); }
            catch (InvalidDataException) { }
        }
        using var covered = AnnotationRenderer.Render(source, document with { Marks = [new AnnotationMark { Kind = "cover", X = 10, Y = 10, Width = 20, Height = 20, Color = "#000000" }] });
        if (covered.GetPixel(20, 20).ToArgb() != Color.Black.ToArgb()) throw new Exception("cover did not replace pixels");
        var cropDocument = document with { Version = 2, Crop = new AnnotationCrop(15, 15, 40, 30), Marks = [new AnnotationMark { Kind = "cover", X = 10, Y = 10, Width = 20, Height = 20, Color = "#000000" }] };
        using var cropped = AnnotationRenderer.Render(source, cropDocument);
        if (cropped.Size != new Size(40, 30) || cropped.GetPixel(0, 0).ToArgb() != Color.Black.ToArgb() || cropped.GetPixel(20, 20).ToArgb() != Color.SlateBlue.ToArgb()) throw new Exception("crop lost source-space annotation alignment");
        using var cropOnly = AnnotationRenderer.Render(source, cropDocument with { Marks = [] });
        if (cropOnly.Size != new Size(40, 30) || cropOnly.GetPixel(0, 0).ToArgb() != Color.SlateBlue.ToArgb()) throw new Exception("crop-only export failed");
        foreach (var invalidCrop in new[] { new AnnotationCrop(-1, 0, 10, 10), new AnnotationCrop(90, 0, 11, 10), new AnnotationCrop(0, 0, 0, 10), new AnnotationCrop(int.MaxValue, 0, 10, 10) })
        {
            try { using var invalid = AnnotationRenderer.Render(source, cropDocument with { Crop = invalidCrop }); throw new Exception("invalid crop accepted"); }
            catch (InvalidDataException) { }
        }
        try { using var invalid = AnnotationRenderer.Render(source, cropDocument with { Version = 1 }); throw new Exception("crop requires document version 2"); }
        catch (InvalidDataException) { }
        // Text may wrap or fall back to a CJK font, but it must remain inside
        // the chosen region and leave the original image untouched.
        using var text = AnnotationRenderer.Render(source, document with { Marks = [new AnnotationMark { Kind = "text", X = 10, Y = 10, Width = 55, Height = 40, Text = "WinOne\n项目进度", FontSize = 16, Color = "#ffffff" }] });
        var paintedTextPixels = 0;
        for (var y = 0; y < text.Height; y++)
        for (var x = 0; x < text.Width; x++)
        {
            if (source.GetPixel(x, y).ToArgb() != Color.SlateBlue.ToArgb()) throw new Exception("text changed original image");
            if (text.GetPixel(x, y).ToArgb() == Color.SlateBlue.ToArgb()) continue;
            if (x < 10 || y < 10 || x >= 65 || y >= 50) throw new Exception("text escaped its clipping bounds");
            paintedTextPixels++;
        }
        if (paintedTextPixels == 0) throw new Exception("text was not rendered");
        foreach (var kind in new[] { "mosaic", "blur" })
        {
            using var checker = new Bitmap(100, 80);
            for (var y = 0; y < checker.Height; y++)
            for (var x = 0; x < checker.Width; x++) checker.SetPixel(x, y, (x + y) % 2 == 0 ? Color.Black : Color.White);
            using var filtered = AnnotationRenderer.Render(checker, document with { Marks = [new AnnotationMark { Kind = kind, X = 10, Y = 10, Width = 40, Height = 30, EffectSize = 12 }] });
            var blendedPixels = 0;
            for (var y = 0; y < filtered.Height; y++)
            for (var x = 0; x < filtered.Width; x++)
            {
                var pixel = filtered.GetPixel(x, y);
                if (x < 10 || y < 10 || x >= 50 || y >= 40)
                {
                    if (pixel.ToArgb() != checker.GetPixel(x, y).ToArgb()) throw new Exception("filter changed pixels outside selected region");
                }
                else if (pixel.R > 0 && pixel.R < 255) blendedPixels++;
            }
            if (blendedPixels == 0) throw new Exception("filter did not blend the checkerboard");
            foreach (var size in new[] { new Size(1, 1), new Size(1, 40), new Size(40, 1), new Size(20, 20) })
            {
                using var tiny = AnnotationRenderer.Render(source, document with { Marks = [new AnnotationMark { Kind = kind, X = 0, Y = 0, Width = size.Width, Height = size.Height, EffectSize = 64 }] });
                if (tiny.GetPixel(50, 50).ToArgb() != Color.SlateBlue.ToArgb()) throw new Exception("effect changed pixels outside its region");
            }
        }
    }
}
