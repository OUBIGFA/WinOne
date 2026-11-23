// SPDX-License-Identifier: GPL-3.0-only
// C ABI declarations from RapidAI/RapidOCRLite v5.0.0a16 (Apache-2.0).
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;

namespace WinOne.Capture.Ocr;

internal static class RapidOcrNative
{
    private const string Library = "rapidocrlite";

    // Resolve only this library. System and image-composition libraries retain
    // their normal loader behavior. The worker supplies an absolute runtime path.
    internal static void Load(string directory)
    {
        var library = NativeLibrary.Load(Path.Combine(directory, "rapidocrlite.dll"));
        NativeLibrary.SetDllImportResolver(typeof(RapidOcrNative).Assembly,
            (name, _, _) => name == Library ? library : IntPtr.Zero);
    }

    [StructLayout(LayoutKind.Sequential)]
    internal struct Options
    {
        internal int StructSize, Backend;
        internal float DetThreshold, DetBoxThreshold, DetUnclipRatio;
        internal int DetLimitSideLength, RecHeight, RecMaxWidth, RecBatchSize,
            RecBatchWidthBucket, RecParallelism, HybridGraphParallelism,
            DetBatchSize, BatchPreprocessParallelism, ImageBatchParallelism, DetLimitType;
        [MarshalAs(UnmanagedType.ByValArray, SizeConst = 3)] internal float[] DetMean;
        [MarshalAs(UnmanagedType.ByValArray, SizeConst = 3)] internal float[] DetStd;
        [MarshalAs(UnmanagedType.ByValArray, SizeConst = 3)] internal float[] RecMean;
        [MarshalAs(UnmanagedType.ByValArray, SizeConst = 3)] internal float[] RecStd;
        internal int DetUseDilation, DetMaxCandidates, VulkanDeviceIndex,
            RecFastMode, ModelType, UseCls;
        internal float ClsThreshold;
        internal int ClsBatchSize, ClsHeight, ClsWidth, ClsCropLongLines;
        internal IntPtr ClsModel;
    }

    [StructLayout(LayoutKind.Sequential)]
    internal struct Point { internal float X, Y; }

    [StructLayout(LayoutKind.Sequential)]
    internal struct Image { internal int Width, Height; internal IntPtr Rgb; }

    [StructLayout(LayoutKind.Sequential)]
    internal struct Result { internal IntPtr Items; internal nuint Count; }

    [StructLayout(LayoutKind.Sequential)]
    internal struct TextBox
    {
        internal IntPtr Text;
        internal float Confidence;
        internal int X, Y, Width, Height;
        [MarshalAs(UnmanagedType.ByValArray, SizeConst = 4)] internal Point[] Box;
    }

    [StructLayout(LayoutKind.Sequential)]
    internal struct Word
    {
        internal IntPtr Text;
        internal double Confidence;
        [MarshalAs(UnmanagedType.ByValArray, SizeConst = 4)] internal Point[] Box;
    }

    internal sealed class Engine : SafeHandleZeroOrMinusOneIsInvalid
    {
        private Engine() : base(true) { }
        protected override bool ReleaseHandle() { Destroy(handle); return true; }
    }

    [DllImport(Library, EntryPoint = "rpocr_options_init_ex", CallingConvention = CallingConvention.Cdecl)]
    internal static extern void InitOptions(out Options options, nuint size);
    [DllImport(Library, EntryPoint = "rpocr_create", CallingConvention = CallingConvention.Cdecl)]
    internal static extern Engine Create(
        [MarshalAs(UnmanagedType.LPUTF8Str)] string detection,
        [MarshalAs(UnmanagedType.LPUTF8Str)] string recognition,
        [MarshalAs(UnmanagedType.LPUTF8Str)] string dictionary, ref Options options);
    [DllImport(Library, EntryPoint = "rpocr_destroy", CallingConvention = CallingConvention.Cdecl)]
    private static extern void Destroy(IntPtr engine);
    [DllImport(Library, EntryPoint = "rpocr_run_ex", CallingConvention = CallingConvention.Cdecl)]
    internal static extern int Run(Engine engine, ref Image image, int tasks, out Result result);
    [DllImport(Library, EntryPoint = "rpocr_result_words", CallingConvention = CallingConvention.Cdecl)]
    internal static extern int Words(ref Result result, nuint index, int singleChar, out Result words);
    [DllImport(Library, EntryPoint = "rpocr_result_free", CallingConvention = CallingConvention.Cdecl)]
    internal static extern void FreeResult(ref Result result);
    [DllImport(Library, EntryPoint = "rpocr_words_free", CallingConvention = CallingConvention.Cdecl)]
    internal static extern void FreeWords(ref Result words);
}
