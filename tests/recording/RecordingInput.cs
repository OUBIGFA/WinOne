using System.Runtime.InteropServices;
using System.Text;

// Opt-in system tests only. Every real pointer event is checked against an
// owned control immediately before dispatch; never click through to the user.
internal static class RecordingInput
{
    [DllImport("user32.dll")] private static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] private static extern bool SetForegroundWindow(IntPtr window);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window, out uint process);
    [DllImport("user32.dll")] private static extern bool AttachThreadInput(uint from, uint to, bool attach);
    [DllImport("user32.dll")] private static extern bool SetCursorPos(int x, int y);
    [DllImport("user32.dll")] private static extern IntPtr WindowFromPoint(Point point);
    [DllImport("user32.dll")] private static extern IntPtr GetAncestor(IntPtr window, uint flags);
    [DllImport("user32.dll", SetLastError = true)] private static extern uint SendInput(uint count, Input[] input, int size);
    [StructLayout(LayoutKind.Sequential)] private struct Input { public uint Type; public MouseInput Mouse; }
    [StructLayout(LayoutKind.Sequential)] private struct MouseInput { public int X, Y; public uint Data, Flags, Time; public UIntPtr Extra; }
    private delegate bool EnumWindowsProc(IntPtr window, IntPtr data);
    [DllImport("kernel32.dll")] private static extern uint GetCurrentThreadId();
    [DllImport("user32.dll")] private static extern bool EnumThreadWindows(uint thread, EnumWindowsProc callback, IntPtr data);
    [DllImport("user32.dll")] private static extern bool IsWindowVisible(IntPtr window);
    [DllImport("user32.dll")] private static extern bool GetWindowRect(IntPtr window, out RectangleEdges bounds);
    [DllImport("user32.dll")] private static extern bool GetWindowDisplayAffinity(IntPtr window, out uint affinity);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern int GetClassName(IntPtr window, StringBuilder name, int count);
    [StructLayout(LayoutKind.Sequential)] private struct RectangleEdges { public int Left, Top, Right, Bottom; }

    internal static void Hover(Form window, ToolStrip strip, string name)
    {
        var button = strip.Items["recording-" + name] ?? throw new Exception("Recording button missing");
        var point = strip.PointToScreen(new Point(button.Bounds.Left + button.Width / 2, button.Bounds.Top + button.Height / 2));
        if (GetAncestor(WindowFromPoint(point), 2) != window.Handle) throw new Exception("A different window covers the recording control; abort input");
        if (!SetCursorPos(point.X, point.Y)) throw new Exception("Could not position the test pointer");
    }
    internal static async Task ClickInactive(Form focus, Form window, ToolStrip strip, string name, Action arrange, Func<bool> accepted)
    {
        focus.Activate();
        if (!SetForegroundWindow(focus.Handle)) {
            // CLI-launched opt-in tests do not initially own foreground input.
            // Briefly attach only for activating our own target, then detach.
            var current = GetCurrentThreadId();
            var foreground = GetWindowThreadProcessId(GetForegroundWindow(), out _);
            var attached = foreground != 0 && foreground != current && AttachThreadInput(current, foreground, true);
            try { SetForegroundWindow(focus.Handle); } finally { if (attached) AttachThreadInput(current, foreground, false); }
        }
        await Until(() => GetForegroundWindow() == focus.Handle, "Owned focus target could not be activated");
        arrange(); // restore topmost order with SWP_NOACTIVATE, not a focus click
        if (GetForegroundWindow() == window.Handle) throw new Exception("The single-click test never deactivated its recorder");
        Hover(window, strip, name);
        // Keep a bounded, test-only native trace: an injected input receipt does
        // not prove that the intended item received the first click.
        using var trace = new ClickTrace(strip);
        ToolStripItem? clicked = null;
        ToolStripItemClickedEventHandler itemClicked = (_, e) => clicked = e.ClickedItem;
        strip.ItemClicked += itemClicked;
        try {
            var inputs = new[] { new Input { Mouse = new MouseInput { Flags = 2 } }, new Input { Mouse = new MouseInput { Flags = 4 } } };
            if (SendInput(2, inputs, Marshal.SizeOf<Input>()) != 2) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
            await Until(accepted, "The first inactive click did not execute recording-" + name);
        } catch {
            var point = strip.PointToClient(Cursor.Position);
            Console.Error.WriteLine($"First click {name}: item={clicked?.Name}; underPointer={strip.GetItemAt(point)?.Name}; foreground={GetForegroundWindow()}; recorder={window.Handle}; pointer={point}; stripEnabled={strip.Enabled}; trace={string.Join(" | ", trace.Events)}");
            foreach (var label in window.Controls.OfType<TableLayoutPanel>().Single().Controls.OfType<Label>()) Console.Error.WriteLine("Panel label: " + label.Text);
            throw;
        } finally { strip.ItemClicked -= itemClicked; }
    }
    private sealed class ClickTrace : NativeWindow, IDisposable
    {
        internal readonly List<string> Events = [];
        internal ClickTrace(Control control) => AssignHandle(control.Handle);
        protected override void WndProc(ref Message message)
        {
            base.WndProc(ref message);
            if (Events.Count < 16 && message.Msg is 0x21 or 0x201 or 0x202 or 0x215)
                Events.Add($"msg=0x{message.Msg:X},pos=0x{message.LParam:X},result={message.Result}");
        }
        public void Dispose() => ReleaseHandle();
    }
    internal static string DescribeThreadWindows()
    {
        var windows = new List<string>();
        EnumThreadWindows(GetCurrentThreadId(), (window, _) => {
            var name = new StringBuilder(256); GetClassName(window, name, name.Capacity);
            GetWindowDisplayAffinity(window, out var affinity);
            windows.Add($"{window}:{name}:visible={IsWindowVisible(window)}:affinity=0x{affinity:X}");
            return true;
        }, IntPtr.Zero);
        return string.Join(" | ", windows);
    }
    internal static async Task<Rectangle> WaitForProtectedTooltip()
    {
        Rectangle result = Rectangle.Empty;
        string? failure = null;
        await Until(() => {
            EnumThreadWindows(GetCurrentThreadId(), (window, _) => {
                if (!WinOne.Capture.NativeUI.RecordingWindowProtection.IsToolTipWindow(window) || !IsWindowVisible(window)) return true;
                if (!GetWindowDisplayAffinity(window, out var affinity) || affinity != 0x11) { failure = "Visible recording tooltip is not capture-excluded"; return false; }
                if (!GetWindowRect(window, out var bounds)) return true;
                result = Rectangle.FromLTRB(bounds.Left, bounds.Top, bounds.Right, bounds.Bottom); return false;
            }, IntPtr.Zero);
            if (failure is not null) throw new Exception(failure);
            return !result.IsEmpty;
        }, "Recording hover did not show its native tooltip");
        return result;
    }
    internal static async Task Until(Func<bool> ready, string message)
    {
        var deadline = DateTime.UtcNow.AddSeconds(5);
        while (!ready()) { if (DateTime.UtcNow >= deadline) throw new Exception(message); await Task.Delay(20); }
    }
}
