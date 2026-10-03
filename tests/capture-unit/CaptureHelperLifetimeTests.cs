using System.Diagnostics;

internal static class CaptureHelperLifetimeTests
{
    private static Process Start(string executable, params string[] arguments)
    {
        var start = new ProcessStartInfo(executable) { UseShellExecute = false, CreateNoWindow = true, RedirectStandardOutput = true };
        foreach (var argument in arguments) start.ArgumentList.Add(argument);
        return Process.Start(start) ?? throw new Exception("lifetime probe did not start");
    }
    private static void Stop(Process process)
    {
        if (!process.HasExited) process.Kill(true);
        if (!process.WaitForExit(2000)) throw new Exception("test process did not stop");
    }
    private static void Ready(Process process)
    {
        if (process.StandardOutput.ReadLineAsync().WaitAsync(TimeSpan.FromSeconds(2)).GetAwaiter().GetResult() != "ready") throw new Exception("lifetime guard did not become ready");
    }
    private static void ExpectExit(Process process, int code)
    {
        if (!process.WaitForExit(2000) || process.ExitCode != code) throw new Exception($"lifetime guard did not exit with {code}");
    }
    internal static void Run(string probe)
    {
        var parentExecutable = Path.ChangeExtension(typeof(CaptureHelperLifetimeTests).Assembly.Location, ".exe");
        using (var parent = Start(parentExecutable, "-777"))
        {
            try
            {
                using var child = Start(probe, parent.Id.ToString(System.Globalization.CultureInfo.InvariantCulture), "5000");
                try { Ready(child); Stop(parent); ExpectExit(child, 8); }
                finally { Stop(child); }
            }
            finally { Stop(parent); }
        }
        using (var child = Start(probe, "0", "150"))
        {
            try { Ready(child); ExpectExit(child, 8); }
            finally { Stop(child); }
        }
        using (var child = Start(probe, "0", "1000", "complete"))
        {
            try { Ready(child); ExpectExit(child, 0); }
            finally { Stop(child); }
        }
        Console.WriteLine("PASS: helper exits on parent death and deadline; normal completion cancels the guard");
    }
}
