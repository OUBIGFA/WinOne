using WinOne.Capture;

internal static class WindowCaptureRegionTests
{
    public static void Run()
    {
        Exception? failure = null;
        var thread = new Thread(() =>
        {
            try
            {
                using var form = new Form { StartPosition = FormStartPosition.Manual, Location = new Point(80, 80), ClientSize = new Size(500, 300), Text = "WinOne region detection test" };
                using var pane = new Panel { Bounds = new Rectangle(20, 20, 220, 240) };
                using var control = new TextBox { Bounds = new Rectangle(30, 30, 120, 28) };
                using var clipped = new Panel { Bounds = new Rectangle(190, 100, 100, 50) };
                using var hidden = new Panel { Bounds = new Rectangle(280, 40, 100, 80), Visible = false };
                using var tree = new TreeView { Bounds = new Rectangle(270, 140, 200, 120) };
                var node = tree.Nodes.Add("Accessible region without HWND");
                pane.Controls.Add(control); pane.Controls.Add(clipped); form.Controls.Add(pane); form.Controls.Add(hidden);
                form.Controls.Add(tree);
                form.Shown += async (_, _) =>
                {
                try
                {
                var frame = ShareX.HelpersLib.CaptureHelpers.GetWindowRectangle(form.Handle);
                var regions = WindowCaptureTargets.Regions([new WindowCaptureTarget(form.Handle, frame)]);
                var textBounds = ShareX.HelpersLib.NativeMethods.GetWindowRect(control.Handle);
                var paneBounds = pane.RectangleToScreen(pane.ClientRectangle);
                if (SelectionGeometry.WindowAtPoint(regions, new Point(textBounds.Left + 5, textBounds.Top + 5), frame) != textBounds)
                    throw new Exception("nested text control was not selectable");
                var clippedBounds = Rectangle.Intersect(paneBounds, ShareX.HelpersLib.NativeMethods.GetWindowRect(clipped.Handle));
                if (!regions.Contains(clippedBounds) || clippedBounds.Width != 30) throw new Exception("child bounds escaped their parent");
                if (!regions.Contains(paneBounds) || !regions.Contains(form.RectangleToScreen(form.ClientRectangle)) || !regions.Contains(frame))
                    throw new Exception("pane, client area or outer frame was omitted");
                if (regions.Contains(hidden.RectangleToScreen(hidden.ClientRectangle))) throw new Exception("hidden panel became selectable");
                if (regions.Count != regions.Distinct().Count()) throw new Exception("identical regions were duplicated");
                var helper = Environment.GetEnvironmentVariable("WINONE_REGION_HELPER");
                if (!string.IsNullOrEmpty(helper))
                {
                    var elapsed = System.Diagnostics.Stopwatch.StartNew();
                    // Use a real Windows Forms message loop, like the selector,
                    // so provider callbacks are serviced while the client waits.
                    var accessible = await AccessibleWindowRegions.ReadAsync(new WindowCaptureTarget(form.Handle, frame), CancellationToken.None, helper);
                    var nodeBounds = tree.RectangleToScreen(node.Bounds);
                    if (!accessible.Any(rect => rect.IntersectsWith(nodeBounds) && rect.Width < tree.Width && rect.Height < tree.Height))
                        throw new Exception("accessibility did not expose a region without its own HWND");
                    Console.WriteLine($"PASS: accessibility helper returned {accessible.Count} regions including a HWND-less tree item in {elapsed.ElapsedMilliseconds} ms");
                }
                Console.WriteLine("PASS: nested controls, pane/client/frame regions, parent clipping and hidden controls");
                }
                catch (Exception error) { failure = error; }
                finally { form.Close(); }
                };
                Application.Run(form);
            }
            catch (Exception error) { failure = error; }
        });
        thread.SetApartmentState(ApartmentState.STA); thread.Start(); thread.Join();
        if (failure is not null) throw failure;
    }
}
