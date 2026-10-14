using System.Diagnostics;
using System.Text.Json;

internal static class SettingsDialogProbe
{
    public static int Run(string directory) {
        if (!HostedToolbar.CurrentDesktopName().StartsWith("WinOne-Resource-", StringComparison.Ordinal)) throw new InvalidOperationException("Use a private desktop");
        var bundle = Environment.GetEnvironmentVariable("WINONE_DIALOG_BUNDLE") ?? throw new InvalidOperationException("Set WINONE_DIALOG_BUNDLE");
        var port = int.Parse(Environment.GetEnvironmentVariable("WINONE_DIALOG_PORT") ?? "9239");
        if (port < 1024 || port > 65535 || Directory.Exists(directory)) throw new ArgumentException("Invalid probe output or port");
        Directory.CreateDirectory(directory);
        var profile = Path.Combine(directory, "profile"); Directory.CreateDirectory(profile);
        File.WriteAllText(Path.Combine(profile, "settings.json"), JsonSerializer.Serialize(new { schemaVersion=1, captureShortcut="", pinShortcut="" }));
        var pipe = new string((char)92,2)+"."+(char)92+"pipe"+(char)92+"WinOne-dialog-"+Guid.NewGuid().ToString("N");
        Process Start(string name, bool ui) {
            var info = new ProcessStartInfo(Path.Combine(bundle,name)) { UseShellExecute=false, CreateNoWindow=true, RedirectStandardError=true };
            foreach(var arg in new[]{"--pipe",pipe,"--data-dir",profile}) info.ArgumentList.Add(arg);
            if(ui){info.ArgumentList.Add("--standard-session");info.ArgumentList.Add("--open-settings"); info.Environment["WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"]=$"--remote-debugging-port={port} --remote-debugging-address=127.0.0.1";}
            else info.ArgumentList.Add("--headless");
            var child=Process.Start(info) ?? throw new Exception("Probe child failed to start");
            child.ErrorDataReceived+=(_,e)=>{if(e.Data is not null)File.AppendAllText(Path.Combine(directory,name+".log"),e.Data+Environment.NewLine);};
            child.BeginErrorReadLine();return child;
        }
        using var host=Start("WinOne.Host.exe",false);
        using var ui=Start("WinOne.exe",true);
        try {
            File.WriteAllText(Path.Combine(directory,"ready.json"),JsonSerializer.Serialize(new{host=host.Id,ui=ui.Id,port}));
            var deadline=DateTime.UtcNow.AddSeconds(50);
            while(!File.Exists(Path.Combine(directory,"done"))){
                if(ui.HasExited || DateTime.UtcNow>=deadline) throw new Exception("Dialog probe ended without a test result");
                if(File.Exists(Path.Combine(directory,"reopen")) && !File.Exists(Path.Combine(directory,"reopened"))){using var repeated=Start("WinOne.exe",true); File.WriteAllText(Path.Combine(directory,"reopened"),"requested");}
                Thread.Sleep(100);
            }
            return 0;
        } finally {
            // Only terminate this fixture's isolated processes on cleanup.
            foreach(var child in new[]{ui,host}) if(!child.HasExited){child.Kill(true);child.WaitForExit(5000);}
        }
    }
}
