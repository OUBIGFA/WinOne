// Shared wire layout for native OCR and screenshot translation.
namespace WinOne.Capture.NativeUI;

internal sealed record PinGlyph(string Text, int[] Box);
internal sealed record PinOcrLine(string Text, int[] Box, PinGlyph[] Glyphs);
internal sealed record PinOcrLayout(PinOcrLine[] Lines);
