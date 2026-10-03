using System.Text.Json;
using System.Text.Json.Nodes;
using WinOne.Capture;

internal static class CapturePlacementTests
{
    public static void Run(string directory)
    {
        var placement = new CapturePlacement(-1800, 100, -1920, 0, 3840, 1080);
        using var image = new Bitmap(40, 30);
        using var desktop = CapturedFrame.Own(new Bitmap(3840, 1080), new Rectangle(-1920, 0, 3840, 1080));
        using var background = desktop.CreatePreview();
        if (background.Size != new Size(1600, 450)) throw new Exception("desktop preview was not bounded");
        var store = new CaptureDraftStore(directory);
        var draft = store.Begin(image, new CaptureSettings(), placement, background);
        if (new CaptureDraftStore(directory).Read()?.Placement != placement) throw new Exception("capture placement did not survive restart");
        var backgroundPath = Path.Combine(directory, "previews", draft.DesktopPreviewId + ".png");
        using (var savedBackground = new Bitmap(backgroundPath)) if (savedBackground.Size != background.Size) throw new Exception("desktop preview was not persisted");
        foreach (var invalid in new[] { placement with { X = -2000 }, placement with { Y = 1070 }, placement with { DesktopWidth = 0 }, placement with { DesktopX = int.MaxValue } })
        {
            try { invalid.Validate(image.Size); throw new Exception("invalid placement accepted"); }
            catch (InvalidDataException) { }
        }
        var metadata = Path.Combine(directory, "capture-draft.json");
        File.WriteAllText(metadata, JsonSerializer.Serialize(draft with { Placement = placement with { X = 3000 } }, SettingsStore.Json));
        try { store.Read(); throw new Exception("corrupt placement was accepted"); }
        catch (InvalidDataException) { }
        var legacy = JsonNode.Parse(JsonSerializer.Serialize(draft, SettingsStore.Json))!.AsObject();
        legacy.Remove("placement");
        legacy.Remove("desktopPreviewId");
        File.WriteAllText(metadata, legacy.ToJsonString());
        if (store.Read()?.Placement is not null) throw new Exception("legacy draft acquired invented coordinates");
        File.WriteAllText(metadata, JsonSerializer.Serialize(draft, SettingsStore.Json));
        store.Cancel(draft.Id, draft.Revision);
        if (File.Exists(backgroundPath)) throw new Exception("cancel retained the desktop preview");
    }
}
