using System.Reflection;
using ShareX;
using ShareX.HelpersLib;

internal static class PinMenuTests
{
    public static void Run(string directory)
    {
        Exception? failure = null;
        var thread = new Thread(() =>
        {
            try
            {
                Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
                Directory.CreateDirectory(directory);
                foreach (var language in new[] { "en-US", "zh-CN" })
                {
                    ShareXResources.Theme = language == "en-US" ? ShareXTheme.DarkTheme : ShareXTheme.LightTheme;
                    PinToScreenForm.OpenManaged(new Bitmap(160, 96), new PinToScreenOptions { Shadow = false }, language);
                    var form = Application.OpenForms.OfType<PinToScreenForm>().Single();
                    Application.DoEvents();
                    var menu = form.ContextMenuStrip ?? throw new Exception("WinOne pin menu missing");
                    void CheckTheme(ToolStrip current)
                    {
                        if (!ReferenceEquals(current.Renderer, menu.Renderer) || current.Font != menu.Font || current.BackColor != menu.BackColor || current.ForeColor != menu.ForeColor)
                            throw new Exception("Pin submenu did not inherit the shared menu style");
                        foreach (ToolStripItem entry in current.Items)
                            if (entry is ToolStripMenuItem item && item.HasDropDownItems) CheckTheme(item.DropDown);
                    }
                    CheckTheme(menu);
                    menu.Show(form, new Point(10, 10)); Application.DoEvents();
                    using (var screenshot = new Bitmap(menu.Width, menu.Height))
                    {
                        menu.DrawToBitmap(screenshot, new Rectangle(Point.Empty, screenshot.Size));
                        screenshot.Save(Path.Combine(directory, $"pin-menu-{language}.png"));
                    }
                    menu.Close();
                    ToolStripMenuItem Item(string name) => (ToolStripMenuItem)menu.Items.Find(name, true).Single();
                    if (Item("pin-copy").Text != (language == "en-US" ? "Copy" : "复制")) throw new Exception("pin menu language mismatch");
                    Item("pin-scale-200").PerformClick();
                    if (form.ImageScale != 200) throw new Exception("pin scale action failed");
                    Item("pin-opacity-50").PerformClick();
                    if (form.ImageOpacity != 50) throw new Exception("pin opacity action failed");
                    Item("pin-collapse").PerformClick();
                    if (!form.Minimized) throw new Exception("pin collapse action failed");
                    Item("pin-reset").PerformClick();
                    if (form.Minimized || form.ImageScale != 100 || form.ImageOpacity != 100) throw new Exception("pin reset did not restore view");
                    Item("pin-topmost").PerformClick();
                    if (form.TopMost || form.Options.TopMost) throw new Exception("topmost toggle did not apply");
                    Item("pin-border").PerformClick();
                    if (form.Options.Border) throw new Exception("border toggle did not apply");
                    typeof(Control).GetMethod("OnMouseUp", BindingFlags.NonPublic | BindingFlags.Instance)!.Invoke(form, [new MouseEventArgs(MouseButtons.Right, 1, 10, 10, 0)]);
                    if (form.IsDisposed || PinToScreenForm.ActiveCount != 1) throw new Exception("right click closed the pin");
                    if (language == "en-US") Item("pin-close").PerformClick();
                    else typeof(Control).GetMethod("OnKeyDown", BindingFlags.NonPublic | BindingFlags.Instance)!.Invoke(form, [new KeyEventArgs(Keys.Escape)]);
                    if (PinToScreenForm.ActiveCount != 0 || !form.IsDisposed) throw new Exception("pin menu close leaked window");
                }
            }
            catch (Exception error) { failure = error; }
            finally { PinToScreenForm.CloseAll(); }
        });
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        if (!thread.Join(TimeSpan.FromSeconds(15))) throw new Exception("pin menu test timed out");
        if (failure is not null) throw failure;
        Console.WriteLine("PASS: bilingual pin menus, zoom, opacity, collapse/reset, right-click and close");
    }
}
