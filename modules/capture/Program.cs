using System.Diagnostics;
using System.Globalization;
using System.Security.Cryptography;
using System.Text;
using ShareX.HelpersLib;

namespace WinOne.Capture;

internal static class Program
{
    [STAThread]
    private static int Main(string[] args)
    {
        Console.OutputEncoding = new UTF8Encoding(false);
        try
        {
            string? pipe = null, directory = null; int parent = 0;
            for (int i = 0; i < args.Length; i++)
            {
                switch (args[i])
                {
                    case "--pipe": pipe = args[++i]; break;
                    case "--data-dir": directory = args[++i]; break;
                    case "--parent": parent = int.Parse(args[++i], CultureInfo.InvariantCulture); break;
                    default: throw new ArgumentException("Unknown argument: " + args[i]);
                }
            }
            if (pipe is null || directory is null || !Path.IsPathFullyQualified(directory)) throw new ArgumentException("Missing pipe or absolute data directory");
            using var mutex = new Mutex(false, "Local\\WinOneCapture-" + Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(pipe))));
            try { if (!mutex.WaitOne(0)) return 2; } catch (AbandonedMutexException) { }
            try
            {
                Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
                Application.SetUnhandledExceptionMode(UnhandledExceptionMode.ThrowException);
                CultureInfo.CurrentUICulture = CultureInfo.GetCultureInfo("zh-CN");
                ShareXResources.Name = "WinOne";
                using var iconStream = typeof(Program).Assembly.GetManifestResourceStream("WinOne.Capture.Icon") ?? throw new InvalidDataException("Missing application icon");
                ShareXResources.Icon = new Icon(iconStream);
                using var app = new CaptureApplication(pipe, directory);
                using var owner = parent == 0 ? null : Process.GetProcessById(parent);
                if (owner is not null) { owner.EnableRaisingEvents = true; owner.Exited += (_, _) => app.RequestStop(); if (owner.HasExited) app.RequestStop(); }
                Application.Run(app); return app.ExitCode;
            }
            finally { mutex.ReleaseMutex(); }
        }
        catch (Exception error) { Console.Error.WriteLine(error); return 1; }
    }
}
