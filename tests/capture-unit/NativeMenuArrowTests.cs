using WinOne.Capture.NativeUI;

internal static class NativeMenuArrowTests
{
    // Render only to owned bitmaps. No window, system setting or desktop input
    // is required to catch a renderer falling back to the filled OS triangle.
    public static void Run()
    {
        Exception? failure = null;
        var thread = new Thread(() => {
            try {
                foreach (var dark in new[] { false, true }) {
                    using var theme = new NativeMenuTheme(dark);
                    using var toolbar = new NativeToolbarControl(dark, "en-US");
                    using var menu = new ContextMenuStrip();
                    using var entry = new ToolStripMenuItem("Submenu");
                    menu.Items.Add(entry);
                    theme.Apply(menu);
                    var renderers = new[] { menu.Renderer }.Concat(toolbar.Controls.OfType<ToolStrip>()
                        .Where(row => row.Name != "capture-grip").Select(row => row.Renderer));
                    foreach (var renderer in renderers) {
                        foreach (var enabled in new[] { true, false }) {
                            entry.Enabled = enabled;
                            foreach (var direction in Enum.GetValues<ArrowDirection>()) {
                                using var image = new Bitmap(32, 32);
                                using var graphics = Graphics.FromImage(image);
                                renderer.DrawArrow(new ToolStripArrowRenderEventArgs(graphics, entry,
                                    new Rectangle(8, 8, 16, 16), theme.Foreground, direction));
                                // The official chevron is open at its centre;
                                // the old triangle fills this interior pixel.
                                if (image.GetPixel(16, 16).A > 32) throw new Exception($"Filled submenu arrow: {direction}");
                                var pixels = 0;
                                for (var y = 0; y < image.Height; y++)
                                    for (var x = 0; x < image.Width; x++)
                                        if (image.GetPixel(x, y).A > 32) pixels++;
                                if (pixels < 8 || pixels > 70) throw new Exception($"Missing or filled outline: {direction}, {pixels} pixels");
                                var target = direction switch {
                                    ArrowDirection.Right => new Point(18, 16),
                                    ArrowDirection.Left => new Point(13, 16),
                                    ArrowDirection.Down => new Point(16, 18),
                                    _ => new Point(16, 13),
                                };
                                // Rotating an even-sized raster moves the tip by
                                // half a pixel. Check its one-pixel neighbourhood
                                // rather than reject a correctly oriented glyph.
                                var tipVisible = Enumerable.Range(target.Y - 1, 3).Any(y =>
                                    Enumerable.Range(target.X - 1, 3).Any(x => image.GetPixel(x, y).A > 32));
                                if (!tipVisible) {
                                    var ink = string.Join("; ", Enumerable.Range(12, 8).Select(y => $"{y}:" + string.Join(",", Enumerable.Range(8, 16).Where(x => image.GetPixel(x, y).A > 32))));
                                    throw new Exception($"Wrong chevron direction: {direction}, ink {ink}");
                                }
                            }
                        }
                    }
                }
            } catch (Exception error) { failure = error; }
        });
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        if (!thread.Join(TimeSpan.FromSeconds(10))) throw new Exception("Menu outline rendering timed out");
        if (failure is not null) throw failure;
        Console.WriteLine("PASS: menu and toolbar outline arrows, four directions, light/dark and disabled states");
    }
}
