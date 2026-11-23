// SPDX-License-Identifier: GPL-3.0-only
using System.Buffers.Binary;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;
using System.Text.Json;
using WinOne.Capture.NativeUI;

namespace WinOne.Capture.Ocr;

// A separate invocation of the capture executable owns the native OCR engines.
// Host pipes, Job Object cancellation and idle/standby ownership stay unchanged.
internal sealed class OcrWorker : IDisposable
{
    private const int MaximumBytes = 32 * 1024 * 1024;
    private static readonly JsonSerializerOptions Json = new(JsonSerializerDefaults.Web);
    private RapidOcrNative.Engine? cpu;
    private bool gpuAvailable = Environment.GetEnvironmentVariable("WINONE_OCR_DEVICE") != "cpu";

    internal static int RunWorker(string[] args)
    {
        try
        {
            if (args.Length < 2 || !Path.IsPathFullyQualified(args[1]) || args.Skip(2).Any(arg => arg is not ("--serve" or "--pin-layout")))
                throw new ArgumentException("Invalid OCR worker arguments");
            // Set before loading the native DLL, whose CRT reads its environment
            // during initialization. Host-visible heaps can exhaust a discrete
            // GPU's BAR aperture even when its dedicated VRAM has free space.
            Environment.SetEnvironmentVariable("PPOCR_GPU_ARENA_DEVICE_LOCAL", "1");
            Environment.SetEnvironmentVariable("PPOCR_GPU_CONSTANT_CACHE_MB", "0");
            Environment.SetEnvironmentVariable("PPOCR_BENCH_THREADS", Math.Min(8, Math.Max(1, Environment.ProcessorCount / 2)).ToString());
            Environment.SetEnvironmentVariable("PPOCR_REC_FAST", "0");
            // The upstream model reader uses narrow relative filenames. Keep
            // those names ASCII even when the installation directory is Unicode.
            Directory.SetCurrentDirectory(args[1]);
            RapidOcrNative.Load(args[1]);
            using var worker = new OcrWorker();
            using var input = Console.OpenStandardInput();
            bool layout = args.Contains("--pin-layout");
            if (!args.Contains("--serve"))
            {
                using var data = new MemoryStream();
                var buffer = new byte[81920];
                int read;
                while ((read = input.Read(buffer, 0, Math.Min(buffer.Length, MaximumBytes + 1 - (int)data.Length))) > 0)
                    data.Write(buffer, 0, read);
                if (data.Length is 0 or > MaximumBytes) throw new InvalidDataException("Invalid image size");
                return worker.Respond(data.ToArray(), layout) ? 0 : 1;
            }
            var header = new byte[4];
            while (true)
            {
                if (input.Read(header, 0, 1) == 0) return 0;
                input.ReadExactly(header.AsSpan(1));
                uint length = BinaryPrimitives.ReadUInt32LittleEndian(header);
                if (length > MaximumBytes) throw new InvalidDataException("Invalid frame size");
                var data = new byte[length];
                input.ReadExactly(data);
                worker.Respond(data, layout);
            }
        }
        catch (Exception error)
        {
            Reply(new { error = error.GetType().Name });
            return 1;
        }
    }

    private static void Reply(object response)
    {
        Console.WriteLine(JsonSerializer.Serialize(response, Json));
        Console.Out.Flush();
    }

    private bool Respond(byte[] data, bool layout)
    {
        try
        {
            // This also covers engine preparation. Model initialization under
            // Windows background throttling otherwise delays the first request.
            using var interactive = new InteractiveWorkScope();
            if (data.Length == 0)
            {
                using var image = new Bitmap(192, 48);
                using (var graphics = Graphics.FromImage(image))
                {
                    graphics.Clear(Color.White);
                    graphics.DrawString("WinOne OCR 123", SystemFonts.DefaultFont, Brushes.Black, 8, 8);
                }
                Recognize(image, false);
                Reply(new { prepared = true, device = "cpu" });
            }
            else
            {
                // Production callers send PNG. Inspect its header before GDI+
                // can allocate a decompressed image above the existing 40 MP cap.
                if (data.Length >= 24 && data.AsSpan(0, 8).SequenceEqual(new byte[] { 137, 80, 78, 71, 13, 10, 26, 10 })
                    && (ulong)BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(16, 4)) * BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(20, 4)) > 40_000_000)
                    throw new InvalidDataException("Image is too large");
                using var stream = new MemoryStream(data, writable: false);
                using var image = new Bitmap(stream);
                if ((long)image.Width * image.Height > 40_000_000) throw new InvalidDataException("Image is too large");
                var result = Recognize(image, layout);
                if (layout) Reply(new { layout = new PinOcrLayout(result.Lines), device = result.Device });
                else Reply(new { text = result.Text, device = result.Device });
            }
            return true;
        }
        catch (Exception error)
        {
            // Diagnostics contain an error class only, never image/text data.
            Reply(new { error = error.GetType().Name });
            return false;
        }
    }

    private RapidOcrNative.Engine Create(bool gpu)
    {
        RapidOcrNative.InitOptions(out var options, (nuint)Marshal.SizeOf<RapidOcrNative.Options>());
        options.Backend = gpu ? 1 : 0;
        options.DetThreshold = .3f;
        options.DetBoxThreshold = .5f;
        options.DetUnclipRatio = 1.6f;
        options.DetLimitSideLength = 32;
        options.DetLimitType = 1;
        options.DetMean = [.485f, .456f, .406f];
        options.DetStd = [.229f, .224f, .225f];
        options.DetUseDilation = 0;
        options.RecBatchSize = gpu ? 6 : 1;
        options.RecFastMode = 0;
        options.UseCls = 0;
        var engine = RapidOcrNative.Create("models/PP-OCRv6_det_small.onnx",
            "models/PP-OCRv6_rec_small.onnx", "models/dictionary.txt", ref options);
        if (engine.IsInvalid) { engine.Dispose(); throw new InvalidOperationException("OCR model initialization failed"); }
        return engine;
    }

    private (string Text, PinOcrLine[] Lines, string Device) Recognize(Bitmap bitmap, bool layout)
    {
        // System.Drawing supplies BGR rows with alignment; the C ABI consumes
        // contiguous RGB. All decoding stays in memory, using the shared .NET runtime.
        using var image = bitmap.Clone(new Rectangle(0, 0, bitmap.Width, bitmap.Height), PixelFormat.Format24bppRgb);
        var pixels = new byte[image.Width * image.Height * 3];
        var locked = image.LockBits(new Rectangle(0, 0, image.Width, image.Height), ImageLockMode.ReadOnly, PixelFormat.Format24bppRgb);
        try
        {
            for (int row = 0; row < image.Height; row++)
                Marshal.Copy(locked.Scan0 + row * locked.Stride, pixels, row * image.Width * 3, image.Width * 3);
        }
        finally { image.UnlockBits(locked); }
        for (int index = 0; index < pixels.Length; index += 3)
            (pixels[index], pixels[index + 2]) = (pixels[index + 2], pixels[index]);
        var pinned = GCHandle.Alloc(pixels, GCHandleType.Pinned);
        try
        {
            var source = new RapidOcrNative.Image { Width = image.Width, Height = image.Height, Rgb = pinned.AddrOfPinnedObject() };
            if ((long)image.Width * image.Height >= 1_000_000 && gpuAvailable)
            {
                try
                {
                    using var gpu = Create(true);
                    var result = Run(gpu, ref source, layout, 3);
                    // The native GPU graph supports full OCR. A short-line
                    // rec-only recovery uses the resident CPU model below.
                    if (layout || result.Blocks.Length > 0)
                        return (layout ? "" : OcrReadingOrder.Compose(result.Blocks), result.Lines, "gpu");
                }
                catch (InvalidOperationException)
                {
                    gpuAvailable = false;
                    Console.Error.WriteLine("OCR GPU unavailable, using CPU");
                }
            }
            cpu ??= Create(false);
            var recognized = Run(cpu, ref source, layout, 3);
            if (!layout && recognized.Blocks.Length == 0)
            {
                var fallback = Run(cpu, ref source, false, 2);
                return (string.Join('\n', fallback.Blocks.Select(block => block.Text)).Trim(), [], "cpu");
            }
            return (layout ? "" : OcrReadingOrder.Compose(recognized.Blocks), recognized.Lines, "cpu");
        }
        finally { pinned.Free(); }
    }

    private static (OcrBlock[] Blocks, PinOcrLine[] Lines) Run(RapidOcrNative.Engine engine, ref RapidOcrNative.Image image, bool layout, int tasks)
    {
        int status = RapidOcrNative.Run(engine, ref image, tasks, out var result);
        try
        {
            if (status != 0) throw new InvalidOperationException("OCR inference failed");
            var blocks = new List<OcrBlock>();
            var lines = new List<PinOcrLine>();
            for (nuint index = 0; index < result.Count; index++)
            {
                var item = Marshal.PtrToStructure<RapidOcrNative.TextBox>(result.Items + checked((int)index * Marshal.SizeOf<RapidOcrNative.TextBox>()));
                string text = Marshal.PtrToStringUTF8(item.Text) ?? "";
                if (item.Confidence < .5 || string.IsNullOrWhiteSpace(text)) continue;
                var block = new OcrBlock(text, item.Box.Min(point => point.X), item.Box.Min(point => point.Y), item.Box.Max(point => point.X), item.Box.Max(point => point.Y));
                blocks.Add(block);
                if (!layout) continue;
                var box = Bounds(item.Box, image.Width, image.Height);
                if (box[2] == 0 || box[3] == 0) continue;
                int wordStatus = RapidOcrNative.Words(ref result, index, 1, out var words);
                try
                {
                    if (wordStatus != 0) throw new InvalidOperationException("OCR character positions unavailable");
                    var glyphs = new List<PinGlyph>();
                    for (nuint wordIndex = 0; wordIndex < words.Count; wordIndex++)
                    {
                        var word = Marshal.PtrToStructure<RapidOcrNative.Word>(words.Items + checked((int)wordIndex * Marshal.SizeOf<RapidOcrNative.Word>()));
                        string glyph = Marshal.PtrToStringUTF8(word.Text) ?? "";
                        if (glyph.Length > 0) glyphs.Add(new PinGlyph(glyph, Bounds(word.Box, image.Width, image.Height)));
                    }
                    lines.Add(new PinOcrLine(text, box, glyphs.ToArray()));
                }
                finally { RapidOcrNative.FreeWords(ref words); }
            }
            return (blocks.ToArray(), lines.ToArray());
        }
        finally { RapidOcrNative.FreeResult(ref result); }
    }

    private static int[] Bounds(RapidOcrNative.Point[] points, int width, int height)
    {
        int left = Math.Clamp((int)Math.Floor(points.Min(point => point.X)), 0, width);
        int top = Math.Clamp((int)Math.Floor(points.Min(point => point.Y)), 0, height);
        int right = Math.Clamp((int)Math.Ceiling(points.Max(point => point.X)), left, width);
        int bottom = Math.Clamp((int)Math.Ceiling(points.Max(point => point.Y)), top, height);
        return [left, top, right - left, bottom - top];
    }

    public void Dispose() => cpu?.Dispose();
}
