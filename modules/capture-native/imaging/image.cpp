// Decode through Windows' native image codecs. Only RGB pixels cross the ABI;
// no managed image library, temporary capture file, or desktop access is used.
#include <windows.h>
#include <wincodec.h>
#include <wrl/client.h>
#include <cstdint>
#include <cstring>

using Microsoft::WRL::ComPtr;
namespace {
struct Apartment {
    HRESULT status = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
    ~Apartment() { if (SUCCEEDED(status)) CoUninitialize(); }
};

HRESULT allocatePixels(UINT width, UINT height, unsigned char** pixels, int* outWidth, int* outHeight) {
    if (width == 0 || height == 0 || static_cast<uint64_t>(width) * height > 40'000'000)
        return E_INVALIDARG;
    *pixels = static_cast<unsigned char*>(CoTaskMemAlloc(static_cast<size_t>(width) * height * 3));
    if (!*pixels) return E_OUTOFMEMORY;
    *outWidth = static_cast<int>(width);
    *outHeight = static_cast<int>(height);
    return S_OK;
}
}

extern "C" __declspec(dllexport) HRESULT __cdecl winone_decode_image(
    const unsigned char* data, unsigned int length, unsigned char** pixels, int* width, int* height) {
    if (!pixels || !width || !height) return E_POINTER;
    *pixels = nullptr; *width = 0; *height = 0;
    if (!data || length == 0 || length > 32 * 1024 * 1024) return E_INVALIDARG;
    Apartment apartment;
    if (FAILED(apartment.status) && apartment.status != RPC_E_CHANGED_MODE) return apartment.status;
    ComPtr<IWICImagingFactory> factory;
    HRESULT result = CoCreateInstance(CLSID_WICImagingFactory, nullptr, CLSCTX_INPROC_SERVER,
                                     IID_PPV_ARGS(&factory));
    if (FAILED(result)) return result;
    ComPtr<IWICStream> stream;
    if (FAILED(result = factory->CreateStream(&stream))) return result;
    if (FAILED(result = stream->InitializeFromMemory(const_cast<BYTE*>(data), length))) return result;
    ComPtr<IWICBitmapDecoder> decoder;
    if (FAILED(result = factory->CreateDecoderFromStream(stream.Get(), nullptr, WICDecodeMetadataCacheOnDemand, &decoder))) return result;
    ComPtr<IWICBitmapFrameDecode> frame;
    if (FAILED(result = decoder->GetFrame(0, &frame))) return result;
    UINT decodedWidth = 0, decodedHeight = 0;
    if (FAILED(result = frame->GetSize(&decodedWidth, &decodedHeight))) return result;
    if (decodedWidth == 0 || decodedHeight == 0 || static_cast<uint64_t>(decodedWidth) * decodedHeight > 40'000'000)
        return E_INVALIDARG;
    ComPtr<IWICFormatConverter> converter;
    if (FAILED(result = factory->CreateFormatConverter(&converter))) return result;
    if (FAILED(result = converter->Initialize(frame.Get(), GUID_WICPixelFormat24bppRGB,
        WICBitmapDitherTypeNone, nullptr, 0, WICBitmapPaletteTypeCustom))) return result;
    if (FAILED(result = allocatePixels(decodedWidth, decodedHeight, pixels, width, height))) return result;
    result = converter->CopyPixels(nullptr, decodedWidth * 3, decodedWidth * decodedHeight * 3, *pixels);
    if (FAILED(result)) { CoTaskMemFree(*pixels); *pixels = nullptr; *width = *height = 0; }
    return result;
}

extern "C" __declspec(dllexport) HRESULT __cdecl winone_prepare_image(
    unsigned char** pixels, int* width, int* height) {
    if (!pixels || !width || !height) return E_POINTER;
    *pixels = nullptr; *width = *height = 0;
    constexpr int imageWidth = 192, imageHeight = 48;
    BITMAPINFO info{};
    info.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    info.bmiHeader.biWidth = imageWidth;
    info.bmiHeader.biHeight = -imageHeight;
    info.bmiHeader.biPlanes = 1;
    info.bmiHeader.biBitCount = 24;
    info.bmiHeader.biCompression = BI_RGB;
    void* bitmapPixels = nullptr;
    HDC dc = CreateCompatibleDC(nullptr);
    if (!dc) return HRESULT_FROM_WIN32(GetLastError());
    HBITMAP bitmap = CreateDIBSection(dc, &info, DIB_RGB_COLORS, &bitmapPixels, nullptr, 0);
    if (!bitmap) { const auto error = GetLastError(); DeleteDC(dc); return HRESULT_FROM_WIN32(error); }
    HGDIOBJ previousBitmap = SelectObject(dc, bitmap);
    HGDIOBJ previousFont = SelectObject(dc, GetStockObject(DEFAULT_GUI_FONT));
    std::memset(bitmapPixels, 255, imageWidth * imageHeight * 3);
    SetBkMode(dc, TRANSPARENT);
    SetTextColor(dc, RGB(0, 0, 0));
    constexpr wchar_t text[] = L"WinOne OCR 123";
    const BOOL drawn = TextOutW(dc, 8, 8, text, static_cast<int>(sizeof(text) / sizeof(wchar_t) - 1));
    GdiFlush(); // Finish batched drawing before reading the DIB's pixel memory.
    HRESULT result = drawn ? allocatePixels(imageWidth, imageHeight, pixels, width, height) : E_FAIL;
    if (SUCCEEDED(result)) {
        const auto* bgr = static_cast<const unsigned char*>(bitmapPixels);
        for (size_t offset = 0; offset < imageWidth * imageHeight * 3; offset += 3) {
            (*pixels)[offset] = bgr[offset + 2];
            (*pixels)[offset + 1] = bgr[offset + 1];
            (*pixels)[offset + 2] = bgr[offset];
        }
    }
    SelectObject(dc, previousFont);
    SelectObject(dc, previousBitmap);
    DeleteObject(bitmap);
    DeleteDC(dc);
    return result;
}
