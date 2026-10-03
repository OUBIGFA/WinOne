using System.Diagnostics;
using System.Text.Json;
using WinOne.Capture;

internal static class NativeHostRoundTripProbe
{
    // Called by the Go integration test that owns the real Host pipe and data.
    public static int Run(string pipe, int parentId, string moduleDirectory)
    {
        using var owner = Process.GetProcessById(parentId);
        var client = new NativeHostConfiguration(pipe, owner);
        using var deadline = new CancellationTokenSource(TimeSpan.FromSeconds(15));
        foreach (var desired in new string[]?[] { Array.Empty<string>(), null, new[] { "mosaic", "arrow" } }) {
            var saved = client.SaveToolbarAsync(desired, deadline.Token).GetAwaiter().GetResult();
            if ((desired is null) != (saved is null) || desired is not null && !desired.SequenceEqual(saved!)) throw new Exception("Real Host did not confirm the requested preferences");
            var loaded = new SettingsStore(moduleDirectory);
            var projected = loaded.Snapshot().ToolbarTools;
            if (!loaded.IsManaged || (desired is null) != (projected is null) || desired is not null && !desired.SequenceEqual(projected!))
                throw new Exception("Native worker startup reader did not restore the confirmed preferences");
        }
        if (client.IsSaving) throw new Exception("Completed configuration retained its active state");
        Console.WriteLine(JsonSerializer.Serialize(new { saves = 3, startupReads = 3, hostProcess = parentId, completed = true }));
        return 0;
    }
}
