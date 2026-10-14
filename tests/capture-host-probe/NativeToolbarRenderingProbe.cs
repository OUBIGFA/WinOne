using System.Text.Json;

internal static class NativeToolbarRenderingProbe
{
    public static int Run(string output)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal))
            throw new InvalidOperationException("Run this visual probe on an isolated test desktop");
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        // Observe the real session transition, not a bitmap rendered after layout.
        using (var source = new Bitmap(800, 600))
        using (var canvas = new ShareX.ScreenCaptureLib.WinOneCaptureCanvas(source))
        using (var session = new NativeCaptureSession(canvas, _ => { }, _ => { })) {
            canvas.Window.Show();
            Application.DoEvents();
            Rectangle? firstVisibleBounds = null;
            session.Toolbar.VisibleChanged += (_, _) => {
                if (session.Toolbar.Visible) firstVisibleBounds ??= session.Toolbar.Bounds;
            };
            var allocated = GC.GetAllocatedBytesForCurrentThread();
            var watch = System.Diagnostics.Stopwatch.StartNew();
            var firstPaint = new Dictionary<string, double>();
            foreach (var control in session.Toolbar.Controls.Cast<Control>().Prepend(session.Toolbar)) {
                var name = control == session.Toolbar ? "background" : control.Name;
                control.Paint += (_, _) => firstPaint.TryAdd(name, watch.Elapsed.TotalMilliseconds);
            }
            canvas.SetRegion(new Rectangle(100, 100, 400, 250));
            canvas.SelectTool("rectangle");
            var dispatchMs = watch.Elapsed.TotalMilliseconds;
            // Drain normal paint messages without forcing Update/DrawToBitmap,
            // which would conceal the background-to-buttons dispatch gap.
            Application.DoEvents();
            watch.Stop();
            allocated = GC.GetAllocatedBytesForCurrentThread() - allocated;
            File.WriteAllText(Path.Combine(output, "first-show.json"), JsonSerializer.Serialize(new { firstVisibleBounds, finalBounds = session.Toolbar.Bounds, dispatchMs, elapsedMs = watch.Elapsed.TotalMilliseconds, firstPaint, allocatedBytes = allocated, scope = "Isolated desktop message dispatch; not compositor presentation latency" }));
            if (firstVisibleBounds != session.Toolbar.Bounds)
                throw new Exception("Toolbar becomes visible before its final layout");
            if (firstPaint.Count != 4) throw new Exception("Toolbar did not paint all three rows and background");
            var cycles = new List<object>();
            using var process = System.Diagnostics.Process.GetCurrentProcess();
            for (var iteration = 0; iteration < 30; iteration++) {
                session.Toolbar.Hide();
                Application.DoEvents();
                firstPaint.Clear();
                allocated = GC.GetAllocatedBytesForCurrentThread();
                watch.Restart();
                session.Toolbar.Show();
                Application.DoEvents();
                watch.Stop();
                allocated = GC.GetAllocatedBytesForCurrentThread() - allocated;
                process.Refresh();
                cycles.Add(new { iteration, elapsedMs = watch.Elapsed.TotalMilliseconds, paints = new Dictionary<string, double>(firstPaint), allocatedBytes = allocated, handles = process.HandleCount, privateBytes = process.PrivateMemorySize64 });
                if (firstPaint.Count != 4) throw new Exception("Repeated toolbar display missed a row");
            }
            File.WriteAllText(Path.Combine(output, "repeat-show.json"), JsonSerializer.Serialize(cycles));
            canvas.Window.Close();
        }
        using var parent = new Form { ShowInTaskbar = false };
        var dark = Environment.GetEnvironmentVariable("WINONE_PROBE_THEME") == "dark";
        using var toolbar = new NativeToolbarControl(dark, Environment.GetEnvironmentVariable("WINONE_PROBE_LANGUAGE"));
        string? selectedCommand = null;
        ShareX.ScreenCaptureLib.WinOneToolStyle? selectedStyle = null;
        toolbar.Command += command => selectedCommand = command;
        toolbar.ToolStyleChanged += style => selectedStyle = style;
        parent.Controls.Add(toolbar);
        toolbar.CreateControl(); toolbar.SetState(true, false, "rectangle", false);
        toolbar.SetToolStyle("rectangle", new ShareX.ScreenCaptureLib.WinOneToolStyle(Color.Red, 3), false);
        var sizes = new[] { toolbar.PreferredToolbarSize, new Size(320, toolbar.PreferredToolbarSize.Height) };
        parent.ClientSize = sizes[0]; parent.Show();
        foreach (var size in sizes) {
            toolbar.Size = size; toolbar.PerformLayout();
            parent.ClientSize = size; parent.Update();
            var drawingRow = toolbar.Controls["capture-tools"]!;
            var actionRow = toolbar.Controls["capture-actions"]!;
            // The toolbar must leave the capture visible and separate the
            // drawing group from completion actions, including at 320px.
            var firstAction = ((ToolStrip)actionRow).Items.OfType<ToolStripButton>().Where(item => item.Placement == ToolStripItemPlacement.Main).Min(item => item.Bounds.Left);
            var lastTool = ((ToolStrip)drawingRow).Items.Cast<ToolStripItem>().Where(item => item.Placement == ToolStripItemPlacement.Main).Max(item => item.Bounds.Right);
            if (drawingRow.Top != actionRow.Top || drawingRow.Height != actionRow.Height || toolbar.Height > 60d * toolbar.DeviceDpi / 96 || actionRow.Left + firstAction - drawingRow.Left - lastTool < 28d * toolbar.DeviceDpi / 96)
                throw new Exception("Toolbar must be one compact row with a visible gap between function groups");
            foreach (var strip in toolbar.Controls.OfType<ToolStrip>()) {
                if (size == sizes[0] && strip.Name is "capture-tools" or "capture-actions") {
                    var right = strip.Items.Cast<ToolStripItem>().Where(item => item.Available && item.Placement == ToolStripItemPlacement.Main).Max(item => item.Bounds.Right);
                    if (strip.ClientSize.Width - right > 36d * toolbar.DeviceDpi / 96)
                        throw new Exception("Preferred toolbar size leaves an empty stretch after " + strip.Name + ": " + (strip.ClientSize.Width - right));
                }
                foreach (var button in strip.Items.Cast<ToolStripItem>().Where(item => item.Available && item is ToolStripButton or ToolStripDropDownButton)) {
                    if (button.Placement == ToolStripItemPlacement.Main && !strip.ClientRectangle.Contains(button.Bounds))
                        throw new Exception("Toolbar clipped an action: " + button.Name);
                    if (button.Placement == ToolStripItemPlacement.None)
                        throw new Exception("Toolbar lost an action: " + button.Name);
                }
            }
            var handle = toolbar.Controls["capture-grip"]!;
            var row = toolbar.Controls["capture-actions"]!;
            if (handle.Right > row.Left || Math.Abs(handle.Top + handle.Height / 2d - toolbar.Height / 2d) > 1)
                throw new Exception("Drag handle is not at the left center of the whole toolbar");
            // Text actions must remain reachable through the existing native
            // overflow menu when the row cannot fit every default tool.
            foreach (var action in new[] { "ocr", "translate" }) {
                var toolRow = toolbar.Controls.OfType<ToolStrip>().Single(strip => strip.Name == "capture-tools");
                var button = toolRow.Items[action] ?? throw new Exception("Missing text action: " + action);
                selectedCommand = null;
                if (button.Placement == ToolStripItemPlacement.Overflow) {
                    toolRow.OverflowButton.ShowDropDown();
                    var entry = toolRow.OverflowButton.DropDown.Items.Cast<ToolStripItem>().Single(item => item.Name == action);
                    if (!entry.Enabled) throw new Exception("Overflow disabled a text action: " + action);
                    entry.PerformClick();
                    toolRow.OverflowButton.DropDown.Close();
                } else button.PerformClick();
                if (selectedCommand != action) throw new Exception("Text action was unreachable: " + action);
            }
            using var image = new Bitmap(size.Width, size.Height);
            toolbar.DrawToBitmap(image, new Rectangle(Point.Empty, size));
            image.Save(Path.Combine(output, $"toolbar-{size.Width}.png"));
            // A separator with the background color is still present in the
            // control tree. Inspect rendered pixels so that regression fails.
            foreach (var strip in toolbar.Controls.OfType<ToolStrip>()) {
                foreach (var separator in strip.Items.OfType<ToolStripSeparator>().Where(item => item.Placement == ToolStripItemPlacement.Main)) {
                    var bounds = separator.Bounds;
                    var visiblePixels = 0;
                    for (var x = bounds.Left; x < bounds.Right; x++)
                        for (var y = bounds.Top + 3; y < bounds.Bottom - 3; y++) {
                            var pixel = image.GetPixel(strip.Left + x, strip.Top + y);
                            if (Math.Abs(pixel.R - strip.BackColor.R) >= 20) visiblePixels++;
                        }
                    if (visiblePixels < 10d * toolbar.DeviceDpi / 96)
                        throw new Exception("Function group separator is invisible: " + strip.Name);
                }
            }
            if (size.Width == 320) {
                foreach (var tools in toolbar.Controls.OfType<ToolStrip>().Where(strip => strip.Name is "capture-actions" or "capture-tools")) {
                tools.OverflowButton.ShowDropDown();
                var menu = tools.OverflowButton.DropDown; menu.Update();
                using var menuImage = new Bitmap(menu.Width, menu.Height);
                menu.DrawToBitmap(menuImage, new Rectangle(Point.Empty, menu.Size));
                menuImage.Save(Path.Combine(output, tools.Name == "capture-tools" ? "overflow.png" : "actions-overflow.png"));
                var entries = menu.Items.OfType<ToolStripMenuItem>().ToArray();
                if (entries.Length == 0 || entries.Zip(entries.Skip(1)).Any(pair => pair.First.Bounds.Bottom > pair.Second.Bounds.Top))
                    throw new Exception("Overflow entries are not a clear single-column list");
                entries[0].PerformClick();
                if (selectedCommand != entries[0].Name) throw new Exception("Overflow action did not reach the original command");
                menu.Close();
                }
            }
        }
        var retake = toolbar.Controls.OfType<ToolStrip>().SelectMany(strip => strip.Items.Cast<ToolStripItem>()).Single(item => item.Name=="retake");
        if(retake.Available)throw new Exception("Normal capture exposed scrolling retake");
        toolbar.SetRetakeAvailable(true);toolbar.SetState(true,false,"rectangle",false);
        foreach(var width in new[]{320,toolbar.PreferredToolbarSize.Width}) {
            toolbar.Size=new Size(width,toolbar.PreferredToolbarSize.Height);parent.ClientSize=toolbar.Size;toolbar.PerformLayout();parent.Update();
            foreach(var strip in toolbar.Controls.OfType<ToolStrip>())foreach(var item in strip.Items.Cast<ToolStripItem>().Where(item=>item.Available&&item is ToolStripButton or ToolStripDropDownButton)) {
                if(item.Placement==ToolStripItemPlacement.None||item.Placement==ToolStripItemPlacement.Main&&!strip.ClientRectangle.Contains(item.Bounds))throw new Exception("Scrolling toolbar clipped an action: "+item.Name);
            }
            selectedCommand=null;
            if(retake.Placement==ToolStripItemPlacement.Overflow) {
                var row=toolbar.Controls.OfType<ToolStrip>().Single(strip=>strip.Name=="capture-actions");
                row.OverflowButton.ShowDropDown();
                row.OverflowButton.DropDown.Items.Cast<ToolStripItem>().Single(item=>item.Name=="retake").PerformClick();
                row.OverflowButton.DropDown.Close();
            } else retake.PerformClick();
            if(selectedCommand!="retake")throw new Exception("Scrolling retake was not reachable");
            using var image=new Bitmap(toolbar.Width,toolbar.Height);toolbar.DrawToBitmap(image,new Rectangle(Point.Empty,image.Size));
            image.Save(Path.Combine(output,$"toolbar-retake-{width}.png"));
        }
        toolbar.SetRetakeAvailable(false);
        var styleButton = toolbar.Controls.OfType<ToolStrip>().SelectMany(strip => strip.Items.Cast<ToolStripItem>()).OfType<NativeToolOptions>().Single();
        styleButton.ShowDropDown();
        var numeric = styleButton.DropDownItems.OfType<ToolStripControlHost>().Select(host => host.Control).OfType<NumericUpDown>().Single();
        numeric.Value = 7;
        if (selectedStyle?.Value != 7) throw new Exception("Native numeric input did not emit the style change");
        styleButton.DropDown.Update();
        using (var menuImage = new Bitmap(styleButton.DropDown.Width, styleButton.DropDown.Height)) {
            styleButton.DropDown.DrawToBitmap(menuImage, new Rectangle(Point.Empty, menuImage.Size));
            menuImage.Save(Path.Combine(output, "style-menu.png"));
        }
        styleButton.DropDown.Close();
        var monitors = new[] { new Rectangle(0, 0, 1920, 1080), new Rectangle(-1920, -200, 1920, 1080) };
        foreach (var monitor in monitors) {
            foreach (var point in new[] { monitor.Location, new Point(monitor.Right - 100, monitor.Bottom - 80), new Point(monitor.Left + 900, monitor.Top + 500) }) {
                var selection = new Rectangle(point, new Size(100, 80));
                var placed = ToolbarPlacement.Place(selection, monitor, sizes[0]);
                if (!monitor.Contains(placed)) throw new Exception("Toolbar escaped its monitor");
            }
        }
        File.WriteAllText(Path.Combine(output, "report.json"), JsonSerializer.Serialize(new { sizes, scrollingRetake=true, monitorCases = 6, scope = "Native control render and monitor placement geometry; no live monitor/DPI interaction" }));
        return 0;
    }
}
