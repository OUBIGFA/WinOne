using System.Text.Json;
using WinOne.Capture.Ocr;

// Execute the existing expected outputs in the native-compiled environment;
// the algorithms themselves are linked from production, not copied into tests.
OcrParagraphTests.Run();
using var fixtures = JsonDocument.Parse(File.ReadAllText(Path.Combine(AppContext.BaseDirectory, "ocr-reading-order.json")));
foreach (var example in fixtures.RootElement.EnumerateArray())
{
    var blocks = example.GetProperty("blocks").EnumerateArray().Select(block => new OcrBlock(
        block.GetProperty("text").GetString()!, block.GetProperty("left").GetDouble(),
        block.GetProperty("top").GetDouble(), block.GetProperty("right").GetDouble(), block.GetProperty("bottom").GetDouble()));
    if (OcrReadingOrder.Compose(blocks, mergeParagraphs: false) != example.GetProperty("text").GetString())
        throw new Exception("Native OCR reading order differs: " + example.GetProperty("name").GetString());
}
Console.WriteLine($"PASS: {fixtures.RootElement.GetArrayLength()} existing reading-order cases in native OCR target");
var warmup = NativeImage.Prepare();
if (warmup.Width != 192 || warmup.Height != 48 || warmup.Pixels.Length != 192 * 48 * 3)
    throw new Exception("Native image preparation dimensions differ");
try { NativeImage.Decode([1, 2, 3, 4]); throw new Exception("Invalid image was accepted"); }
catch (InvalidDataException) { }
Console.WriteLine("PASS: native image preparation and invalid-input handling");
