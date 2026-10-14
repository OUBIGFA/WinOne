using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;

internal static class TrayExitProbe
{
    private delegate bool EnumProc(IntPtr window, IntPtr state);
    [DllImport("user32.dll")] private static extern bool EnumWindows(EnumProc callback, IntPtr state);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr window, out int pid);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern int GetClassNameW(IntPtr window, StringBuilder name, int length);
    [DllImport("user32.dll", SetLastError = true)] private static extern bool PostMessageW(IntPtr window, uint message, IntPtr w, IntPtr l);
    private static IntPtr Find(int pid, string expected) {
        var result = IntPtr.Zero;
        EnumWindows((window, _) => {
            GetWindowThreadProcessId(window, out var owner);
            if (owner != pid) return true;
            var name = new StringBuilder(256); GetClassNameW(window, name, name.Capacity);
            if (name.ToString() == expected) result = window;
            return true;
        }, IntPtr.Zero);
        return result;
    }
    internal static bool RequestExit(int pid) {
        var tray=Find(pid,"SystrayClass");
        return tray!=IntPtr.Zero&&PostMessageW(tray,0x111,new IntPtr(3),IntPtr.Zero);
    }
    public static int Run(string output) {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private desktop");
        var bundle = Environment.GetEnvironmentVariable("WINONE_EXIT_BUNDLE") ?? throw new InvalidOperationException("Set WINONE_EXIT_BUNDLE");
        if (Directory.Exists(output)) throw new IOException("Choose a new output directory");
        Directory.CreateDirectory(output);
        var profile = Path.Combine(output, "profile"); Directory.CreateDirectory(profile);
        File.WriteAllText(Path.Combine(profile, "settings.json"), JsonSerializer.Serialize(new { schemaVersion = 1, captureShortcut = "", pinShortcut = "" }));
        var pipe = new string((char)92, 2) + "." + (char)92 + "pipe" + (char)92 + "WinOne-tray-exit-" + Guid.NewGuid().ToString("N");
        Process Start(string executable, bool settings) {
            var info = new ProcessStartInfo(Path.Combine(bundle, executable)) { UseShellExecute = false, CreateNoWindow = true, RedirectStandardError = true };
            foreach (var arg in new[] { "--pipe", pipe, "--data-dir", profile, "--standard-session" }) info.ArgumentList.Add(arg);
            if (settings) info.ArgumentList.Add("--open-settings");
            var process = Process.Start(info) ?? throw new Exception("Launch failed");
            process.ErrorDataReceived += (_, args) => { if (args.Data is not null) { lock (process) File.AppendAllText(Path.Combine(output, executable + ".log"), args.Data + Environment.NewLine); } };
            process.BeginErrorReadLine();
            return process;
        }
        using var host = Start("WinOne.Host.exe", false);
        using var desktop = Start("WinOne.exe", true);
        try {
            void CheckTrayStartup() {
                var log = Path.Combine(output, "WinOne.Host.exe.log");
                if (File.Exists(log) && File.ReadAllText(log).Contains("unable to init instance", StringComparison.Ordinal))
                    throw new InvalidOperationException("Tray initialization failed on this desktop; no exit menu exists. This run cannot verify product exit.");
            }
            var deadline = DateTime.UtcNow.AddSeconds(20);
            IntPtr tray;
            while ((tray = Find(host.Id, "SystrayClass")) == IntPtr.Zero || Find(desktop.Id, "WinOne.Desktop") == IntPtr.Zero) {
                CheckTrayStartup();
                if (host.HasExited || desktop.HasExited || DateTime.UtcNow >= deadline) throw new Exception("Test windows did not initialize");
                Thread.Sleep(50);
            }
            CheckTrayStartup();
            // Empty profile has separator/open/exit IDs 1/2/3. Send the actual
            // tray action without global input or foreground activation.
            if (!PostMessageW(tray, 0x111, new IntPtr(3), IntPtr.Zero)) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
            // The tray HWND exists before its menu action loop is ready. Retry
            // the idempotent exit request while observing this owned process.
            var exitDeadline = DateTime.UtcNow.AddSeconds(10);
            while (!host.WaitForExit(250) && DateTime.UtcNow < exitDeadline) {
                CheckTrayStartup();
                if (!PostMessageW(tray, 0x111, new IntPtr(3), IntPtr.Zero)) break;
            }
            var hostExited = host.HasExited;
            var desktopExited = desktop.WaitForExit(5000);
            File.WriteAllText(Path.Combine(output, "report.json"), JsonSerializer.Serialize(new { hostExited, desktopExited }));
            if (!hostExited || !desktopExited) throw new Exception($"Tray exit left processes: hostExited={hostExited}, desktopExited={desktopExited}");
            return 0;
        } finally {
            // Cleanup only this probe's children and their browser processes.
            foreach (var process in new[] { desktop, host }) if (!process.HasExited) { process.Kill(entireProcessTree: true); process.WaitForExit(5000); }
        }
    }
}
