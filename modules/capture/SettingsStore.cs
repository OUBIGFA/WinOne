using System.Text.Json;

namespace WinOne.Capture;

internal record CaptureSettings
{
    public int SchemaVersion { get; init; } = 1;
    public long Revision { get; init; }
    public string OutputDirectory { get; init; } = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.MyPictures, Environment.SpecialFolderOption.DoNotVerify), "WinOne");
    public bool CopyToClipboard { get; init; } = true;
    public bool AskWhereToSave { get; init; }
    public bool OpenEditor { get; init; } = true;
    public bool PinAfterCapture { get; init; }
    public string Theme { get; init; } = "system";
    public int ScrollDelay { get; init; } = 300;
    public bool ScrollFromTop { get; init; }
    public string[]? ToolbarTools { get; init; }
    public string? FileNameTemplate { get; init; }
}

// Managed workers consume a Host projection; standalone test engines retain the
// legacy store. Reload swaps immutable records without changing active sessions.
internal sealed class SettingsStore
{
    internal static readonly JsonSerializerOptions Json = new(JsonSerializerDefaults.Web) { WriteIndented = true };
    private readonly object gate = new();
    private readonly string path;
    private readonly string? managedPath;
    public bool IsManaged => managedPath is not null;
    private CaptureSettings settings;
    public SettingsStore(string directory)
    {
        path = Path.Combine(directory, "settings.json");
        var projection = Path.Combine(directory, "managed-settings.json");
        // File.Exists hides access/type errors. Only an absent projection permits
        // legacy mode; an unreadable or malformed Host projection must fail.
        try { _ = File.GetAttributes(projection); managedPath = projection; }
        catch (FileNotFoundException) { }
        catch (DirectoryNotFoundException) { }
        settings = managedPath is not null ? Read(managedPath) : File.Exists(path) ? Read(path) : new();
        Validate(settings);
    }
    public CaptureSettings Snapshot() { lock (gate) return settings; }
    public CaptureSettings Save(CaptureSettings next)
    {
        lock (gate)
        {
            if (IsManaged) throw new InvalidOperationException("请通过 WinOne 保存截图设置");
            if (next.Revision != settings.Revision) throw new InvalidOperationException("截图设置已变更，请刷新后重试");
            Validate(next);
            next = next with { Revision = checked(next.Revision + 1) };
            AtomicWrite(path, JsonSerializer.SerializeToUtf8Bytes(next, Json));
            settings = next;
            return settings;
        }
    }
    public CaptureSettings ReloadManaged()
    {
        lock (gate)
        {
            if (managedPath is null) throw new InvalidOperationException("未启用统一配置");
            var next = Read(managedPath);
            Validate(next);
            settings = next;
            return settings;
        }
    }
    private static CaptureSettings Read(string path)
    {
        using var file = File.OpenRead(path);
        if (file.Length > 2 * 1024 * 1024) throw new InvalidDataException("截图配置过大");
        using var document = JsonDocument.Parse(file, new JsonDocumentOptions { MaxDepth = 32 });
        if (document.RootElement.ValueKind != JsonValueKind.Object) throw new JsonException("截图配置必须为对象");
        var names = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        foreach (var property in document.RootElement.EnumerateObject())
            if (!names.Add(property.Name)) throw new JsonException("截图配置包含重复字段");
        var strict = new JsonSerializerOptions(Json) { UnmappedMemberHandling = System.Text.Json.Serialization.JsonUnmappedMemberHandling.Disallow };
        return document.RootElement.Deserialize<CaptureSettings>(strict) ?? throw new InvalidDataException("截图配置为空");
    }
    internal static void Validate(CaptureSettings settings)
    {
        _ = CaptureFileName.Render(settings.FileNameTemplate, DateTime.Now);
        if (settings.SchemaVersion != 1 || settings.Revision < 0) throw new InvalidDataException("不支持的截图配置版本");
        if (!Path.IsPathFullyQualified(settings.OutputDirectory)) throw new InvalidDataException("保存目录必须为绝对路径");
        if (settings.Theme is not ("system" or "light" or "dark")) throw new InvalidDataException("无效的主题");
        if (settings.ScrollDelay is < 100 or > 2000) throw new InvalidDataException("滚动间隔须为 100–2000 毫秒");
        if (settings.ToolbarTools is { } tools)
        {
            var allowed = new HashSet<string> { "select", "crop", "rectangle", "ellipse", "arrow", "pen", "cover", "text", "mosaic", "blur" };
            if (tools.Any(tool => !allowed.Remove(tool))) throw new InvalidDataException("截图工具栏包含未知或重复工具");
        }
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
