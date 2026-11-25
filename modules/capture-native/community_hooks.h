// Narrow host seams applied to the fixed community source. Pixel selection,
// drawing, editing, scrolling, and pin rendering remain in the upstream engine.
#pragma once
#include <windows.h>
#include <atomic>
#include <stdexcept>
#include "ocr/OcrEngine.h"
#include "core/translate/TranslationTypes.h"

namespace winone::community {
template<class Function> Function endpoint(const char* name) {
    const auto address = GetProcAddress(GetModuleHandleW(nullptr), name);
    if (!address) throw std::runtime_error(std::string("Missing WinOne capture endpoint: ") + name);
    return reinterpret_cast<Function>(address);
}
inline bool ocrAvailable() {
    return endpoint<bool(*)()>("WinOneNativeOcrAvailable")();
}
inline std::vector<tools3000::ocr::OcrResult> ocr(const cv::Mat& image) {
    std::vector<tools3000::ocr::OcrResult> result;
    if (!endpoint<bool(*)(void*, const void*)>("WinOneNativeOcr")(&result, &image))
        throw std::runtime_error("WinOne native OCR failed");
    return result;
}
inline tools3000::core::translate::TranslationBatchResult translate(
    const tools3000::core::translate::TranslationBatchRequest& request, const std::atomic<bool>* cancelled) {
    using Result = tools3000::core::translate::TranslationBatchResult;
    Result result;
    if (!endpoint<bool(*)(void*, const void*, const void*)>("WinOneNativeTranslate")(&result, &request, cancelled))
        throw std::runtime_error("WinOne translation request failed");
    return result;
}
}
