using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text.Json;
using ShareX.HelpersLib;
using ShareX.ScreenCaptureLib;
using WinOne.Capture;
using WinOne.Capture.NativeUI;

// Repeated captures must not keep top-level windows or USER objects: the
// capture process lives as long as the Host, and windows it leaks fill the
// desktop until every window snapshot is slow or fails. Runs the production
// workflow on a private desktop, quick and with the editor, then compares
// the process's windows after warm-up with those after many captures.
internal static class WorkflowWindowsProbe
{
    private delegate bool EnumProc(IntPtr window, IntPtr parameter);
    [DllImport("user32.dll")] private static extern bool EnumWindows(EnumProc proc, IntPtr parameter);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window, out uint process);
    [DllImport("user32.dll")] private static extern uint GetGuiResources(IntPtr process, uint kind);

    private static int OwnWindows()
    {
        var count = 0;
        var self = (uint)Environment.ProcessId;
        EnumWindows((window, _) => { GetWindowThreadProcessId(window, out var owner); if (owner == self) count++; return true; }, IntPtr.Zero);
        return count;
    }

    public static int Run(string directory)
    {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private desktop");
        Directory.CreateDirectory(directory);
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        var bounds = CaptureHelpers.GetScreenBounds();
        CapturedFrame Acquire() => CapturedFrame.Own(new Bitmap(bounds.Width, bounds.Height), bounds);
        var settings = new SettingsStore(directory);
        var operations = new CaptureOperations(settings, directory, _ => { }, (_, _) => null);
        using var process = Process.GetCurrentProcess();
        var report = new List<object>();
        var failures = new List<string>();
        foreach (var editor in new[] { false, true })
        {
            settings.Save(settings.Snapshot() with { OutputDirectory = Path.Combine(directory, "images"), OpenEditor = editor, CopyToClipboard = false });
            void Capture()
            {
                try
                {
                    NativeCaptureWorkflow.Run(settings, operations, "window", null, CancellationToken.None, Acquire,
                        () => Array.Empty<WindowCaptureTarget>(), (canvas, session) => {
                            canvas.Window.Shown += (_, _) => canvas.Window.BeginInvoke((Action)(() => {
                                canvas.SetRegion(new Rectangle(100, 100, 240, 140));
                                // Let the toolbar and its menus appear, then cancel.
                                canvas.Window.Refresh();
                                canvas.Window.BeginInvoke(canvas.Window.Close);
                            }));
                            canvas.ShowDialog();
                        }).GetAwaiter().GetResult();
                }
                catch (OperationCanceledException) { }
                catch (InvalidOperationException) { }
                Application.DoEvents();
                if (operations.GetDraft() is { } draft) operations.CancelDraft(draft.Id, draft.Revision);
            }
            for (var i = 0; i < 3; i++) Capture();
            GC.Collect(); GC.WaitForPendingFinalizers(); Application.DoEvents();
            var windowsBefore = OwnWindows(); var userBefore = GetGuiResources(process.Handle, 1);
            for (var i = 0; i < 30; i++) Capture();
            GC.Collect(); GC.WaitForPendingFinalizers(); Application.DoEvents();
            var windowsAfter = OwnWindows(); var userAfter = GetGuiResources(process.Handle, 1);
            report.Add(new { editor, windowsBefore, windowsAfter, userBefore, userAfter });
            // A few objects may come and go (tooltips, IME); 30 captures
            // keeping one window each would add 30.
            if (windowsAfter > windowsBefore + 2) failures.Add($"editor={editor}: top-level windows {windowsBefore} -> {windowsAfter}");
            if (userAfter > userBefore + 10) failures.Add($"editor={editor}: USER objects {userBefore} -> {userAfter}");
        }
        File.WriteAllText(Path.Combine(directory, "windows.json"), JsonSerializer.Serialize(new { report, failures }));
        return failures.Count == 0 ? 0 : 1;
    }

}
