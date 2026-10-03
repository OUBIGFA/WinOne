using ShareX.ScreenCaptureLib;

internal static class NativeViewportTests
{
    private static int[] Pixels(Bitmap image) => Enumerable.Range(0, image.Height)
        .SelectMany(y => Enumerable.Range(0, image.Width).Select(x => image.GetPixel(x, y).ToArgb())).ToArray();
    public static int Run()
    {
        using var image = new Bitmap(360, 240);
        using (var graphics = Graphics.FromImage(image)) graphics.Clear(Color.LightBlue);
        using var source = new WinOneCaptureCanvas(image);
        source.SetRegion(new Rectangle(10, 10, 340, 220));
        source.AddRectangle(new Rectangle(20, 20, 80, 50));
        source.AddDrawing("arrow", new RectangleF(30, 90, 70, 40));
        source.AddDrawing("pen", new RectangleF(120, 20, 90, 60), [new(120, 20), new(150, 80), new(210, 35)]);
        source.AddDrawing("mosaic", new RectangleF(40, 30, 40, 30));
        source.AddDrawing("blur", new RectangleF(150, 40, 30, 30));
        source.AddText("文字 WinOne", new Rectangle(25, 150, 290, 60), 24, [new("文字 WinOne", 0, 0)]);
        source.SelectTool("select");
        var document = source.ExportDocument();
        var encoded = NativeCaptureDocumentCodec.Encode(document, image.Size);
        using var expectedImage = source.RenderSelection(); var expected = Pixels(expectedImage);
        using var restored = new WinOneCaptureCanvas(image, viewport: true);
        restored.RestoreDocument(document);
        foreach (var (scale, origin) in new[] { (0.5f, new PointF(-40, 100)), (1.5f, new PointF(25, -50)), (0.75f, new PointF(-15.5f, 22.25f)) }) {
            restored.SetViewport(scale, origin);
            var snapshot = restored.SnapshotDocument() ?? throw new Exception("Stable viewport could not be saved");
            if (!NativeCaptureDocumentCodec.Encode(snapshot, image.Size).SequenceEqual(encoded)) throw new Exception("Zoom/pan changed image-space document data");
            using var output = restored.RenderSelection();
            if (output.Size != expectedImage.Size || !Pixels(output).SequenceEqual(expected)) throw new Exception("Viewport changed full-resolution exported pixels");
            if (restored.TextAt(new Point(40, 160))?.Text != "文字 WinOne") throw new Exception("Panned text could not be selected in image coordinates");
            var view = restored.ImageToView(new Rectangle(40, 160, 100, 40));
            var point = restored.ViewToImage(view.Location);
            if (Math.Abs(point.X - 40) > 1 || Math.Abs(point.Y - 160) > 1) throw new Exception("Viewport input mapping drifted");
            restored.AddDrawing("cover", new RectangleF(110, 100, 40, 30));
            restored.Undo();
            using var undone = restored.RenderSelection();
            if (!Pixels(undone).SequenceEqual(expected)) throw new Exception("Viewport undo lost original layers or selection");
            restored.SelectTool(document.Tool);
        }
        Console.WriteLine("PASS: zoom/pan preserves editable documents, full-resolution pixels, text targeting and undo");
        return 0;
    }
}
