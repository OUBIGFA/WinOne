using System.Runtime.InteropServices;

namespace WinOne.Capture.Ocr;

// Native Windows codec adapter. The OCR and paragraph algorithms consume the
// same tightly packed RGB pixels as before, without System.Drawing or WinForms.
internal sealed record NativeImage(int Width, int Height, byte[] Pixels)
{
    private const string Library = "WinOne.NativeImage.dll";
    [DllImport(Library, EntryPoint = "winone_decode_image", CallingConvention = CallingConvention.Cdecl)]
    private static extern int DecodeNative(byte[] data, uint length, out IntPtr pixels, out int width, out int height);
    [DllImport(Library, EntryPoint = "winone_prepare_image", CallingConvention = CallingConvention.Cdecl)]
    private static extern int PrepareNative(out IntPtr pixels, out int width, out int height);

    internal static NativeImage Decode(byte[] data)
    {
        if (data.Length is 0 or > 32 * 1024 * 1024) throw new InvalidDataException("Invalid image size");
        var result = DecodeNative(data, checked((uint)data.Length), out var pixels, out var width, out var height);
        return Take(result, pixels, width, height);
    }

    internal static NativeImage Prepare()
    {
        var result = PrepareNative(out var pixels, out var width, out var height);
        return Take(result, pixels, width, height);
    }

    private static NativeImage Take(int result, IntPtr pixels, int width, int height)
    {
        try {
            if (result < 0 || pixels == IntPtr.Zero || width <= 0 || height <= 0 || (long)width * height > 40_000_000)
                throw new InvalidDataException("Native image decoding failed");
            var data = new byte[checked(width * height * 3)];
            Marshal.Copy(pixels, data, 0, data.Length);
            return new(width, height, data);
        } finally { if (pixels != IntPtr.Zero) Marshal.FreeCoTaskMem(pixels); }
    }
}
