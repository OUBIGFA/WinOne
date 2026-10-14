using System.Runtime.InteropServices;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

internal static class NativeSaveCloseTests
{
    [DllImport("user32.dll")] private static extern IntPtr SendMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);

    public static void Run(string root)
    {
        foreach (var scenario in new[] { "save", "saveAs", "cancel-picker", "failed-save", "retry-close" }) {
            var directory = Path.Combine(root, scenario); Directory.CreateDirectory(directory);
            var outputDirectory = Path.Combine(directory, "images");
            if (scenario == "failed-save") File.WriteAllText(outputDirectory, "owned output blocker");
            var settings = new SettingsStore(directory);
            settings.Save(settings.Snapshot() with { OutputDirectory = outputDirectory, CopyToClipboard = false, OpenEditor = true, AskWhereToSave = false });
            var pickerCalls = 0; var sawFailure = false; var closeCalls = 0;
            var operations = new CaptureOperations(settings, directory,
                _ => throw new Exception("Saving unexpectedly copied to the system clipboard"),
                (_, _) => { pickerCalls++; return scenario == "cancel-picker" ? null : Path.Combine(directory, "chosen.png"); });
            var bounds = SystemInformation.VirtualScreen;
            var saved = (CaptureOutput)NativeCaptureWorkflow.Run(settings, operations, "region", null, CancellationToken.None,
                () => CapturedFrame.Own(new Bitmap(bounds.Width, bounds.Height), bounds),
                show: (canvas, session) => {
                    var scheduled = false; var timedOut = false; Exception? failure = null;
                    ToolStripItem Button(string name) => session.Toolbar.Controls.OfType<ToolStrip>().SelectMany(strip => strip.Items.Cast<ToolStripItem>()).Single(item => item.Name == name);
                    void Check(Action action) { try { action(); } catch (Exception error) { failure = error; canvas.Window.Close(); } }
                    using var deadline = new System.Windows.Forms.Timer { Interval = 4000 };
                    deadline.Tick += (_, _) => { timedOut = true; canvas.Window.Close(); };
                    if (scenario == "retry-close") canvas.Window.FormClosing += (_, args) => {
                        closeCalls++;
                        if (closeCalls != 1) return;
                        args.Cancel = true;
                        canvas.Window.BeginInvoke((Action)(() => SendMessageW(canvas.Window.Handle, 0x0100, (IntPtr)Keys.Enter, IntPtr.Zero)));
                    };
                    session.Updated += () => {
                        if (scenario == "failed-save" && session.Error is not null && !sawFailure) {
                            sawFailure = true;
                            canvas.Window.BeginInvoke((Action)(() => Check(() => {
                                if (session.CloseRequested || canvas.Window.IsDisposed) throw new Exception("Failed save closed the screenshot");
                                File.Move(outputDirectory, Path.Combine(directory, "blocker.txt")); Button("save").PerformClick();
                            })));
                        }
                        if (!session.Ready || scheduled) return; scheduled = true;
                        canvas.Window.BeginInvoke((Action)(() => Check(() => {
                            canvas.SetRegion(new Rectangle(20, 30, 240, 140));
                            canvas.AddRectangle(new Rectangle(40, 50, 80, 50));
                            Button(scenario is "saveAs" or "cancel-picker" ? "saveAs" : "save").PerformClick();
                            if (scenario == "cancel-picker") canvas.Window.BeginInvoke((Action)(() => Check(() => {
                                if (canvas.Window.IsDisposed || session.CloseRequested || operations.Last is not null) throw new Exception("Cancelled picker closed or saved the screenshot");
                                Button("save").PerformClick();
                            })));
                        })));
                    };
                    deadline.Start(); canvas.ShowDialog(); deadline.Stop();
                    if (timedOut) throw new Exception("Successful " + scenario + " did not exit the screenshot");
                    if (failure is not null) throw failure;
                }).GetAwaiter().GetResult();
            if (!File.Exists(saved.Path) || saved.Width != 240 || saved.Height != 140 || saved.Copied) throw new Exception("Saved output changed: " + scenario);
            if (scenario == "failed-save" && !sawFailure) throw new Exception("Save failure was not exercised");
            if (scenario is "saveAs" or "cancel-picker" && pickerCalls != 1) throw new Exception("Save dialog opened repeatedly");
            if (scenario == "retry-close" && (closeCalls != 2 || Directory.GetFiles(outputDirectory, "*.png").Length != 1)) throw new Exception("Close retry repeated the file save");
        }
    }
}
