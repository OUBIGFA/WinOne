#pragma once
#include "capture/MarkupEngine.h"
#include "capture/NumberBadgeFormatter.h"
#include "core/utils/WinUtils.h"
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
    const bool pale=(number.color.r*299+number.color.g*587+number.color.b*114)>170000;
    text.color = pale ? MarkupColor{32,32,32,255} : MarkupColor{255,255,255,255};
    text.textOutline = false; text.hasCustomOutline = false;
    text.isActive = false; text.isEditing = false; text.fill = false;
    return text;
}
inline cv::Rect textBounds(const tools3000::capture::MarkupElement& number) {
    using namespace tools3000::capture;
    const auto* text = MarkupToolRegistry::instance().getHandler(MarkupTool::Text);
    if (!text || (number.text.empty() && !number.isEditing)) return {};
    auto label = textElement(number);
    if (label.text.empty()) label.text = "输入说明";
    return text->getBoundingBox(label);
}
inline cv::Rect bubbleBounds(const tools3000::capture::MarkupElement& number) {
    const auto text = textBounds(number);
    const int radius = static_cast<int>(std::ceil(std::max(6.f,number.fontSize)*.625f));
    const int left = number.startPt.x-radius-4;
    return text.empty() ? cv::Rect{} : cv::Rect(left, text.y-7, text.x+text.width+10-left, std::max(text.height+14,2*radius+8));
}
inline cv::Scalar background(const tools3000::capture::MarkupElement& number) {
    return {double(number.color.b),double(number.color.g),double(number.color.r),255};
}
inline void rounded(cv::Mat& canvas, cv::Rect rect, cv::Scalar color, int radius) {
    radius=std::max(1,std::min(radius,std::min(rect.width,rect.height)/2));
    cv::rectangle(canvas,{rect.x+radius,rect.y,rect.width-2*radius,rect.height},color,cv::FILLED);
    cv::rectangle(canvas,{rect.x,rect.y+radius,rect.width,rect.height-2*radius},color,cv::FILLED);
    for(int x:{rect.x+radius,rect.x+rect.width-radius-1})for(int y:{rect.y+radius,rect.y+rect.height-radius-1})
        cv::circle(canvas,{x,y},radius,color,cv::FILLED,cv::LINE_AA);
}
inline void renderBackground(cv::Mat& canvas,const tools3000::capture::MarkupElement& number) {
    const auto bubble=bubbleBounds(number);
    if(!bubble.empty()) rounded(canvas,bubble,background(number),std::max(4,static_cast<int>(number.fontSize*.3f)));
}
inline void renderBadge(cv::Mat& canvas,const tools3000::capture::MarkupElement& number) {
    using namespace tools3000::capture;
    const int radius=static_cast<int>(std::ceil(std::max(6.f,number.fontSize)*.625f));
    if(bubbleBounds(number).empty()) rounded(canvas,{number.startPt.x-radius-4,number.startPt.y-radius-4,2*radius+8,2*radius+8},background(number),std::max(4,radius/2));
    const auto* renderer=MarkupToolRegistry::instance().getHandler(MarkupTool::Text);
    auto glyph=textElement(number);
    glyph.text=tools3000::core::WinUtils::wstringToUtf8(formatNumberBadgeLabel(number.numberValue,number.numberFormat));
    glyph.fontSize=number.fontSize;glyph.textMaxWidth=0;glyph.startPt={0,0};glyph.endPt={0,0};
    const auto bounds=renderer->getBoundingBox(glyph);
    const auto bubble=bubbleBounds(number);
    const auto centerY=bubble.empty()?number.startPt.y:bubble.y+bubble.height/2;
    glyph.startPt={number.startPt.x-bounds.width/2,centerY-bounds.height/2};glyph.endPt=glyph.startPt;
    renderer->render(canvas,glyph);
}
inline void render(cv::Mat& canvas, const tools3000::capture::MarkupElement& number) {
    using namespace tools3000::capture;
    if (number.text.empty() && !number.isEditing) return;
    const auto* text = MarkupToolRegistry::instance().getHandler(MarkupTool::Text);
    if (!text) throw std::runtime_error("Community text renderer is unavailable");
    auto label=textElement(number);
    if(label.text.empty()){label.text="输入说明";label.color={190,190,190,255};}
    text->render(canvas, label);
}
}
