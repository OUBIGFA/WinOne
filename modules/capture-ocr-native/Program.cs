using System.Buffers.Binary;
using System.Text;
using System.Text.Json;
using WinOne.Capture.Ocr;
using WinOne.Capture;

internal static class Program
{
    private const int MaximumBytes = 32 * 1024 * 1024;
    private static int Main(string[] args)
    {
        Console.OutputEncoding = new UTF8Encoding(false);
        try {
            if (args.Length > 0 && args[0] == "--ocr-worker") args = args[1..];
            if (args.Length == 0 || !Path.IsPathFullyQualified(args[0]) || args.Skip(1).Any(arg => arg is not ("--serve" or "--pin-layout")))
                throw new ArgumentException("Invalid OCR worker arguments");
            Environment.SetEnvironmentVariable("PPOCR_GPU_ARENA_DEVICE_LOCAL", "1");
            Environment.SetEnvironmentVariable("PPOCR_GPU_CONSTANT_CACHE_MB", "0");
            Environment.SetEnvironmentVariable("PPOCR_BENCH_THREADS", Math.Min(8, Math.Max(1, Environment.ProcessorCount / 2)).ToString());
            Environment.SetEnvironmentVariable("PPOCR_REC_FAST", "0");
            // Keep the pinned engine's narrow model paths ASCII. This isolated
            // process owns its working directory; no UI or other module shares it.
            Directory.SetCurrentDirectory(args[0]);
            RapidOcrNative.Load(args[0]);
            using var recognition = new OcrRecognition();
            using var input = Console.OpenStandardInput();
            bool layout = args.Contains("--pin-layout");
            if (!args.Contains("--serve")) {
                using var data = new MemoryStream();
                var buffer = new byte[81920];
                int read;
                while ((read = input.Read(buffer, 0, Math.Min(buffer.Length, MaximumBytes + 1 - (int)data.Length))) > 0)
                    data.Write(buffer, 0, read);
                if (data.Length is 0 or > MaximumBytes) throw new InvalidDataException("Invalid image size");
                return Respond(recognition, data.ToArray(), layout) ? 0 : 1;
            }
            var header = new byte[4];
            while (true) {
                if (input.Read(header, 0, 1) == 0) return 0;
                input.ReadExactly(header.AsSpan(1));
                uint length = BinaryPrimitives.ReadUInt32LittleEndian(header);
                // Per-frame geometry requests reuse the warmed CPU model; old
                // clients keep their plain PNG framing and startup layout flag.
                bool frameLayout = layout || (length & 0x80000000u) != 0;
                length &= 0x7fffffffu;
                if (length > MaximumBytes) throw new InvalidDataException("Invalid frame size");
                var data = new byte[length];
                input.ReadExactly(data);
                Respond(recognition, data, frameLayout);
            }
        } catch (Exception error) { Error(error); return 1; }
    }

    private static bool Respond(OcrRecognition recognition, byte[] data, bool layout)
    {
        try {
            bool prepare = data.Length == 0;
            // Reuse the existing scoped Windows policy: foreground requests
            // must not inherit idle execution throttling; restore it afterwards.
            using var interactive = new InteractiveWorkScope();
            var image = prepare ? NativeImage.Prepare() : NativeImage.Decode(data);
            var result = recognition.Recognize(image.Pixels, image.Width, image.Height, prepare ? false : layout);
            Reply(writer => {
                if (prepare) writer.WriteBoolean("prepared", true);
                else if (!layout) writer.WriteString("text", result.Text);
                else {
                    writer.WriteStartObject("layout");
                    writer.WriteStartArray("lines");
                    foreach (var line in result.Lines) {
                        writer.WriteStartObject(); writer.WriteString("text", line.Text); Box(writer, line.Box);
                        writer.WriteStartArray("glyphs");
                        foreach (var glyph in line.Glyphs) {
                            writer.WriteStartObject(); writer.WriteString("text", glyph.Text); Box(writer, glyph.Box); writer.WriteEndObject();
                        }
                        writer.WriteEndArray(); writer.WriteEndObject();
                    }
                    writer.WriteEndArray(); writer.WriteEndObject();
                }
                writer.WriteString("device", result.Device);
            });
            return true;
        } catch (Exception error) { Error(error); return false; }
    }

    private static void Box(Utf8JsonWriter writer, int[] box)
    {
        writer.WriteStartArray("box");
        foreach (int value in box) writer.WriteNumberValue(value);
        writer.WriteEndArray();
    }
    private static void Error(Exception error) => Reply(writer => writer.WriteString("error", error.GetType().Name));
    private static void Reply(Action<Utf8JsonWriter> write)
    {
        using var bytes = new MemoryStream();
        using (var writer = new Utf8JsonWriter(bytes)) { writer.WriteStartObject(); write(writer); writer.WriteEndObject(); }
        Console.WriteLine(Encoding.UTF8.GetString(bytes.ToArray()));
        Console.Out.Flush();
    }
}
