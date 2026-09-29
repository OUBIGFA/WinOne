using System.Text.Json;

namespace WinOne.Capture;

internal record CaptureSettings
{
    public int SchemaVersion { get; init; } = 1;
    public long Revision { get; init; }
    public string OutputDirectory { get; init; } = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.MyPictures, Environment.SpecialFolderOption.DoNotVerify), "WinOne");
    public bool CopyToClipboard { get; init; } = true;
    public bool AskWhereToSave { get; init; }
    public bool OpenEditor { get; init; }
    public bool PinAfterCapture { get; init; }
    public string Theme { get; init; } = "system";
    public int ScrollDelay { get; init; } = 300;
    public bool ScrollFromTop { get; init; }
}

// Settings are owned by this worker and use revision checks across settings windows.
internal sealed class SettingsStore
{
    internal static readonly JsonSerializerOptions Json = new(JsonSerializerDefaults.Web) { WriteIndented = true };
    private readonly object gate = new();
    private readonly string path;
    private CaptureSettings settings;
    public SettingsStore(string directory)
    {
        path = Path.Combine(directory, "settings.json");
        settings = File.Exists(path) ? JsonSerializer.Deserialize<CaptureSettings>(File.ReadAllBytes(path), Json) ?? throw new InvalidDataException("截图配置为空") : new();
        Validate(settings);
    }
    public CaptureSettings Snapshot() { lock (gate) return settings; }
    public CaptureSettings Save(CaptureSettings next)
    {
        lock (gate)
        {
            if (next.Revision != settings.Revision) throw new InvalidOperationException("截图设置已变更，请刷新后重试");
            Validate(next);
            next = next with { Revision = checked(next.Revision + 1) };
            AtomicWrite(path, JsonSerializer.SerializeToUtf8Bytes(next, Json));
            settings = next;
            return settings;
        }
    }
    private static void Validate(CaptureSettings settings)
    {
        if (settings.SchemaVersion != 1 || settings.Revision < 0) throw new InvalidDataException("不支持的截图配置版本");
        if (!Path.IsPathFullyQualified(settings.OutputDirectory)) throw new InvalidDataException("保存目录必须为绝对路径");
        if (settings.Theme is not ("system" or "light" or "dark")) throw new InvalidDataException("无效的主题");
        if (settings.ScrollDelay is < 100 or > 2000) throw new InvalidDataException("滚动间隔须为 100–2000 毫秒");
    }
    internal static void AtomicWrite(string path, byte[] bytes)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        var temporary = path + "." + Guid.NewGuid().ToString("N") + ".tmp";
        try
        {
            using (var file = new FileStream(temporary, FileMode.CreateNew, FileAccess.Write, FileShare.None, 4096, FileOptions.WriteThrough)) { file.Write(bytes); file.Flush(true); }
            File.Move(temporary, path, true);
        }
        finally { if (File.Exists(temporary)) File.Delete(temporary); }
    }
}
