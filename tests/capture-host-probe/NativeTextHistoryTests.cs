using ShareX.ScreenCaptureLib;

internal static class NativeTextHistoryTests
{
    private static int[] Pixels(Bitmap image) => Enumerable.Range(0, image.Height)
        .SelectMany(y => Enumerable.Range(0, image.Width).Select(x => image.GetPixel(x, y).ToArgb())).ToArray();

    public static void Run()
    {
        using var image = new Bitmap(260, 180);
        using (var graphics = Graphics.FromImage(image)) graphics.Clear(Color.White);
        using var capture = new WinOneCaptureCanvas(image);
        var bounds = new Rectangle(20, 20, 220, 120);
        capture.SetRegion(new Rectangle(0, 0, image.Width, image.Height));
        capture.AddText("原始文字", bounds, 24, new[] { new WinOneTextLine("原始文字", 0, 0) });
        var id = capture.EditableText!.Id;
        using var original = capture.Render(); var expected = Pixels(original);
        capture.HideEditedText(id, true); capture.HideEditedText(id, false);
        using (var cancelled = capture.Render())
            if (!Pixels(cancelled).SequenceEqual(expected)) throw new Exception("Cancelling native text changed the image");
        capture.UpdateText(id, "修改文字", bounds, new[] { new WinOneTextLine("修改文字", 0, 0) });
        using var changed = capture.Render(); var changedPixels = Pixels(changed);
        if (changedPixels.SequenceEqual(expected)) throw new Exception("Native text edit did not affect output");
        capture.Undo();
        using (var undone = capture.Render())
            if (!Pixels(undone).SequenceEqual(expected)) throw new Exception("Native text undo lost layout or pixels");
        capture.Redo();
        using (var redone = capture.Render())
            if (!Pixels(redone).SequenceEqual(changedPixels)) throw new Exception("Native text redo changed layout or pixels");
        capture.UpdateText(id, "修改文字", bounds, new[] { new WinOneTextLine("修改文字", 0, 0) });
        capture.Undo();
        using (var afterNoChange = capture.Render())
            if (!Pixels(afterNoChange).SequenceEqual(expected)) throw new Exception("Unchanged native text added a history step");
        capture.UpdateText(id, "原始文字", bounds, new[] { new WinOneTextLine("原始文字", 0, 0) }, 36, Color.RoyalBlue);
        using var styled = capture.Render(); var styledPixels = Pixels(styled);
        if (styledPixels.SequenceEqual(expected)) throw new Exception("Text style did not change output");
        capture.Undo();
        using (var styleUndo = capture.Render())
            if (!Pixels(styleUndo).SequenceEqual(expected)) throw new Exception("Text style undo lost original pixels");
        capture.Redo();
        using (var styleRedo = capture.Render())
            if (!Pixels(styleRedo).SequenceEqual(styledPixels)) throw new Exception("Text style redo lost color or size");
    }
}
