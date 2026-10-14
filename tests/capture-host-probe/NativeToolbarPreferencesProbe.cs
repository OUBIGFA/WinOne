using System.Text.Json;
using ShareX.ScreenCaptureLib;

internal static class NativeToolbarPreferencesProbe
{
    public static int Run(string output)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private desktop");
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        var cases = new (string name, string[]? preferences, string[] visible)[] {
            ("default", null, new[] { "select", "region", "scroll", "rectangle", "ellipse", "arrow", "pen", "cover", "mosaic", "blur", "text", "ocr", "translate" }),
            ("empty", Array.Empty<string>(), new[] { "scroll", "rectangle" }),
            ("subset", new[] { "mosaic", "crop" }, new[] { "region", "scroll", "rectangle", "mosaic" }),
            ("legacy-move", new[] { "select" }, new[] { "select", "scroll", "rectangle" }),
            ("text-actions", new[] { "ocr", "translate" }, new[] { "scroll", "rectangle", "ocr", "translate" }),
        };
        var records = new List<object>();
        foreach (var item in cases) {
            using var parent = new Form { ShowInTaskbar = false };
            using var toolbar = new NativeToolbarControl(language: "en-US", toolbarTools: item.preferences);
            parent.Controls.Add(toolbar);
            toolbar.SetState(false, false, "rectangle", false);
            toolbar.SetToolStyle("rectangle", new WinOneToolStyle(Color.Red, 3), false);
            toolbar.Size = toolbar.PreferredToolbarSize; parent.ClientSize = toolbar.Size;
            parent.Show(); toolbar.PerformLayout(); parent.Update();
            var row = (ToolStrip)toolbar.Controls["capture-tools"]!;
            // Scrolling stays available independently of annotation visibility.
            var scroll = row.Items["scroll"] ?? throw new Exception("Scrolling capture is missing");
            var region = row.Items["region"] ?? throw new Exception("Selection is missing");
            if (row.Items.IndexOf(scroll) != row.Items.IndexOf(region) + 1)
                throw new Exception("Scrolling capture is no longer beside selection");
            var actual = row.Items.OfType<ToolStripButton>().Where(button => button.Placement == ToolStripItemPlacement.Main).Select(button => button.Name).ToArray();
            if (!actual.Order().SequenceEqual(item.visible.Order())) throw new Exception(item.name + " ignored stored toolbar preferences");
            if (row.Items[0].Name != "select") throw new Exception("Move is not the first tool");
            if (item.preferences is not null) {
                string? selected = null;
                toolbar.Command += command => { selected = command; toolbar.SetState(false, false, command, false); };
                row.OverflowButton.ShowDropDown();
                var entry = row.OverflowButton.DropDown.Items.OfType<ToolStripMenuItem>().Single(button => button.Name == "blur");
                entry.PerformClick(); row.OverflowButton.HideDropDown(); toolbar.PerformLayout();
                var active = row.Items.OfType<ToolStripButton>().Single(button => button.Name == "blur");
                var old = row.Items.OfType<ToolStripButton>().Single(button => button.Name == "rectangle");
                if (selected != "blur" || active.Placement != ToolStripItemPlacement.Main || !active.Checked || old.Placement != ToolStripItemPlacement.Overflow)
                    throw new Exception("Selected menu tool did not become visible while the previous hidden tool returned to the menu");
                toolbar.Size = new Size(320, toolbar.Height); parent.ClientSize = toolbar.Size; toolbar.PerformLayout();
                if (active.Placement != ToolStripItemPlacement.Main || !row.ClientRectangle.Contains(active.Bounds)) throw new Exception("Narrow toolbar clipped the active tool");
            }
            using var image = new Bitmap(toolbar.Width, toolbar.Height);
            toolbar.DrawToBitmap(image, new Rectangle(Point.Empty, image.Size));
            image.Save(Path.Combine(output, item.name + ".png"));
            records.Add(new { item.name, visible = actual, preferences = item.preferences });
            parent.Close();
        }
        File.WriteAllText(Path.Combine(output, "report.json"), JsonSerializer.Serialize(records));
        Console.WriteLine("PASS: toolbar preferences, legacy mapping, menu selection and active tool visibility");
        return 0;
    }
}
