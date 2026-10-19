using System.Drawing.Imaging;
using ShareX.ScreenCaptureLib;

internal static class NativeCanvasCopyTests
{
    internal static void Run()
    {
        // The fast screen copy must stay independent of the acquisition buffer
        // used concurrently by autosave. Non-screen formats still normalize.
        foreach (var format in new[] { PixelFormat.Format32bppArgb, PixelFormat.Format24bppRgb }) {
            using var image = new Bitmap(40, 30, format);
            for (var y = 0; y < image.Height; y++)
                for (var x = 0; x < image.Width; x++)
                    image.SetPixel(x, y, Color.FromArgb((x * 7) % 256, (y * 11) % 256, (x + y) % 256));
            var expected = image.GetPixel(4, 5);
            using var canvas = new WinOneCaptureCanvas(image);
            image.SetPixel(4, 5, Color.Magenta);
            using var output = canvas.Render();
            if (output.Size != image.Size || output.GetPixel(4, 5).ToArgb() != expected.ToArgb())
                throw new InvalidOperationException("Canvas copy shared or changed source pixels: " + format);
            canvas.SetRegion(new Rectangle(2, 3, 10, 9));
            using var selected = canvas.RenderSelection();
            if (selected.Size != new Size(10, 9) || selected.GetPixel(2, 2).ToArgb() != expected.ToArgb())
                throw new InvalidOperationException("Fast canvas copy changed physical selection pixels");
        }
    }
}
