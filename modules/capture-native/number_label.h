#pragma once
#include "capture/MarkupEngine.h"
#include <opencv2/imgproc.hpp>

namespace winone::number_label {
// Keep the community number badge and community text renderer. The host's
// optional explanation is a text element positioned beside that same badge.
inline tools3000::capture::MarkupElement textElement(const tools3000::capture::MarkupElement& number) {
    using namespace tools3000::capture;
    auto text = number;
    text.tool = MarkupTool::Text;
    const float scale = std::max(6.f, number.fontSize) / 25.6f;
    text.startPt = {number.startPt.x + static_cast<int>(std::ceil(16 * scale + 12)),
                    number.startPt.y - static_cast<int>(std::ceil(number.fontSize * .7f))};
    text.endPt = text.startPt;
    text.color = {255, 255, 255, 255};
    text.textOutline = false; text.hasCustomOutline = false;
    text.isActive = false; text.isEditing = false;
    return text;
}
inline cv::Rect textBounds(const tools3000::capture::MarkupElement& number) {
    using namespace tools3000::capture;
    const auto* text = MarkupToolRegistry::instance().getHandler(MarkupTool::Text);
    if (!text || number.text.empty()) return {};
    return text->getBoundingBox(textElement(number));
}
inline cv::Rect bubbleBounds(const tools3000::capture::MarkupElement& number) {
    const auto text = textBounds(number);
    return text.empty() ? cv::Rect{} : cv::Rect(text.x - 8, text.y - 5, text.width + 16, text.height + 10);
}
inline void render(cv::Mat& canvas, const tools3000::capture::MarkupElement& number) {
    using namespace tools3000::capture;
    if (number.text.empty()) return;
    const auto* text = MarkupToolRegistry::instance().getHandler(MarkupTool::Text);
    if (!text) throw std::runtime_error("Community text renderer is unavailable");
    const auto bubble = bubbleBounds(number) & cv::Rect(0, 0, canvas.cols, canvas.rows);
    if (!bubble.empty()) cv::rectangle(canvas, bubble, cv::Scalar(92, 92, 92, 255), cv::FILLED);
    text->render(canvas, textElement(number));
}
}
