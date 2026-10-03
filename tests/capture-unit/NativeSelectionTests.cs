using WinOne.Capture;

internal static class NativeSelectionTests
{
    // Deliver activation messages without showing or interacting with the desktop.
    public static void HandoffMessages()
    {
        Exception? failure = null;
        var thread = new Thread(() => {
            try {
                using var frame = CapturedFrame.Own(new Bitmap(240, 180), new Rectangle(40, 40, 240, 180));
                foreach (var windowMode in new[] { false, true }) {
                    using var selector = new Harness(frame, windows: windowMode ? [new Rectangle(50, 50, 70, 80)] : null, directEdit: true);
                    typeof(NativeSelectionWindow).GetField("shown", System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.NonPublic)!.SetValue(selector, true);
                    selector.BeforeEditorHandoff = _ => selector.LoseFocus();
                    if (windowMode) selector.SelectAt(new Point(20, 20));
                    else selector.Drag(new Point(10, 10), new Point(80, 90));
                    if (selector.DialogResult != DialogResult.OK || selector.Selection != new Rectangle(50, 50, 70, 80)) throw new Exception("editor handoff lost the confirmed selection on deactivation");
                }
                using (var selector = new Harness(frame, directEdit: true)) {
                    var attempts = 0;
                    selector.BeforeEditorHandoff = _ => {
                        attempts++;
                        if (attempts == 1) selector.Key(Keys.Enter);
                        throw new InvalidOperationException("Editor unavailable; try again");
                    };
                    selector.Drag(new Point(10, 10), new Point(80, 90));
                    if (attempts != 1) throw new Exception("reentrant confirmation started another handoff");
                    if (selector.DialogResult != DialogResult.None || selector.Selection.IsEmpty || selector.AccessibleDescription != "Editor unavailable; try again") throw new Exception("handoff failure discarded selection or hid feedback");
                    selector.BeforeEditorHandoff = _ => { attempts++; };
                    selector.Key(Keys.Enter);
                    if (attempts != 2 || selector.DialogResult != DialogResult.OK || !selector.EditRequested) throw new Exception("failed handoff could not be retried");
                }
                using (var selector = new Harness(frame, directEdit: true)) {
                    typeof(NativeSelectionWindow).GetField("shown", System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.NonPublic)!.SetValue(selector, true);
                    selector.BeforeEditorHandoff = _ => selector.DisplayChanged();
                    selector.Drag(new Point(10, 10), new Point(80, 90));
                    if (selector.DialogResult != DialogResult.Cancel || !selector.Selection.IsEmpty) throw new Exception("display cancellation was overwritten by editor handoff");
                }
            } catch (Exception error) { failure = error; }
        });
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        if (!thread.Join(TimeSpan.FromSeconds(10))) throw new Exception("handoff message test timed out");
        if (failure is not null) throw failure;
    }
    private sealed class Harness : NativeSelectionWindow
    {
        public Harness(CapturedFrame frame, IReadOnlyList<Rectangle>? windows = null, Func<NativeSelectionAction, Rectangle, bool>? execute = null, bool quickSave = false, bool directEdit = false, bool directConfirm = false) : base(frame, "en-US", windows: windows, executeAction: execute, quickSave: quickSave, directEdit: directEdit, directConfirm: directConfirm) { }
        public bool ToolbarVisible => Controls.OfType<Panel>().Any(panel => panel.Visible);
        public void DoubleClickAt(Point point)
        {
            SelectAt(point);
            OnMouseDown(new MouseEventArgs(MouseButtons.Left, 2, point.X, point.Y, 0));
            OnMouseDoubleClick(new MouseEventArgs(MouseButtons.Left, 2, point.X, point.Y, 0));
        }
        public void Action(string name) => ((Button)Controls.Find("selection-" + name, true).Single()).PerformClick();
        public void LoseFocus() => OnDeactivate(EventArgs.Empty);
        public void Hover(Point point) => OnMouseMove(new MouseEventArgs(MouseButtons.None, 0, point.X, point.Y, 0));
        public void SelectAt(Point point) => Drag(point, point);
        public void Drag(Point start, Point end)
        {
            OnMouseDown(new MouseEventArgs(MouseButtons.Left, 1, start.X, start.Y, 0));
            OnMouseMove(new MouseEventArgs(MouseButtons.Left, 0, end.X, end.Y, 0));
            OnMouseUp(new MouseEventArgs(MouseButtons.Left, 1, end.X, end.Y, 0));
        }
        public void Key(Keys key) => OnKeyDown(new KeyEventArgs(key));
        public void Edit() => ((Button)Controls.Find("selection-edit", true).Single()).PerformClick();
        public void DisplayChanged()
        {
            var message = Message.Create(Handle, 0x007E, IntPtr.Zero, IntPtr.Zero);
            WndProc(ref message);
        }
    }

    public static void RunOnSta(string? previewPath = null)
    {
        Exception? failure = null;
        var thread = new Thread(() =>
        {
            try
            {
                Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
                Application.EnableVisualStyles();
                var image = new Bitmap(240, 180);
                using (var drawing = Graphics.FromImage(image)) drawing.Clear(Color.SlateBlue);
                image.SetPixel(70, 80, Color.Coral);
                using var frame = CapturedFrame.Own(image, new Rectangle(40, 40, 240, 180));
                using (var scrolling = new Harness(frame, directConfirm: true))
                {
                    scrolling.Show(); Application.DoEvents();
                    scrolling.Drag(new Point(10, 10), new Point(80, 90));
                    if (scrolling.Controls.Count != 0 || scrolling.DialogResult != DialogResult.OK || scrolling.EditRequested) throw new Exception("scrolling selection retained native actions");
                }
                foreach (var windowMode in new[] { false, true })
                {
                    using var direct = new Harness(frame, windows: windowMode ? [new Rectangle(50, 50, 70, 80)] : null, directEdit: true);
                    direct.Show(); Application.DoEvents();
                    if (direct.Controls.Count != 0) throw new Exception("direct editor path constructed native controls");
                    if (windowMode) direct.SelectAt(new Point(20, 20));
                    else direct.Drag(new Point(10, 10), new Point(80, 90));
                    if (direct.DialogResult != DialogResult.OK || !direct.EditRequested || direct.Selection != new Rectangle(50, 50, 70, 80)) throw new Exception("selection did not hand off directly to the editor");
                }
                var attempts = 0;
                using (var window = new Harness(frame, execute: (action, selection) =>
                {
                    if (action != NativeSelectionAction.Copy || selection != new Rectangle(50, 50, 70, 80)) throw new Exception("direct action changed selected coordinates");
                    if (++attempts == 1) throw new InvalidOperationException("Clipboard unavailable");
                    return true;
                }))
                {
                    window.Show(); Application.DoEvents();
                    window.Drag(new Point(10, 10), new Point(80, 90));
                    window.Action("copy");
                    if (window.DialogResult != DialogResult.None || window.Selection.IsEmpty) throw new Exception("failed action discarded selection");
                    if (window.AccessibleDescription != "Clipboard unavailable") throw new Exception("action failure was not exposed");
                    window.Key(Keys.Control | Keys.C);
                    if (window.DialogResult != DialogResult.OK || attempts != 2) throw new Exception("direct action retry failed");
                }
                Harness? saveWindow = null;
                using (saveWindow = new Harness(frame, execute: (action, _) => { saveWindow!.LoseFocus(); return false; }))
                {
                    saveWindow.Show(); Application.DoEvents();
                    saveWindow.Drag(new Point(10, 10), new Point(80, 90));
                    saveWindow.Action("save");
                    if (saveWindow.DialogResult != DialogResult.None || saveWindow.Selection.IsEmpty) throw new Exception("cancelled save discarded selection");
                    saveWindow.Key(Keys.Escape);
                }
                foreach (var action in new[] { NativeSelectionAction.Save, NativeSelectionAction.Pin })
                {
                    var called = false;
                    using var direct = new Harness(frame, execute: (received, _) => { called = received == action; return called; });
                    direct.Show(); Application.DoEvents();
                    direct.Drag(new Point(10, 10), new Point(80, 90));
                    if (action == NativeSelectionAction.Save) direct.Key(Keys.Control | Keys.S);
                    else direct.Action("pin");
                    if (!called || direct.DialogResult != DialogResult.OK) throw new Exception("direct action did not complete");
                }
                using (var window = new Harness(frame))
                {
                    window.Show(); Application.DoEvents();
                    window.Key(Keys.Enter);
                    if (window.DialogResult != DialogResult.None) throw new Exception("empty selection was confirmed");
                    window.Drag(new Point(100, 110), new Point(50, 60));
                    if (window.Selection != new Rectangle(90, 100, 50, 50)) throw new Exception("window input used the wrong coordinate space");
                    window.Key(Keys.Enter);
                    if (window.DialogResult != DialogResult.OK) throw new Exception("selection did not confirm");
                    using var crop = frame.Crop(window.Selection);
                    if (crop.GetPixel(20, 20).ToArgb() != Color.Coral.ToArgb()) throw new Exception("selection painting changed source pixels");
                }
                using (var window = new Harness(frame))
                {
                    window.Show(); Application.DoEvents();
                    window.Drag(new Point(10, 10), new Point(80, 90));
                    window.Edit();
                    if (window.DialogResult != DialogResult.OK || !window.EditRequested) throw new Exception("toolbar did not request the unified editor");
                }
                using (var window = new Harness(frame))
                {
                    window.Show(); Application.DoEvents();
                    window.Drag(new Point(10, 10), new Point(80, 90));
                    window.Key(Keys.Escape);
                    if (window.DialogResult != DialogResult.Cancel || !window.Selection.IsEmpty) throw new Exception("cancel retained a selection");
                }
                using (var window = new Harness(frame))
                {
                    window.Show(); Application.DoEvents();
                    window.DisplayChanged();
                    if (window.DialogResult != DialogResult.Cancel || window.CancellationReason != "display_changed") throw new Exception("display change retained stale coordinates");
                }
                using (var window = new Harness(frame, [new Rectangle(60, 60, 100, 80), new Rectangle(40, 40, 200, 160)]))
                {
                    window.Show(); Application.DoEvents();
                    window.Hover(new Point(40, 40));
                    if (window.Selection != new Rectangle(60, 60, 100, 80)) throw new Exception("hover did not choose the front window");
                    window.SelectAt(new Point(40, 40));
                    window.Hover(new Point(180, 140));
                    if (window.Selection != new Rectangle(60, 60, 100, 80)) throw new Exception("moving to the toolbar changed the selected window");
                    window.Edit();
                    if (!window.EditRequested || window.DialogResult != DialogResult.OK) throw new Exception("selected window did not enter editing");
                }
                using (var back = new Form { Bounds = new Rectangle(60, 60, 200, 140), StartPosition = FormStartPosition.Manual, FormBorderStyle = FormBorderStyle.None, ShowInTaskbar = false, TopMost = true })
                using (var front = new Form { Bounds = new Rectangle(90, 90, 160, 100), StartPosition = FormStartPosition.Manual, FormBorderStyle = FormBorderStyle.None, ShowInTaskbar = false, TopMost = true })
                {
                    back.Show(); front.Show(); front.BringToFront(); Application.DoEvents();
                    var targets = WindowCaptureTargets.Snapshot().ToList();
                    var frontIndex = targets.FindIndex(target => target.Handle == front.Handle);
                    var backIndex = targets.FindIndex(target => target.Handle == back.Handle);
                    if (frontIndex < 0 || backIndex <= frontIndex) throw new Exception("native window targets lost Z order");
                    front.WindowState = FormWindowState.Minimized; Application.DoEvents();
                    if (WindowCaptureTargets.Snapshot().Any(target => target.Handle == front.Handle)) throw new Exception("minimized window remained selectable");
                }
                using (var window = new Harness(frame, [new Rectangle(60, 60, 100, 80)]))
                {
                    window.Show(); Application.DoEvents();
                    window.Drag(new Point(40, 40), new Point(40 + Math.Max(0, (SystemInformation.DragSize.Width - 1) / 2), 40 + Math.Max(0, (SystemInformation.DragSize.Height - 1) / 2)));
                    if (window.Selection != new Rectangle(60, 60, 100, 80)) throw new Exception("click jitter became an area drag");
                    window.Drag(new Point(120, 60), new Point(150, 60));
                    if (window.Selection != new Rectangle(60, 60, 130, 80)) throw new Exception("window right edge did not resize");
                    window.Drag(new Point(20, 20), new Point(10, 10));
                    if (window.Selection != new Rectangle(50, 50, 140, 90)) throw new Exception("window corner did not resize");
                    window.Key(Keys.Escape);
                }
                using (var window = new Harness(frame, [new Rectangle(60, 60, 100, 80)]))
                {
                    window.Show(); Application.DoEvents();
                    window.Drag(new Point(40, 40), new Point(180, 140));
                    if (window.Selection != new Rectangle(80, 80, 140, 100)) throw new Exception("window mode did not allow an area drag");
                    window.Drag(new Point(180, 90), new Point(200, 90));
                    if (window.Selection != new Rectangle(80, 80, 160, 100)) throw new Exception("area right edge did not resize");
                    window.Key(Keys.Enter);
                    if (window.DialogResult != DialogResult.OK) throw new Exception("resized area could not confirm");
                }
                var quickAttempts = 0;
                using (var quick = new Harness(frame, execute: (action, _) => { if (++quickAttempts == 1) throw new IOException("Test save failure"); return action == NativeSelectionAction.Save; }, quickSave: true))
                {
                    quick.Show(); Application.DoEvents();
                    quick.Drag(new Point(20, 20), new Point(90, 90));
                    if (quick.ToolbarVisible) throw new Exception("quick save displayed a toolbar");
                    quick.Key(Keys.Enter);
                    if (quick.DialogResult != DialogResult.None || quick.Selection.IsEmpty || quick.ToolbarVisible) throw new Exception("quick save failure discarded selection");
                    quick.Key(Keys.Enter);
                    if (quick.DialogResult != DialogResult.OK) throw new Exception("quick save did not confirm");
                }
                Rectangle copiedRegion = Rectangle.Empty;
                using (var edit = new Harness(frame, [new Rectangle(40, 40, 240, 180)], (action, region) => { if (action != NativeSelectionAction.SaveAndCopy) return false; copiedRegion = region; return true; }))
                {
                    edit.Show(); Application.DoEvents();
                    edit.Drag(new Point(20, 20), new Point(90, 90));
                    edit.DoubleClickAt(new Point(50, 50));
                    if (edit.DialogResult != DialogResult.OK || copiedRegion != new Rectangle(60, 60, 70, 70)) throw new Exception("double click changed the selected area or missed save-copy");
                }
                if (previewPath is not null)
                {
                    var previewImage = new Bitmap(900, 500);
                    using (var graphics = Graphics.FromImage(previewImage))
                    {
                        graphics.Clear(Color.FromArgb(226, 232, 240));
                        graphics.FillRectangle(Brushes.WhiteSmoke, 20, 20, 700, 360);
                        graphics.DrawRectangle(Pens.Gray, 20, 20, 700, 360);
                        graphics.FillRectangle(Brushes.White, 120, 80, 400, 250);
                        graphics.DrawRectangle(Pens.Gray, 120, 80, 400, 250);
                        using var labelFont = new Font("Segoe UI", 14, FontStyle.Regular, GraphicsUnit.Pixel);
                        graphics.DrawString("Window A", labelFont, Brushes.Black, 140, 100);
                    }
                    using var previewFrame = CapturedFrame.Own(previewImage, new Rectangle(40, 40, 900, 500));
                    using var window = new Harness(previewFrame, execute: (_, _) => false);
                    window.Show(); Application.DoEvents();
                    window.Drag(new Point(140, 100), new Point(650, 330));
                    Application.DoEvents();
                    using var rendered = new Bitmap(window.Width, window.Height);
                    window.DrawToBitmap(rendered, new Rectangle(Point.Empty, rendered.Size));
                    rendered.Save(previewPath, System.Drawing.Imaging.ImageFormat.Png);
                    window.Key(Keys.Escape);
                    using var windowPicker = new Harness(previewFrame, [new Rectangle(160, 120, 400, 250), new Rectangle(60, 60, 700, 360)]);
                    windowPicker.Show(); Application.DoEvents();
                    windowPicker.SelectAt(new Point(300, 200)); Application.DoEvents();
                    using var windowPreview = new Bitmap(windowPicker.Width, windowPicker.Height);
                    windowPicker.DrawToBitmap(windowPreview, new Rectangle(Point.Empty, windowPreview.Size));
                    windowPreview.Save(Path.Combine(Path.GetDirectoryName(previewPath)!, "native-window-selection.png"), System.Drawing.Imaging.ImageFormat.Png);
                    windowPicker.Key(Keys.Escape);
                }
            }
            catch (Exception error) { failure = error; }
        });
        thread.SetApartmentState(ApartmentState.STA);
        thread.Start(); thread.Join();
        if (failure is not null) throw new Exception("Native selection test failed", failure);
    }
}
