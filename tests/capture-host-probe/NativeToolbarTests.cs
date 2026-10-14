using ShareX.ScreenCaptureLib;

internal static class NativeToolbarTests
{
    public static void Run()
    {
        using (var source = new Bitmap(300, 200))
        using (var canvas = new WinOneCaptureCanvas(source))
        using (var session = new NativeCaptureSession(canvas, _ => {}, _ => {})) {
            var form = (RegionCaptureForm)canvas.Window;
            if (!form.Options.ShowMagnifier) throw new Exception("Initial selection lost its magnifier");
            canvas.SetRegion(new Rectangle(10, 10, 200, 150));
            if (form.Options.ShowMagnifier) throw new Exception("Committed selection still shows the magnifier");
            foreach (var tool in new[] { "select", "rectangle", "region" }) {
                canvas.SelectTool(tool);
                if (form.Options.ShowMagnifier) throw new Exception("Changing tool restored the magnifier after selection");
            }
            using var restored = new WinOneCaptureCanvas(source);
            restored.RestoreDocument(canvas.ExportDocument());
            if (((RegionCaptureForm)restored.Window).Options.ShowMagnifier) throw new Exception("Restored selection shows the magnifier");
            var starts=0;
            session.EnableRetake(()=>{starts++;return Task.CompletedTask;},false);
            var scroll=session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(row=>row.Items.OfType<ToolStripButton>()).Single(item=>item.Name=="scroll");
            scroll.PerformClick();
            if(starts!=1 || !scroll.Checked)throw new Exception("Scroll switch did not enable capture");
            scroll.PerformClick();
            if(starts!=1 || scroll.Checked)throw new Exception("Turning scrolling off started another capture");
        }
        var viewport = new Rectangle(0, 0, 1200, 800);
        var preferred = new Size(400, 96);
        var below = ToolbarPlacement.Place(new Rectangle(300, 100, 600, 300), viewport, preferred);
        if (below.Location != new Point(400, 408)) throw new Exception("Toolbar must start centered below the selection");
        var bottom = ToolbarPlacement.Place(new Rectangle(100, 100, 1000, 680), viewport, preferred);
        if (bottom.Location != new Point(692, 676)) throw new Exception("Toolbar must fall back to the selection bottom-right, not above it");
        var dragged = ToolbarPlacement.Place(new Rectangle(300, 100, 600, 300), viewport, preferred, new Point(-50, 900));
        if (dragged.Location != new Point(8, 696)) throw new Exception("User toolbar placement escaped the monitor");
        using (var stable = new NativeToolbarControl(saveToolbar: value => Task.FromResult(value))) {
            var row = (ToolStrip)stable.Controls["capture-tools"]!;
            if (row.Items.OfType<ToolStripButton>().First().Name != "select") throw new Exception("Move must be the first annotation tool");
            string? command = null;
            stable.Command += value => command = value;
            row.Items["select"]!.PerformClick();
            if (command != "select") throw new Exception("Move did not dispatch the selection tool");
            var style = row.Items.OfType<NativeToolOptions>().Single();
            stable.SetToolStyle("rectangle", new WinOneToolStyle(Color.Red, 3), false);
            var size = stable.PreferredToolbarSize;
            stable.SetToolStyle("region", new WinOneToolStyle(Color.Red, 3), false);
            if (!style.Available || style.Enabled || stable.PreferredToolbarSize != size) throw new Exception("Unsupported tool removed or resized Style instead of disabling it");
            stable.SetToolStyle("text", new WinOneToolStyle(Color.RoyalBlue, 24), false);
            if (!style.Enabled || !style.Available) throw new Exception("Supported tool did not restore Style");
            if (style.DropDownItems.OfType<ToolStripMenuItem>().Any(item => item.HasDropDownItems)) throw new Exception("Style requires an extra submenu");
            var colors = style.DropDownItems.OfType<ToolStripMenuItem>().Where(item => item.Tag is Color).ToArray();
            if (colors.Length != 6 || colors.Any(item => item.Image is null || string.IsNullOrWhiteSpace(item.Text))) throw new Exception("Colors need both swatches and names at the first menu level");
            if (row.OverflowButton.DropDown.Items.Find("toolbar-save", false).Length != 1) throw new Exception("Customization is hidden behind an extra menu");
        }
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
