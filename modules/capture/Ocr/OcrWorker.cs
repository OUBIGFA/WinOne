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
    private readonly OcrRecognition recognition = new();

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
                bool frameLayout = layout || (length & 0x80000000u) != 0;
                length &= 0x7fffffffu;
                if (length > MaximumBytes) throw new InvalidDataException("Invalid frame size");
                var data = new byte[length];
                input.ReadExactly(data);
                worker.Respond(data, frameLayout);
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
        return recognition.Recognize(pixels, image.Width, image.Height, layout);
    }

    public void Dispose() => recognition.Dispose();
}
