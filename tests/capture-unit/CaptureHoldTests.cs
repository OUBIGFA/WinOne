using WinOne.Capture;

internal static class CaptureHoldTests
{
    // Off-screen rendering checks ownership without covering the user's desktop.
    public static void Run()
    {
        Exception? failure = null;
        var thread = new Thread(() =>
        {
            try
            {
                var pixels = new Bitmap(80, 60);
                using (var graphics = Graphics.FromImage(pixels)) graphics.Clear(Color.Coral);
                using var frame = CapturedFrame.Own(pixels, new Rectangle(-80, -60, 80, 60));
                using (var borrowed = new CaptureHoldWindow(frame, frame.Bounds)) { }
                using (var crop = frame.Crop(frame.Bounds))
                    Check(crop.GetPixel(10, 10).ToArgb() == Color.Coral.ToArgb(), "borrowed hold disposed the source");
                using (var hold = new CaptureHoldWindow(frame, frame.Bounds))
                {
                    hold.Adopt(frame, new string('a', 32));
                    frame.Dispose();
                    Check(hold.DraftId == new string('a', 32), "adopt lost the draft identity");
                    using var painted = new Bitmap(80, 60);
                    hold.DrawToBitmap(painted, new Rectangle(0, 0, 80, 60));
                    Check(painted.GetPixel(10, 10).ToArgb() == Color.Coral.ToArgb(), "handoff lost the original frame");
                }
                try { _ = pixels.GetPixel(10, 10); throw new Exception("hold retained its owned image after disposal"); }
                catch (ArgumentException) { }
            }
            catch (Exception error) { failure = error; }
        });
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        if (!thread.Join(TimeSpan.FromSeconds(10))) throw new Exception("handoff test timed out");
        if (failure is not null) throw new Exception("handoff lifetime test failed", failure);
    }
    private static void Check(bool condition, string message) { if (!condition) throw new Exception(message); }
}
