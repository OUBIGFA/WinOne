using ShareX.ScreenCaptureLib;

internal static class NativeDrawingTests
{
    private static int[] Pixels(Bitmap image) => Enumerable.Range(0, image.Height).SelectMany(y => Enumerable.Range(0, image.Width).Select(x => image.GetPixel(x, y).ToArgb())).ToArray();
    public static void Run()
    {
        using var source = new Bitmap(120, 100);
        using (var graphics = Graphics.FromImage(source)) graphics.Clear(Color.White);
        foreach (var tool in new[] { "ellipse", "pen", "cover" }) {
            using var capture = new WinOneCaptureCanvas(source);
            var points = new[] { new PointF(10, 10), new PointF(35, 30), new PointF(60, 10) };
            capture.AddDrawing(tool, new Rectangle(10, 10, 50, 40), points);
            using var drawn = capture.Render(); var pixels = Pixels(drawn);
            if (pixels.All(pixel => pixel == Color.White.ToArgb())) throw new Exception(tool + " produced no drawing");
            if (tool == "cover") {
                var color = drawn.GetPixel(30, 30).ToArgb();
                for (var y = 10; y < 50; y++) for (var x = 10; x < 60; x++)
                    if (drawn.GetPixel(x, y).ToArgb() != color) throw new Exception($"Cover mismatch at {x},{y}: {drawn.GetPixel(x, y)}, expected {Color.FromArgb(color)}");
            }
            capture.Undo(); using var undone = capture.Render();
            if (Pixels(undone).Any(pixel => pixel != Color.White.ToArgb())) throw new Exception(tool + " undo did not restore source");
            capture.Redo(); using var redone = capture.Render();
            if (!Pixels(redone).SequenceEqual(pixels)) throw new Exception(tool + " redo changed output");
            if (tool == "cover") {
                capture.AddDrawing("rectangle", new Rectangle(70, 10, 40, 40));
                using var switched = capture.Render();
                if (switched.GetPixel(90, 30).ToArgb() != Color.White.ToArgb()) throw new Exception("Cover style leaked into rectangle");
            }
        }
        using var fractional = new WinOneCaptureCanvas(source);
        fractional.AddDrawing("cover", new RectangleF(10.4f, 10.4f, 49.2f, 39.2f));
        using var filled = fractional.Render();
        for (var y = 0; y < filled.Height; y++) for (var x = 0; x < filled.Width; x++) {
            var inside = x >= 10 && x < 60 && y >= 10 && y < 50;
            if ((filled.GetPixel(x, y).ToArgb() != Color.White.ToArgb()) != inside)
                throw new Exception("Fractional cover did not cover exactly its intersecting pixels");
        }
    }
}
