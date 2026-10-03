using ShareX.ScreenCaptureLib;

internal static class KeyboardRoutingTests
{
    public static void Run()
    {
        using var image = new Bitmap(300, 200);
        using var capture = new WinOneCaptureCanvas(image);
        var received = new List<Keys>();
        capture.KeyPressed += received.Add;
        // Exercise the actual Form KeyDown event, without changing OS keyboard state.
        var keyDown = typeof(Control).GetMethod("OnKeyDown", System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.NonPublic)!;
        keyDown.Invoke(capture.Window, new object[] { new KeyEventArgs(Keys.T) });
        keyDown.Invoke(capture.Window, new object[] { new KeyEventArgs(Keys.Control | Keys.Z) });
        if (!received.SequenceEqual(new[] { Keys.T, Keys.Control | Keys.Z }))
            throw new Exception("Native keys bypassed the hosted command entry");
        if (capture.Tool == "DrawingTextBackground")
            throw new Exception("Native key selected the standalone text editor");
    }
}
