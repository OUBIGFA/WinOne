using ShareX.ScreenCaptureLib;

internal static class SingleSelectionTests
{
    public static void Run()
    {
        using var image = new Bitmap(300, 200);
        using var capture = new WinOneCaptureCanvas(image);
        var first = new Rectangle(10, 10, 200, 150);
        var second = new Rectangle(30, 20, 180, 120);
        capture.SetRegion(first);
        capture.AddRectangle(new Rectangle(50, 50, 60, 40));
        capture.BeginRegion(second);
        if (capture.Selection != first) throw new Exception("In-progress selection replaced the committed region");
        capture.FinishGesture();
        if (capture.SelectionCount != 1 || capture.Selection != second)
            throw new Exception("Replacing a selection retained another export region");
        // Annotation history must not unexpectedly change the chosen crop.
        capture.Undo();
        if (capture.SelectionCount != 1 || capture.Selection != second || capture.AnnotationCount != 0)
            throw new Exception("Annotation undo changed the screenshot selection");
        capture.Redo();
        if (capture.SelectionCount != 1 || capture.Selection != second || capture.AnnotationCount != 1)
            throw new Exception("Annotation redo changed the screenshot selection");
        using var result = capture.RenderSelection();
        if (result.Size != second.Size) throw new Exception("Output did not match the visible selection");
    }
}
