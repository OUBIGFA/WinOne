#pragma once
#include <d2d1.h>
#include <wrl/client.h>
#include <opencv2/imgcodecs.hpp>
#include <filesystem>
#include <fstream>

// Read the actual community renderer's frame before EndDraw, on the private
// desktop. PrintWindow cannot read an inactive Direct2D HWND surface.
namespace winone::probe {
inline std::filesystem::path renderDestination;
inline void readFrame(ID2D1RenderTarget* target) {
    if (renderDestination.empty()) return;
    Microsoft::WRL::ComPtr<ID2D1GdiInteropRenderTarget> interop;
    if (FAILED(target->QueryInterface(IID_PPV_ARGS(&interop)))) throw std::runtime_error("Render target does not expose its frame");
    HDC source = nullptr;
    if (FAILED(interop->GetDC(D2D1_DC_INITIALIZE_MODE_COPY, &source))) throw std::runtime_error("Cannot read Direct2D frame");
    const auto size = target->GetPixelSize();
    HDC memory = CreateCompatibleDC(source);
    BITMAPINFO info{}; info.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    info.bmiHeader.biWidth = static_cast<LONG>(size.width); info.bmiHeader.biHeight = -static_cast<LONG>(size.height);
    info.bmiHeader.biPlanes = 1; info.bmiHeader.biBitCount = 32; info.bmiHeader.biCompression = BI_RGB;
    void* pixels = nullptr;
    HBITMAP bitmap = CreateDIBSection(memory, &info, DIB_RGB_COLORS, &pixels, nullptr, 0);
    auto previous = SelectObject(memory, bitmap);
    const bool copied = BitBlt(memory, 0, 0, size.width, size.height, source, 0, 0, SRCCOPY) != FALSE;
    GdiFlush();
    cv::Mat image;
    if (copied) image = cv::Mat(size.height, size.width, CV_8UC4, pixels).clone();
    SelectObject(memory, previous); DeleteObject(bitmap); DeleteDC(memory);
    RECT unchanged{}; interop->ReleaseDC(&unchanged);
    if (!copied) throw std::runtime_error("Cannot copy Direct2D frame pixels");
    for (int y = 0; y < image.rows; ++y) for (int x = 0; x < image.cols; ++x) image.at<cv::Vec4b>(y,x)[3] = 255;
    std::vector<uchar> encoded;
    if (!cv::imencode(".png", image, encoded)) throw std::runtime_error("Cannot encode renderer evidence");
    std::ofstream file(renderDestination, std::ios::binary);
    file.write(reinterpret_cast<const char*>(encoded.data()), encoded.size());
    if (!file) throw std::runtime_error("Cannot save renderer evidence");
    renderDestination.clear();
}
}
