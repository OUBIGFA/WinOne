using System.Reflection;
using ShareX.ScreenCaptureLib;

internal static class RecordingEffectTests
{
    internal static void Run()
    {
        using var image = new Bitmap(360, 220);
        using var canvas = new WinOneCaptureCanvas(image);
        // The locked upstream manager is internal to ShareX. Inspect only its
        // association; rendering is the real public effect/overlay API, not a
        // copied renderer or source-text assertion.
        var manager = canvas.Window.GetType().GetProperty("ShapeManager", BindingFlags.Instance | BindingFlags.NonPublic)!.GetValue(canvas.Window);
        foreach (var effect in new BaseEffectShape[] { new BlurEffectShape(), new PixelateEffectShape(), new HighlightEffectShape() }) {
            using (effect) {
                typeof(BaseShape).GetProperty("Manager", BindingFlags.Instance | BindingFlags.NonPublic)!.SetValue(effect, manager);
                effect.Rectangle = new RectangleF(8, 8, 340, 200);
                using var empty = Draw(effect, "");
                foreach (var caption in new[] { effect.OverlayText, "Processing", "模糊 [15]" }) {
                    using var labelled = Draw(effect, caption);
                    if (!Same(empty, labelled)) throw new Exception("Effect preview still paints a caption: " + effect.ShapeType);
                }
                typeof(BaseShape).GetProperty("Manager", BindingFlags.Instance | BindingFlags.NonPublic)!.SetValue(effect, null);
                using var upstream = Draw(effect, "Processing");
                if (Same(empty, upstream)) throw new Exception("WinOne caption suppression changed unrelated upstream rendering");
            }
        }
    }
    private static Bitmap Draw(BaseEffectShape effect, string text)
    {
        var image = new Bitmap(360, 220);
        using var graphics = Graphics.FromImage(image);
        graphics.Clear(Color.White); effect.OnDrawOverlay(graphics, text);
        return image;
    }
    private static bool Same(Bitmap first, Bitmap second)
    {
        for (var y = 0; y < first.Height; y++) for (var x = 0; x < first.Width; x++)
            if (first.GetPixel(x, y) != second.GetPixel(x, y)) return false;
        return true;
    }
}
