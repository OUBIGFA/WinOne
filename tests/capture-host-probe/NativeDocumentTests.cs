using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using ShareX.ScreenCaptureLib;

internal static class NativeDocumentTests
{
    private static int[] Pixels(Bitmap image) => Enumerable.Range(0, image.Height).SelectMany(y => Enumerable.Range(0, image.Width).Select(x => image.GetPixel(x, y).ToArgb())).ToArray();
    public static void Run()
    {
        using var background = new Bitmap(240, 180);
        for (var y = 0; y < background.Height; y++) for (var x = 0; x < background.Width; x++) background.SetPixel(x, y, (x / 4 + y / 4) % 2 == 0 ? Color.White : Color.LightGray);
        byte[]? example = null;
        foreach (var kind in new[] { "rectangle", "ellipse", "arrow", "pen", "cover", "mosaic", "blur", "text", "mixed", "edge", "outside", "dot" }) {
            using var original = new WinOneCaptureCanvas(background);
            original.SetRegion(new Rectangle(5, 5, 225, 165));
            var kinds = kind is "mixed" or "edge" or "outside" ? new[] { "mosaic", "rectangle", "pen", "blur", "cover", "arrow", "text" } : kind == "dot" ? new[] { "pen" } : new[] { kind };
            var offset = kind == "edge" ? new PointF(-50, -50) : kind == "outside" ? new PointF(-400, -300) : PointF.Empty;
            foreach (var tool in kinds) {
                if (tool == "text") original.AddText("原生 WinOne", new Rectangle(30 + (int)offset.X, 40 + (int)offset.Y, 180, 100), 26, new[] { new WinOneTextLine("原生 WinOne", 0, 0) }, Color.RoyalBlue);
                else {
                    var points = kind == "dot" ? new[] { new PointF(20, 20) } : new[] { new PointF(20 + offset.X, 20 + offset.Y), new PointF(70 + offset.X, 90 + offset.Y), new PointF(150 + offset.X, 30 + offset.Y) };
                    original.AddDrawing(tool, new RectangleF(20.5f + offset.X, 20.5f + offset.Y, kind == "dot" ? 0 : 140, kind == "dot" ? 0 : 80), points);
                    original.SetToolStyle(new WinOneToolStyle(Color.ForestGreen, tool == "mosaic" ? 13 : tool == "blur" ? 9 : 5));
                }
            }
            using var expected = original.RenderSelection();
            var document = original.ExportDocument();
            var bytes = NativeCaptureDocumentCodec.Encode(document, background.Size);
            if (kind == "rectangle") example = bytes;
            var decoded = NativeCaptureDocumentCodec.Decode(bytes, background.Size);
            using var restored = new WinOneCaptureCanvas(background);
            restored.RestoreDocument(decoded);
            using var actual = restored.RenderSelection();
            if (actual.Size != expected.Size || !Pixels(actual).SequenceEqual(Pixels(expected))) throw new Exception(kind + " document roundtrip changed output pixels");
            if (restored.AnnotationCount != original.AnnotationCount) throw new Exception(kind + " restore lost editable layers");
            restored.AddRectangle(new Rectangle(180, 120, 25, 25)); restored.Undo();
            using var undone = restored.RenderSelection();
            if (!Pixels(undone).SequenceEqual(Pixels(expected))) throw new Exception(kind + " restored document could not continue editing and undo");
            try { restored.RestoreDocument(decoded); throw new Exception("Recovery overwrote an active canvas"); }
            catch (InvalidOperationException) { }
        }
        if (example is null) throw new Exception("Missing document fixture");
        var baseline = NativeCaptureDocumentCodec.Decode(example, background.Size);
        foreach (var layer in new[] {
            baseline.Layers[0] with { FillColor = Color.FromArgb(90, Color.Gold).ToArgb(), CornerRadius = 11, Shadow = true, ShadowColor = Color.FromArgb(100, Color.Black).ToArgb(), ShadowOffset = new Point(3, 4), Dash = 2 },
            baseline.Layers[0] with { Kind = "arrow", Points = new[] { new PointF(20.25f, 20.5f), new PointF(95.5f, 100.25f), new PointF(160.25f, 20.5f) }, CenterNodeActive = true, ArrowHead = (int)ShareX.HelpersLib.ArrowHeadDirection.Both }
        }) {
            using var first = new WinOneCaptureCanvas(background);
            first.RestoreDocument(baseline with { Layers = new[] { layer } });
            using var expected = first.RenderSelection();
            using var second = new WinOneCaptureCanvas(background);
            second.RestoreDocument(NativeCaptureDocumentCodec.Decode(NativeCaptureDocumentCodec.Encode(first.ExportDocument(), background.Size), background.Size));
            using var actual = second.RenderSelection();
            if (!Pixels(actual).SequenceEqual(Pixels(expected))) throw new Exception("Restored curve, fill, dash or shadow changed during serialization");
        }
        void Reject(byte[] bytes) {
            try { NativeCaptureDocumentCodec.Decode(bytes, background.Size); throw new Exception("Invalid native document was accepted"); }
            catch (InvalidDataException) { } catch (JsonException) { }
        }
        byte[] Change(Action<JsonObject> change) { var root = JsonNode.Parse(example)!.AsObject(); change(root); return JsonSerializer.SerializeToUtf8Bytes(root); }
        Reject(Change(root => root["layers"]![0]!["kind"] = "executable"));
        Reject(Change(root => root["$type"] = "Unexpected.Runtime.Type"));
        Reject(Change(root => root["selection"] = JsonSerializer.SerializeToNode(new { x = int.MaxValue, y = 0, width = int.MaxValue, height = 5 })));
        Reject(Change(root => root["layers"]![0]!["bounds"]!["x"] = 1e20));
        Reject(Change(root => root["layers"]![0]!["points"] = JsonSerializer.SerializeToNode(new[] { new { x = 1e20, y = 0 } })));
        var text = Encoding.UTF8.GetString(example);
        Reject(Encoding.UTF8.GetBytes("{\"version\":1," + text[1..]));
        Reject(new byte[768 * 1024 + 1]);
        Console.WriteLine("PASS: editable native document roundtrips, edge/outside layers, text, curves and invalid-data rejection");
    }
}
