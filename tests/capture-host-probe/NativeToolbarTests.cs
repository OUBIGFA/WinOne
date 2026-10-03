using ShareX.ScreenCaptureLib;

internal static class NativeToolbarTests
{
    public static void Run()
    {
        using var image = new Bitmap(300, 200);
        using var capture = new WinOneCaptureCanvas(image);
        capture.SetRegion(new Rectangle(10, 10, 200, 150));
        capture.AddRectangle(new Rectangle(40, 40, 60, 40));
        using var toolbar = new NativeToolbarControl();
        var copied = 0; var closed = 0;
        var confirmation = new CaptureCompletion(capture.RenderSelection, bitmap => {
            if (bitmap.Size != new Size(200, 150)) throw new Exception("Toolbar copied the wrong selection");
            copied++;
        }, () => closed++);
        toolbar.Command += command => {
            switch (command) {
                case "undo": capture.Undo(); break;
                case "redo": capture.Redo(); break;
                case "confirm": confirmation.Complete(); break;
                default: throw new Exception("Unexpected command");
            }
            toolbar.SetState(capture.CanUndo, capture.CanRedo, "rectangle", confirmation.Committed);
        };
        toolbar.SetState(capture.CanUndo, capture.CanRedo, "rectangle", false);
        ToolStripItem Button(string name) => toolbar.Controls.OfType<ToolStrip>().SelectMany(row => row.Items.Cast<ToolStripItem>()).Single(item => item.Name == name);
        Button("undo").PerformClick();
        if (capture.AnnotationCount != 0 || !Button("redo").Enabled) throw new Exception("Native toolbar undo failed");
        Button("redo").PerformClick();
        if (capture.AnnotationCount != 1) throw new Exception("Native toolbar redo failed");
        Button("confirm").PerformClick(); Button("confirm").PerformClick();
        if (copied != 1 || closed != 1 || Button("undo").Enabled) throw new Exception("Native toolbar duplicated completion");
    }
}
