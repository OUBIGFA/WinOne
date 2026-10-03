using ShareX.ScreenCaptureLib;

internal static class NativeToolStyleTests
{
    private static int[] Pixels(Bitmap image) => Enumerable.Range(0, image.Height)
        .SelectMany(y => Enumerable.Range(0, image.Width).Select(x => image.GetPixel(x, y).ToArgb())).ToArray();

    public static void Run()
    {
        using var source = new Bitmap(120, 100);
        for (var y = 0; y < source.Height; y++) for (var x = 0; x < source.Width; x++)
            source.SetPixel(x, y, (x / 3 + y / 3) % 2 == 0 ? Color.White : Color.Black);
        foreach (var tool in new[] { "rectangle", "ellipse", "arrow", "pen", "cover", "mosaic", "blur" }) {
            using var canvas = new WinOneCaptureCanvas(source);
            canvas.SetRegion(new Rectangle(0, 0, 120, 100));
            canvas.AddDrawing(tool, new Rectangle(15, 15, 70, 60), new[] { new PointF(15, 15), new PointF(40, 65), new PointF(85, 15) });
            if (canvas.ExportDocument().Layers.Any(layer => layer.Shadow))
                throw new Exception(tool + " unexpectedly adds a drop shadow");
            using var before = canvas.Render(); var oldPixels = Pixels(before);
            var style = canvas.GetToolStyle();
            var next = new WinOneToolStyle(Color.RoyalBlue, tool == "blur" ? (style.Value == 31 ? 9 : 31) : tool == "mosaic" ? (style.Value == 13 ? 5 : 13) : 9);
            canvas.SetToolStyle(next);
            using var changed = canvas.Render(); var newPixels = Pixels(changed);
            if (newPixels.SequenceEqual(oldPixels)) throw new Exception(tool + " style did not change output");
            canvas.Undo(); using var undone = canvas.Render();
            if (!Pixels(undone).SequenceEqual(oldPixels) || canvas.AnnotationCount != 1) throw new Exception(tool + " style undo lost the original mark");
            canvas.Redo(); using var redone = canvas.Render();
            if (!Pixels(redone).SequenceEqual(newPixels)) throw new Exception(tool + " style redo changed pixels");
            if (canvas.Selection != new Rectangle(0, 0, 120, 100)) throw new Exception("Style changed the crop");
        }
    }
}
