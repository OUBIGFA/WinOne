using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Diagnostics;
using System.Text;
using WinOne.Capture;

// A separate, never-activated desktop exercises ShowDialog and hold-window
// messages without covering user windows, moving the pointer or taking screenshots.
internal static class ModalHandoffTests
{
    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern IntPtr CreateDesktopW(string name, IntPtr device, IntPtr mode, uint flags, uint access, IntPtr security);
    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool CloseDesktop(IntPtr desktop);
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct StartupInfo {
        public int Size; public string? Reserved, Desktop, Title;
        public int X, Y, Width, Height, XChars, YChars, Fill, Flags;
        public short Show, ReservedSize; public IntPtr ReservedBytes, Input, Output, Error;
    }
    [StructLayout(LayoutKind.Sequential)]
    private struct ProcessInfo { public IntPtr Process, Thread; public int Id, ThreadId; }
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern bool CreateProcessW(string application, StringBuilder command, IntPtr processAttributes, IntPtr threadAttributes,
        bool inherit, uint flags, IntPtr environment, string? directory, ref StartupInfo startup, out ProcessInfo process);
    [DllImport("kernel32.dll")]
    private static extern bool CloseHandle(IntPtr handle);
    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool GetExitCodeProcess(IntPtr process, out uint code);

    public static void Run(string root)
    {
        // Read/write objects, create windows/menus and enumerate; no SWITCHDESKTOP.
        var name = "WinOne-Test-" + Guid.NewGuid().ToString("N");
        var desktop = CreateDesktopW(name, IntPtr.Zero, IntPtr.Zero, 0, 0xc7, IntPtr.Zero);
        if (desktop == IntPtr.Zero) throw new Win32Exception(Marshal.GetLastWin32Error());
        try {
            Directory.CreateDirectory(root);
            var run = Path.Combine(root, Guid.NewGuid().ToString("N"));
            Directory.CreateDirectory(run);
            var executable = Environment.ProcessPath ?? throw new Exception("Missing test executable");
            static string Quote(string value) => value.Contains('"') ? throw new ArgumentException("Invalid test path") : "\"" + value + "\"";
            var command = Quote(executable);
            if (Path.GetFileNameWithoutExtension(executable).Equals("dotnet", StringComparison.OrdinalIgnoreCase))
                command += " " + Quote(typeof(ModalHandoffTests).Assembly.Location);
            command += " --isolated-modal-worker " + Quote(run);
            var startup = new StartupInfo { Size = Marshal.SizeOf<StartupInfo>(), Desktop = name };
            if (!CreateProcessW(executable, new StringBuilder(command), IntPtr.Zero, IntPtr.Zero, false, 0, IntPtr.Zero, null, ref startup, out var created))
                throw new Win32Exception(Marshal.GetLastWin32Error());
            try {
                using var child = Process.GetProcessById(created.Id);
                if (!child.WaitForExit(30000)) { child.Kill(); child.WaitForExit(); throw new TimeoutException("Isolated modal worker timed out"); }
                var result = File.ReadAllText(Path.Combine(run, "result.txt"));
                if (!GetExitCodeProcess(created.Process, out var code)) throw new Win32Exception(Marshal.GetLastWin32Error());
                if (code != 0) throw new Exception(result);
                Console.WriteLine(result);
            } finally { CloseHandle(created.Thread); CloseHandle(created.Process); }
        } finally { if (!CloseDesktop(desktop)) throw new Win32Exception(Marshal.GetLastWin32Error()); }
    }

    // The process starts on its private desktop before CLR/COM creates windows.
    public static void RunWorker(string root)
    {
        Exception? failure = null;
        var thread = new Thread(() => {
            try {
                Application.SetHighDpiMode(HighDpiMode.PerMonitorV2);
                Application.EnableVisualStyles();
                foreach (var origin in new[] { new Point(40, 40), new Point(-240, -180) })
                    foreach (var window in new[] { false, true })
                        foreach (var retry in new[] { false, true })
                            Exercise(Path.Combine(root, Guid.NewGuid().ToString("N")), origin, window, retry);
            } catch (Exception error) { failure = error; }
            finally { Application.ExitThread(); }
        }) { IsBackground = true };
        thread.SetApartmentState(ApartmentState.STA); thread.Start();
        // A timed-out test fails the test process instead of closing a live desktop.
        if (!thread.Join(TimeSpan.FromSeconds(25))) throw new TimeoutException("Isolated modal handoff timed out");
        if (failure is not null) throw new Exception("Isolated modal handoff failed", failure);
    }

    private static void Exercise(string directory, Point origin, bool windowMode, bool retry)
    {
        Directory.CreateDirectory(directory);
        using var pixels = new Bitmap(240, 180);
        using (var graphics = Graphics.FromImage(pixels)) graphics.Clear(Color.Coral);
        using var frame = CapturedFrame.Own(new Bitmap(pixels), new Rectangle(origin, pixels.Size));
        var expected = new Rectangle(origin.X + 10, origin.Y + 10, 70, 80);
        using var selector = new Selector(frame, windowMode ? [expected] : null);
        CaptureHoldWindow? hold = null;
        var attempts = 0;
        Exception? callbackFailure = null;
        try {
            selector.BeforeEditorHandoff = selection => {
                if (selection != expected) throw new Exception("selection changed before hold creation");
                attempts++;
                if (retry && attempts == 1) throw new InvalidOperationException("Injected handoff failure");
                hold = new CaptureHoldWindow(frame, selection);
                hold.Show(); hold.Refresh();
            };
            selector.Shown += (_, _) => selector.BeginInvoke((Action)(() => {
                try {
                    selector.Select(windowMode);
                    if (retry) {
                        if (selector.DialogResult != DialogResult.None || selector.Selection != expected) throw new Exception("failed handoff did not preserve the modal selection");
                        selector.Confirm();
                    }
                } catch (Exception error) { callbackFailure = error; selector.Close(); }
            }));
            var result = selector.ShowDialog();
            if (callbackFailure is not null) throw callbackFailure;
            if (result != DialogResult.OK || selector.Selection != expected || !selector.EditRequested || hold is null || !hold.Visible)
                throw new Exception($"modal handoff invalid: result={result}, selection={selector.Selection}, reason={selector.CancellationReason}, attempts={attempts}");
            var operations = new CaptureOperations(new SettingsStore(directory), directory);
            var draft = operations.PrepareSelectionEditor(frame, selector.Selection, new CaptureSettings { OutputDirectory = directory, CopyToClipboard = false });
            if (draft.Document.Crop != new AnnotationCrop(10, 10, 70, 80)) throw new Exception("modal selection produced the wrong draft crop");
            hold.Adopt(frame, draft.Id);
            if (new CaptureDraftStore(directory).Read()?.Id != draft.Id) throw new Exception("modal draft was not recoverable");
            var restarted = new CaptureOperations(new SettingsStore(directory), directory, _ => throw new Exception("isolated test must not copy to clipboard"));
            var output = restarted.CompleteDraft(draft.Id, draft.Revision, null, CancellationToken.None);
            using var saved = new Bitmap(output.Path);
            if (saved.Size != expected.Size || saved.GetPixel(0, 0).ToArgb() != Color.Coral.ToArgb() || saved.GetPixel(69, 79).ToArgb() != Color.Coral.ToArgb())
                throw new Exception("modal selection did not export its exact original pixels");
            if (restarted.GetDraft() is not null) throw new Exception("saved modal draft reopened");
        } finally { hold?.Dispose(); }
    }

    private sealed class Selector(CapturedFrame frame, IReadOnlyList<Rectangle>? windows)
        : NativeSelectionWindow(frame, "en-US", windows: windows, directEdit: true)
    {
        public void Select(bool window)
        {
            var start = window ? new Point(20, 20) : new Point(10, 10);
            var end = window ? start : new Point(80, 90);
            OnMouseDown(new MouseEventArgs(MouseButtons.Left, 1, start.X, start.Y, 0));
            OnMouseMove(new MouseEventArgs(MouseButtons.Left, 0, end.X, end.Y, 0));
            OnMouseUp(new MouseEventArgs(MouseButtons.Left, 1, end.X, end.Y, 0));
        }
        public void Confirm() => OnKeyDown(new KeyEventArgs(Keys.Enter));
    }
}
