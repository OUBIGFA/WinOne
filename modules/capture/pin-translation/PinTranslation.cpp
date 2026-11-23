#include "capture/InPlaceTranslateEngine.h"
#include <opencv2/imgproc.hpp>
#include "ocr/OcrEngine.h"
#include <algorithm>
#include <cstdint>
#include <memory>

using namespace tools3000::capture;

struct PixelRect { int x, y, width, height; };
struct PinPage {
    cv::Mat original;
    cv::Mat alpha;
    std::vector<PinOcrBlock> blocks;
    std::vector<cv::Rect> codes;
    std::vector<cv::Rect> obstacles;
    std::vector<TranslationParagraph> paragraphs;
    std::string combined;
};

#define PIN_API extern "C" __declspec(dllexport)

// One owned page per explicit translation. All coordinates use original image pixels.
PIN_API PinPage* pin_create(const void* bgra, int width, int height, int stride) noexcept {
    if (!bgra || width <= 0 || height <= 0 || int64_t(width)*height > 40000000 || stride < width*4) return nullptr;
    try {
        auto page = std::make_unique<PinPage>();
        cv::Mat pixels(height, width, CV_8UC4, const_cast<void*>(bgra), stride);
        cv::cvtColor(pixels, page->original, cv::COLOR_BGRA2BGR);
        cv::extractChannel(pixels, page->alpha, 3);
        // Keep local code contents out of translation input and guard their pixels.
        for (const auto& code : tools3000::ocr::OcrEngine::detectQrCodes(page->original))
            page->codes.push_back(code.boundingBox);
        page->obstacles = page->codes;
        return page.release();
    } catch (...) { return nullptr; }
}

static cv::Rect clip(const PinPage& page, PixelRect r) {
    const int w = page.original.cols, h = page.original.rows;
    const int x = std::clamp(r.x, 0, w), y = std::clamp(r.y, 0, h);
    const int right = static_cast<int>(std::clamp<int64_t>(int64_t(r.x) + r.width, x, w));
    const int bottom = static_cast<int>(std::clamp<int64_t>(int64_t(r.y) + r.height, y, h));
    return {x, y, right-x, bottom-y};
}
PIN_API int pin_obstacle(PinPage* page, PixelRect bounds) noexcept {
    if (!page) return 0;
    try {
        const auto box = clip(*page, bounds);
        if (!box.empty()) page->obstacles.push_back(box);
        return 1;
    } catch (...) { return 0; }
}
PIN_API int pin_add(PinPage* page, const char* text, PixelRect bounds, const PixelRect* chars, int count,
                    int region, int row) noexcept {
    if (!page || !text || count < 0 || (count && !chars)) return 0;
    try {
        auto box = clip(*page, bounds);
        if (box.empty()) return 0;
        for (const auto& code : page->codes) {
            if ((code & box).area() >= box.area() * .5) return 1;
        }
        PinOcrBlock block{text, box, {}, region, row};
        for (int i=0; i<count; ++i) block.charBoxes.push_back(clip(*page, chars[i]));
        page->blocks.push_back(std::move(block));
        return 1;
    } catch (...) { return 0; }
}
PIN_API int pin_prepare(PinPage* page) noexcept {
    if (!page) return -1;
    try {
        using tools3000::ocr::OcrEngine;
        std::vector<tools3000::ocr::OcrResult> raw;
        for (auto& block : page->blocks) {
            if (block.charBoxes.size() != OcrEngine::countUtf8Codepoints(block.text))
                block.charBoxes = OcrEngine::buildProportionalCharBoxes(block.text, block.origBox);
            raw.push_back({block.text, 1.f, block.origBox, block.charBoxes, block.layoutRegionId, block.alignedRowId});
        }
        auto ordered = OcrEngine::assembleRecognizedBlocksWithLayout(std::move(raw), page->original);
        auto cells = OcrEngine::splitIntoAtomicBlocks(ordered);
        std::vector<PinOcrBlock> atomic;
        for (auto& cell : cells)
            atomic.push_back({std::move(cell.text), cell.box, std::move(cell.charBoxes), cell.layoutRegionId, cell.alignedRowId});
        page->paragraphs = InPlaceTranslateEngine::buildTranslationParagraphs(atomic, page->original.size(), {}, page->original);
        return static_cast<int>(page->paragraphs.size());
    } catch (...) { return -1; }
}
PIN_API const char* pin_text(PinPage* page, int index) noexcept {
    if (!page || index < 0 || static_cast<size_t>(index) >= page->paragraphs.size()) return nullptr;
    return page->paragraphs[index].originalText.c_str();
}
PIN_API int pin_translated(PinPage* page, int index, const char* text) noexcept {
    if (!pin_text(page,index) || !text) return 0;
    try { page->paragraphs[index].translatedText = text; return 1; } catch (...) { return 0; }
}
PIN_API int pin_render(PinPage* page, const char* target, void* output, int stride) noexcept {
    if (!page || !target || !output || stride < page->original.cols*4) return -1;
    try {
        auto rendered = InPlaceTranslateEngine::renderTranslatedCanvas(page->original, page->paragraphs, target,
                                                                       page->obstacles);
        if (!rendered.success || !rendered.renderedCount) return 0;
        cv::Mat pixels(page->original.rows, page->original.cols, CV_8UC4, output, stride);
        cv::cvtColor(rendered.translatedCanvas, pixels, cv::COLOR_BGR2BGRA);
        cv::insertChannel(page->alpha, pixels, 3);
        page->combined = InPlaceTranslateEngine::joinTranslatedTexts(rendered.paragraphs);
        return 1;
    } catch (...) { return -1; }
}
PIN_API const char* pin_combined(PinPage* page) noexcept { return page ? page->combined.c_str() : nullptr; }
PIN_API void pin_destroy(PinPage* page) noexcept { delete page; }
