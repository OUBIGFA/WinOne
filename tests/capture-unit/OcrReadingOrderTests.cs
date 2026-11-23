using System.Text.Json;
using WinOne.Capture;
using WinOne.Capture.Ocr;

internal static class OcrReadingOrderTests
{
    private sealed record Example(string Name, OcrBlock[] Blocks, string Text);

    internal static void Run()
    {
        // Golden outputs were produced by the previously shipped HushSnap
        // adapter. They cover median clustering, indentation, paragraphs,
        // vertical reading, mixed scripts, punctuation and URL preservation.
        var examples = JsonSerializer.Deserialize<Example[]>(File.ReadAllText(
            Path.Combine(AppContext.BaseDirectory, "ocr-reading-order.json")), SettingsStore.Json)!;
        foreach (var example in examples)
        {
            string actual = OcrReadingOrder.Compose(example.Blocks, mergeParagraphs: false);
            if (actual != example.Text) throw new Exception($"OCR reading order differs in {example.Name}: {actual}");
        }
        Console.WriteLine($"PASS: {examples.Length} HushSnap reading-order cases");
    }
}
