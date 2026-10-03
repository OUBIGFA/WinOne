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
            ("default", null, new[] { "region", "rectangle", "ellipse", "arrow", "pen", "cover", "mosaic", "blur", "text" }),
            ("empty", Array.Empty<string>(), new[] { "rectangle" }),
            ("subset", new[] { "mosaic", "crop" }, new[] { "region", "rectangle", "mosaic" }),
            ("legacy-move", new[] { "select" }, new[] { "rectangle" }),
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
            var actual = row.Items.OfType<ToolStripButton>().Where(button => button.Placement == ToolStripItemPlacement.Main).Select(button => button.Name).ToArray();
            if (!actual.Order().SequenceEqual(item.visible.Order())) throw new Exception(item.name + " ignored stored toolbar preferences");
            if (row.Items.Cast<ToolStripItem>().Any(button => button.Name == "select")) throw new Exception("Legacy preference restored the removed move tool");
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
