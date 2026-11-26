#pragma once
#include "capture/MarkupEngine.h"
#include "capture/NumberBadgeFormatter.h"
#include "core/utils/WinUtils.h"
#include <algorithm>
#include <cmath>
#include <mutex>
#include <stdexcept>
#include <windows.h>
#include <objidl.h>
#include <gdiplus.h>
#pragma comment(lib, "gdiplus.lib")
#include <opencv2/imgproc.hpp>

namespace winone::number_label {

struct CalloutLayout {
    cv::Rect textRect;
    cv::Rect boxRect;
    cv::Rect bubbleRect;
    int badgeR = 11;
    int gap = 3;
    int tailW = 6;
    int tailHalfH = 5;
    int padX = 9;
    int padY = 4;
    int cornerR = 6;
    int tipX = 0;
    int tipY = 0;
};

inline void ensureGdiplus() {
    static std::once_flag once;
    static ULONG_PTR token = 0;
    std::call_once(once, [] {
        Gdiplus::GdiplusStartupInput input;
        Gdiplus::GdiplusStartup(&token, &input, nullptr);
    });
}

inline tools3000::capture::MarkupColor calloutColor() {
    return {47, 47, 47, 255};
}

inline tools3000::capture::MarkupColor textForegroundColor() {
    return {255, 255, 255, 255};
}

inline tools3000::capture::MarkupColor placeholderColor() {
    return {175, 175, 175, 255};
}

inline bool isPale(const tools3000::capture::MarkupColor& color) {
    return (color.r * 299 + color.g * 587 + color.b * 114) > 170000;
}

inline float badgeScale(const tools3000::capture::MarkupElement& number) {
    return std::max(10.f, number.fontSize) / 22.0f;
}

inline int badgeRadius(const tools3000::capture::MarkupElement& number) {
    return static_cast<int>(std::round(15.0f * badgeScale(number)));
}

inline tools3000::capture::MarkupElement makeTextSample(const tools3000::capture::MarkupElement& number,
                                                       const std::string& sampleText,
                                                       bool ignoreMaxWidth = false) {
    using namespace tools3000::capture;
    auto text = number;
    text.tool = MarkupTool::Text;
    text.text = sampleText;
    text.startPt = {0, 0};
    text.endPt = {0, 0};
    if (ignoreMaxWidth) text.textMaxWidth = 0.0f;
    text.color = textForegroundColor();
    text.textOutline = false;
    text.hasCustomOutline = false;
    text.isActive = false;
    text.isEditing = false;
    text.fill = false;
    return text;
}

inline CalloutLayout layout(const tools3000::capture::MarkupElement& number) {
    using namespace tools3000::capture;
    CalloutLayout out;
    const float scale = badgeScale(number);
    out.badgeR = badgeRadius(number);
    out.gap = std::max(3, static_cast<int>(std::lround(4.0f * scale)));
    out.tailW = std::max(6, static_cast<int>(std::lround(7.0f * scale)));
    out.tailHalfH = std::max(5, static_cast<int>(std::lround(6.0f * scale)));
    // Keep text, selection and caret inside the rounded corners at every size.
    // At the default font this leaves 8 px horizontally and 6 px vertically.
    out.padX = std::max(8, static_cast<int>(std::lround(11.0f * scale)));
    out.padY = std::max(6, static_cast<int>(std::lround(8.0f * scale)));
    out.cornerR = std::max(6, static_cast<int>(std::lround(8.0f * scale)));
    out.tipX = number.startPt.x + out.badgeR + out.gap;

    const auto* textHandler = MarkupToolRegistry::instance().getHandler(MarkupTool::Text);
    if (!textHandler || (number.text.empty() && !number.isEditing)) {
        return out;
    }

    const auto lineSample = makeTextSample(number, "国Ag", true);
    const cv::Rect lineBox = textHandler->getBoundingBox(lineSample);
    const int lineH = number.isEditing && number.winoneNumberEditorLineHeight > 0
        ? number.winoneNumberEditorLineHeight : std::max(1, lineBox.height);

    const std::string shownText = number.text.empty() ? "输入说明" : number.text;
    const auto textSample = makeTextSample(number, shownText, false);
    auto measured = textHandler->getBoundingBox(textSample);
    // During editing RichEdit owns its line metrics and caret space. Keeping
    // the bubble around that real window avoids clipped rows and extra blocks.
    if (number.isEditing && number.winoneNumberEditorSize.width > 0) {
        measured.width = number.winoneNumberEditorSize.width;
        measured.height = number.winoneNumberEditorSize.height;
    }

    const int textW = std::max(measured.width, number.text.empty() ? std::max(72, static_cast<int>(std::ceil(number.fontSize * 4.5f))) : 1);
    const int textH = std::max(lineH, measured.height);

    const int firstRowBoxH = std::max(lineH + out.padY * 2, out.badgeR * 2);
    const int boxW = textW + out.padX * 2;
    const int boxH = std::max(textH + out.padY * 2, out.badgeR * 2);
    const int boxLeft = out.tipX + out.tailW;
    const int boxTop = number.startPt.y - firstRowBoxH / 2;

    const int textX = boxLeft + out.padX;
    const int textY = (textH <= lineH)
        ? (boxTop + (boxH - textH) / 2 - 1)
        : (boxTop + out.padY);

    out.textRect = cv::Rect(textX, textY, textW, textH);
    out.boxRect = cv::Rect(boxLeft, boxTop, boxW, boxH);
    out.bubbleRect = cv::Rect(out.tipX, boxTop, out.tailW + boxW, boxH);

    const int minTipY = boxTop + out.cornerR + out.tailHalfH;
    const int maxTipY = boxTop + boxH - out.cornerR - out.tailHalfH;
    out.tipY = (minTipY <= maxTipY)
        ? std::clamp(number.startPt.y, minTipY, maxTipY)
        : (boxTop + boxH / 2);

    return out;
}

inline tools3000::capture::MarkupElement textElement(const tools3000::capture::MarkupElement& number) {
    using namespace tools3000::capture;
    const auto l = layout(number);
    auto text = number;
    text.tool = MarkupTool::Text;
    text.startPt = {l.textRect.x, l.textRect.y};
    text.endPt = {l.textRect.x + l.textRect.width, l.textRect.y + l.textRect.height};
    text.color = textForegroundColor();
    text.textOutline = false;
    text.hasCustomOutline = false;
    text.isActive = false;
    text.isEditing = false;
    text.fill = false;
    return text;
}

inline cv::Rect textBounds(const tools3000::capture::MarkupElement& number) {
    return layout(number).textRect;
}

inline cv::Rect bubbleBounds(const tools3000::capture::MarkupElement& number) {
    return layout(number).bubbleRect;
}

inline void blendOverlay(cv::Mat& canvas, const cv::Mat& fgMat, int startX, int startY) {
    if (canvas.empty() || fgMat.empty()) return;
    const int szH = fgMat.rows;
    const int szW = fgMat.cols;
    if (canvas.channels() == 3) {
        for (int y = 0; y < szH; ++y) {
            const int targetY = startY + y;
            if (targetY < 0 || targetY >= canvas.rows) continue;
            auto* rowBg = canvas.ptr<cv::Vec3b>(targetY);
            const auto* rowFg = fgMat.ptr<cv::Vec4b>(y);
            for (int x = 0; x < szW; ++x) {
                const int targetX = startX + x;
                if (targetX < 0 || targetX >= canvas.cols) continue;
                const auto& fg = rowFg[x];
                const float alpha = fg[3] / 255.0f;
                if (alpha > 0.001f) {
                    auto& bg = rowBg[targetX];
                    bg[0] = static_cast<uchar>(std::clamp(static_cast<int>(std::lround(fg[0] * alpha + bg[0] * (1.0f - alpha))), 0, 255));
                    bg[1] = static_cast<uchar>(std::clamp(static_cast<int>(std::lround(fg[1] * alpha + bg[1] * (1.0f - alpha))), 0, 255));
                    bg[2] = static_cast<uchar>(std::clamp(static_cast<int>(std::lround(fg[2] * alpha + bg[2] * (1.0f - alpha))), 0, 255));
                }
            }
        }
    } else if (canvas.channels() == 4) {
        for (int y = 0; y < szH; ++y) {
            const int targetY = startY + y;
            if (targetY < 0 || targetY >= canvas.rows) continue;
            auto* rowBg = canvas.ptr<cv::Vec4b>(targetY);
            const auto* rowFg = fgMat.ptr<cv::Vec4b>(y);
            for (int x = 0; x < szW; ++x) {
                const int targetX = startX + x;
                if (targetX < 0 || targetX >= canvas.cols) continue;
                const auto& fg = rowFg[x];
                const float alpha = fg[3] / 255.0f;
                if (alpha > 0.001f) {
                    auto& bg = rowBg[targetX];
                    bg[0] = static_cast<uchar>(std::clamp(static_cast<int>(std::lround(fg[0] * alpha + bg[0] * (1.0f - alpha))), 0, 255));
                    bg[1] = static_cast<uchar>(std::clamp(static_cast<int>(std::lround(fg[1] * alpha + bg[1] * (1.0f - alpha))), 0, 255));
                    bg[2] = static_cast<uchar>(std::clamp(static_cast<int>(std::lround(fg[2] * alpha + bg[2] * (1.0f - alpha))), 0, 255));
                    bg[3] = 255;
                }
            }
        }
    }
}

inline void renderBackground(cv::Mat& canvas, const tools3000::capture::MarkupElement& number) {
    const auto l = layout(number);
    if (l.boxRect.empty() || l.bubbleRect.empty()) return;
    ensureGdiplus();

    constexpr int margin = 2;
    const int originX = l.bubbleRect.x - margin;
    const int originY = l.bubbleRect.y - margin;
    const int matW = l.bubbleRect.width + margin * 2;
    const int matH = l.bubbleRect.height + margin * 2;
    if (matW <= 0 || matH <= 0) return;

    cv::Mat fgMat(matH, matW, CV_8UC4, cv::Scalar(0, 0, 0, 0));
    {
        Gdiplus::Bitmap bitmap(matW, matH, static_cast<INT>(fgMat.step), PixelFormat32bppARGB, fgMat.data);
        Gdiplus::Graphics g(&bitmap);
        g.SetSmoothingMode(Gdiplus::SmoothingModeHighQuality);
        g.SetPixelOffsetMode(Gdiplus::PixelOffsetModeHighQuality);

        const float x0 = static_cast<float>(l.boxRect.x - originX);
        const float y0 = static_cast<float>(l.boxRect.y - originY);
        const float bw = static_cast<float>(l.boxRect.width);
        const float bh = static_cast<float>(l.boxRect.height);
        const float x1 = x0 + bw;
        const float y1 = y0 + bh;
        const float r = std::min({static_cast<float>(l.cornerR), bw * 0.45f, bh * 0.45f});
        const float d = r * 2.0f;

        const float tipX = static_cast<float>(l.tipX - originX);
        const float tipY = static_cast<float>(l.tipY - originY);
        const float maxHalfH = std::max(2.0f, (bh - d) * 0.5f);
        const float halfH = std::min(static_cast<float>(l.tailHalfH), maxHalfH);
        const float tipTopY = std::max(y0 + r, tipY - halfH);
        const float tipBotY = std::min(y1 - r, tipY + halfH);

        Gdiplus::GraphicsPath path;
        path.AddArc(x0, y0, d, d, 180.0f, 90.0f);
        path.AddLine(x0 + r, y0, x1 - r, y0);
        path.AddArc(x1 - d, y0, d, d, 270.0f, 90.0f);
        path.AddLine(x1, y0 + r, x1, y1 - r);
        path.AddArc(x1 - d, y1 - d, d, d, 0.0f, 90.0f);
        path.AddLine(x1 - r, y1, x0 + r, y1);
        path.AddArc(x0, y1 - d, d, d, 90.0f, 90.0f);
        path.AddLine(x0, y1 - r, x0, tipBotY);
        path.AddLine(x0, tipBotY, tipX, tipY);
        path.AddLine(tipX, tipY, x0, tipTopY);
        path.AddLine(x0, tipTopY, x0, y0 + r);
        path.CloseFigure();

        const auto bg = calloutColor();
        Gdiplus::SolidBrush fillBrush(Gdiplus::Color(255, bg.r, bg.g, bg.b));
        g.FillPath(&fillBrush, &path);

        Gdiplus::Pen borderPen(Gdiplus::Color(36, 255, 255, 255), 1.0f);
        g.DrawPath(&borderPen, &path);
    }
    blendOverlay(canvas, fgMat, originX, originY);
}

inline void render(cv::Mat& canvas, const tools3000::capture::MarkupElement& number) {
    using namespace tools3000::capture;
    // The native control owns all text/selection/caret drawing while editing.
    if (number.isEditing) return;
    if (number.text.empty() && !number.isEditing) return;
    const auto* text = MarkupToolRegistry::instance().getHandler(MarkupTool::Text);
    if (!text) throw std::runtime_error("Community text renderer is unavailable");
    auto label = textElement(number);
    if (label.text.empty()) {
        label.text = "输入说明";
        label.color = placeholderColor();
    }
    text->render(canvas, label);
}

}
