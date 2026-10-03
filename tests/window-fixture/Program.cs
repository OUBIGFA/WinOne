using System.Diagnostics;
using System.Globalization;
using System.Text.Json;
using System.Runtime.InteropServices;

internal static class Program
{
    [DllImport("user32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetWindowPos(IntPtr window, IntPtr after, int x, int y, int width, int height, uint flags);
    [DllImport("user32.dll")]
    private static extern IntPtr GetWindowLongPtrW(IntPtr window, int index);
    // This window is owned by the integration test and never automates another application.
    [STAThread]
    private static int Main(string[] args)
    {
        if (args.Length is < 1 or > 2) return 2;
        Application.SetHighDpiMode(HighDpiMode.PerMonitorV2); Application.EnableVisualStyles();
        using var form = new Form { Text = "WinOne native test window", Width = 400, Height = 240, StartPosition = FormStartPosition.CenterScreen, BackColor = Color.SlateGray, TopMost = args.Length == 2 && args[1] == "--topmost" };
        form.Controls.Add(new Label { Text = "WinOne 窗口验证", Dock = DockStyle.Fill, TextAlign = ContentAlignment.MiddleCenter, ForeColor = Color.White, Font = new Font("Microsoft YaHei UI", 14) });
        void WriteIdentity()
        {
            var state = new { handle = form.Handle.ToInt64().ToString(CultureInfo.InvariantCulture), pid = Environment.ProcessId, created = Process.GetCurrentProcess().StartTime.ToUniversalTime().ToFileTimeUtc().ToString(CultureInfo.InvariantCulture), title = form.Text, topmost = (GetWindowLongPtrW(form.Handle, -20).ToInt64() & 8) != 0 };
            File.WriteAllText(args[0], JsonSerializer.Serialize(state));
        }
        using var lifetime = new System.Windows.Forms.Timer { Interval = 100 };
        if (args.Length == 2 && args[1] == "--controlled")
        {
            var deadline = DateTime.UtcNow.AddMinutes(3);
            var shownByController = false;
            var hiddenByController = false;
            lifetime.Tick += (_, _) => {
                if (!shownByController && File.Exists(args[0] + ".show")) { shownByController = true; form.Show(); form.Activate(); }
                if (!hiddenByController && File.Exists(args[0] + ".hide")) { hiddenByController = true; form.Hide(); }
                if (File.Exists(args[0] + ".close") || DateTime.UtcNow >= deadline) form.Close();
            };
            lifetime.Start();
        }
        form.Shown += (_, _) => form.BeginInvoke(() =>
        {
            if (args.Length == 2 && args[1] == "--topmost" && !SetWindowPos(form.Handle, new IntPtr(-1), 0, 0, 0, 0, 0x13)) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
            WriteIdentity();
        });
        Application.Run(form); return 0;
    }
}
