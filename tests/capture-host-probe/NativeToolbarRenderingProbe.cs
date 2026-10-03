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
            foreach (var strip in toolbar.Controls.OfType<ToolStrip>()) {
                foreach (var button in strip.Items.Cast<ToolStripItem>().Where(item => item is ToolStripButton or ToolStripDropDownButton)) {
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
            using var image = new Bitmap(size.Width, size.Height);
            toolbar.DrawToBitmap(image, new Rectangle(Point.Empty, size));
            image.Save(Path.Combine(output, $"toolbar-{size.Width}.png"));
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
        File.WriteAllText(Path.Combine(output, "report.json"), JsonSerializer.Serialize(new { sizes, monitorCases = 6, scope = "Native control render and monitor placement geometry; no live monitor/DPI interaction" }));
        return 0;
    }
}
