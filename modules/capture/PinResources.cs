using System.Reflection;

// Only the five assets used by the upstream pin window are included, not the ShareX application UI.
namespace ShareX.Properties;

internal static class Resources
{
    private static byte[] Read(string name)
    {
        using var source = Assembly.GetExecutingAssembly().GetManifestResourceStream("ShareX.Asset." + name) ?? throw new FileNotFoundException(name);
        using var bytes = new MemoryStream(); source.CopyTo(bytes); return bytes.ToArray();
    }
    private static Bitmap Bitmap(string name) { using var stream = new MemoryStream(Read(name)); using var image = new Bitmap(stream); return new Bitmap(image); }
    internal static byte[] openhand => Read("openhand.cur");
    internal static byte[] closedhand => Read("closedhand.cur");
    internal static Bitmap document_copy { get; } = Bitmap("document-copy.png");
    internal static Bitmap gear { get; } = Bitmap("gear.png");
    internal static Bitmap cross_button { get; } = Bitmap("cross-button.png");
}
