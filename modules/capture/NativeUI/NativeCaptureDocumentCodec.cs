using System.Text.Json;
using System.Text.Json.Serialization;
using ShareX.ScreenCaptureLib;

namespace WinOne.Capture.NativeUI;

internal static class NativeCaptureDocumentCodec
{
    private const int MaximumBytes = 768 * 1024;
    private static readonly JsonSerializerOptions Json = CreateOptions();
    private static JsonSerializerOptions CreateOptions()
    {
        var options = new JsonSerializerOptions { PropertyNamingPolicy = JsonNamingPolicy.CamelCase, UnmappedMemberHandling = JsonUnmappedMemberHandling.Disallow, MaxDepth = 32 };
        options.Converters.Add(new RectangleConverter()); options.Converters.Add(new RectangleFConverter());
        options.Converters.Add(new PointConverter()); options.Converters.Add(new PointFConverter());
        return options;
    }
    public static byte[] Encode(WinOneCaptureDocument document, Size image)
    {
        document.Validate(image);
        var bytes = JsonSerializer.SerializeToUtf8Bytes(document, Json);
        if (bytes.Length > MaximumBytes) throw new InvalidDataException("原生截图草稿过大");
        return bytes;
    }
    public static WinOneCaptureDocument Decode(byte[] bytes, Size image)
    {
        if (bytes.Length == 0 || bytes.Length > MaximumBytes) throw new InvalidDataException("原生截图草稿大小无效");
        using var parsed = JsonDocument.Parse(bytes, new JsonDocumentOptions { MaxDepth = 32 });
        CheckNames(parsed.RootElement);
        var document = JsonSerializer.Deserialize<WinOneCaptureDocument>(bytes, Json) ?? throw new InvalidDataException("原生截图草稿为空");
        document.Validate(image); return document;
    }
    internal static void CheckNames(JsonElement element)
    {
        if (element.ValueKind == JsonValueKind.Object) {
            var names = new HashSet<string>(StringComparer.Ordinal);
            foreach (var property in element.EnumerateObject()) {
                if (!names.Add(property.Name)) throw new InvalidDataException("原生截图草稿包含重复字段");
                CheckNames(property.Value);
            }
        } else if (element.ValueKind == JsonValueKind.Array) foreach (var item in element.EnumerateArray()) CheckNames(item);
    }
    private static JsonDocument Geometry(ref Utf8JsonReader reader, params string[] fields)
    {
        var document = JsonDocument.ParseValue(ref reader);
        var root = document.RootElement;
        if (root.ValueKind != JsonValueKind.Object || root.EnumerateObject().Count() != fields.Length || root.EnumerateObject().Any(property => !fields.Contains(property.Name))) {
            document.Dispose(); throw new InvalidDataException("原生截图坐标格式无效");
        }
        return document;
    }
    // Canonical coordinate fields avoid contradictory Rectangle.Location/X
    // representations and never serialize runtime object metadata.
    private sealed class RectangleConverter : JsonConverter<Rectangle>
    {
        public override Rectangle Read(ref Utf8JsonReader reader, Type type, JsonSerializerOptions options) {
            using var value = Geometry(ref reader, "x", "y", "width", "height"); var r = value.RootElement;
            return new(r.GetProperty("x").GetInt32(), r.GetProperty("y").GetInt32(), r.GetProperty("width").GetInt32(), r.GetProperty("height").GetInt32());
        }
        public override void Write(Utf8JsonWriter w, Rectangle r, JsonSerializerOptions options) { w.WriteStartObject(); w.WriteNumber("x", r.X); w.WriteNumber("y", r.Y); w.WriteNumber("width", r.Width); w.WriteNumber("height", r.Height); w.WriteEndObject(); }
    }
    private sealed class RectangleFConverter : JsonConverter<RectangleF>
    {
        public override RectangleF Read(ref Utf8JsonReader reader, Type type, JsonSerializerOptions options) {
            using var value = Geometry(ref reader, "x", "y", "width", "height"); var r = value.RootElement;
            return new(r.GetProperty("x").GetSingle(), r.GetProperty("y").GetSingle(), r.GetProperty("width").GetSingle(), r.GetProperty("height").GetSingle());
        }
        public override void Write(Utf8JsonWriter w, RectangleF r, JsonSerializerOptions options) { w.WriteStartObject(); w.WriteNumber("x", r.X); w.WriteNumber("y", r.Y); w.WriteNumber("width", r.Width); w.WriteNumber("height", r.Height); w.WriteEndObject(); }
    }
    private sealed class PointConverter : JsonConverter<Point>
    {
        public override Point Read(ref Utf8JsonReader reader, Type type, JsonSerializerOptions options) {
            using var value = Geometry(ref reader, "x", "y"); var p = value.RootElement;
            return new(p.GetProperty("x").GetInt32(), p.GetProperty("y").GetInt32());
        }
        public override void Write(Utf8JsonWriter w, Point p, JsonSerializerOptions options) { w.WriteStartObject(); w.WriteNumber("x", p.X); w.WriteNumber("y", p.Y); w.WriteEndObject(); }
    }
    private sealed class PointFConverter : JsonConverter<PointF>
    {
        public override PointF Read(ref Utf8JsonReader reader, Type type, JsonSerializerOptions options) {
            using var value = Geometry(ref reader, "x", "y"); var p = value.RootElement;
            return new(p.GetProperty("x").GetSingle(), p.GetProperty("y").GetSingle());
        }
        public override void Write(Utf8JsonWriter w, PointF p, JsonSerializerOptions options) { w.WriteStartObject(); w.WriteNumber("x", p.X); w.WriteNumber("y", p.Y); w.WriteEndObject(); }
    }
}
