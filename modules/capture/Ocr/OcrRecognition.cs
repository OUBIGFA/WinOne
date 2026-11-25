// SPDX-License-Identifier: GPL-3.0-only
// Shared inference and layout behavior. Both the legacy adapter and the native
// compiled OCR worker use this exact implementation and the pinned models.
using System.Runtime.InteropServices;
using WinOne.Capture.NativeUI;

namespace WinOne.Capture.Ocr;

internal sealed class OcrRecognition : IDisposable
{
    private RapidOcrNative.Engine? cpu;
    private bool gpuAvailable = Environment.GetEnvironmentVariable("WINONE_OCR_DEVICE") != "cpu";
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

    internal (string Text, PinOcrLine[] Lines, string Device) Recognize(byte[] pixels, int width, int height, bool layout)
    {
        if (width <= 0 || height <= 0 || (long)width * height > 40_000_000 || pixels.Length != checked(width * height * 3))
            throw new InvalidDataException("Invalid RGB image");
        var pinned = GCHandle.Alloc(pixels, GCHandleType.Pinned);
        try
        {
            var source = new RapidOcrNative.Image { Width = width, Height = height, Rgb = pinned.AddrOfPinnedObject() };
            if ((long)width * height >= 1_000_000 && gpuAvailable)
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
