using System.Runtime.InteropServices;
using WinOne.Capture;

// A desktop where programs keep thousands of hidden top-level windows must
// still allow captures: the window snapshot once failed above 4096 windows
// ("窗口数量超过处理上限") and every shortcut capture failed until WinOne
// restarted. The windows here are never shown, so the test takes no focus.
internal static class ManyWindowsTests
{
    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern IntPtr CreateWindowEx(int exStyle, string className, string? name, int style,
        int x, int y, int width, int height, IntPtr parent, IntPtr menu, IntPtr instance, IntPtr param);
    [DllImport("user32.dll")]
    private static extern bool DestroyWindow(IntPtr window);

    public static void Run()
    {
        const int extra = 5000;
        var windows = new List<IntPtr>(extra);
        try
        {
            // "Static" is a system class: no registration, no window procedure of ours.
            for (var i = 0; i < extra; i++)
            {
                var window = CreateWindowEx(0, "Static", null, 0, 0, 0, 10, 10, IntPtr.Zero, IntPtr.Zero, IntPtr.Zero, IntPtr.Zero);
                if (window == IntPtr.Zero) throw new Exception($"could not create hidden window {i}: {Marshal.GetLastWin32Error()}");
                windows.Add(window);
            }
            var elapsed = System.Diagnostics.Stopwatch.StartNew();
            var targets = WindowCaptureTargets.Snapshot();
            elapsed.Stop();
            if (targets.Any(target => windows.Contains(target.Handle))) throw new Exception("a hidden window became a capture target");
            // Hidden windows are rejected before their details are read.
            if (elapsed.ElapsedMilliseconds > 1000) throw new Exception($"snapshot of {extra} extra windows took {elapsed.ElapsedMilliseconds} ms");
            Console.WriteLine($"PASS: snapshot with {extra} extra hidden windows, {targets.Count} targets, {elapsed.ElapsedMilliseconds} ms");
        }
        finally
        {
            foreach (var window in windows) DestroyWindow(window);
        }
    }
}
