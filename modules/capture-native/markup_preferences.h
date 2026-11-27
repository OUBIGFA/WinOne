#pragma once
#include "capture/CaptureState.h"
#include "core/config/ConfigManager.h"

namespace winone::capture {
namespace markup_preferences {
using namespace tools3000::capture;
using nlohmann::json;

inline std::string key(MarkupTool tool, bool cover) {
    const char* name = nullptr;
    if (cover) name = "cover";
    else switch (tool) {
        case MarkupTool::Rectangle: name = "rectangle"; break;
        case MarkupTool::Ellipse: name = "ellipse"; break;
        case MarkupTool::Arrow: name = "arrow"; break;
        case MarkupTool::Pen: name = "pen"; break;
        case MarkupTool::Text: name = "text"; break;
        case MarkupTool::Number: name = "number"; break;
        case MarkupTool::Mosaic: name = "mosaic"; break;
        case MarkupTool::Blur: name = "blur"; break;
        default: return {};
    }
    return std::string("/capture/winoneMarkup/") + name;
}

// Reuse the engine's atomic, coalesced preference writer inside WinOne's data
// directory. Each tool owns its style; a number's size cannot become a pen width.
inline void save(const CaptureState& state) {
    if (!state.isMarkupToolActive) return;
    const auto path = key(state.currentTool, state.winoneCoverTool);
    if (path.empty()) return;
    json value;
#define FIELD(name) value[#name] = state.name
    FIELD(currentStrokeWidth); FIELD(currentLineStyle); FIELD(currentFillMode);
    FIELD(currentElementCornerRadius); FIELD(currentArrowStyle); FIELD(currentMosaicType);
    FIELD(currentTextOutline); FIELD(currentOutlineWidth); FIELD(currentSmoothCurve);
    FIELD(currentLineArrowEnabled); FIELD(currentStartCap); FIELD(currentEndCap);
    FIELD(currentNumberFormat);
#undef FIELD
    value["color"] = {state.currentColor.r, state.currentColor.g, state.currentColor.b, state.currentColor.a};
    const auto outline = state.currentTextOutlineColor;
    value["outlineColor"] = {outline.r, outline.g, outline.b, outline.a};
    auto& config = tools3000::core::ConfigManager::instance();
    // No dirty notification or disk write for drawing with an unchanged style.
    if (config.get<json>(path, nullptr) != value && !config.set(path, value))
        throw std::runtime_error("Cannot save capture annotation preferences");
}

inline void load(CaptureState& state, MarkupTool tool, bool cover = false) {
    const auto path = key(tool, cover);
    if (path.empty()) return;
    auto& config = tools3000::core::ConfigManager::instance();
    const auto value = config.get<json>(path, json::object());
    if (!value.is_object()) throw std::runtime_error("Invalid capture annotation preferences");
    const auto readColor = [&](const char* name, MarkupColor fallback) {
        if (!value.contains(name)) return fallback;
        const auto& rgba = value.at(name);
        if (!rgba.is_array() || rgba.size() != 4) throw std::runtime_error("Invalid capture annotation color");
        for (const auto& channel : rgba)
            if (!channel.is_number_integer() || channel.get<int>() < 0 || channel.get<int>() > 255)
                throw std::runtime_error("Invalid capture annotation color");
        return MarkupColor{rgba[0].get<uint8_t>(), rgba[1].get<uint8_t>(), rgba[2].get<uint8_t>(), rgba[3].get<uint8_t>()};
    };
    state.currentColor = readColor("color", cover ? MarkupColor::Black() : MarkupColor::Red());
    state.currentTextOutlineColor = readColor("outlineColor", MarkupColor::Auto());
    state.currentStrokeWidth = std::clamp(value.value("currentStrokeWidth", tool == MarkupTool::Number ? 16 : 4), 1, 256);
    state.currentFillMode = value.value("currentFillMode", cover);
    state.currentElementCornerRadius = std::clamp(value.value("currentElementCornerRadius", 0.f), 0.f, 256.f);
    state.currentLineStyle = static_cast<LineStyle>(std::clamp(value.value("currentLineStyle", 0), 0, 5));
    state.currentArrowStyle = static_cast<ArrowStyle>(std::clamp(value.value("currentArrowStyle", 2), 0, 5));
    state.currentMosaicType = std::clamp(value.value("currentMosaicType", tool == MarkupTool::Blur ? 1 : 0), 0, 1);
    state.currentTextOutline = value.value("currentTextOutline", config.get<bool>("/capture/markup/text/outline", true));
    state.currentOutlineWidth = std::clamp(value.value("currentOutlineWidth", 2), 0, 12);
    state.currentSmoothCurve = value.value("currentSmoothCurve", false);
    state.currentLineArrowEnabled = value.value("currentLineArrowEnabled", false);
    state.currentStartCap = static_cast<LineCapStyle>(std::clamp(value.value("currentStartCap", 0), 0, 9));
    state.currentEndCap = static_cast<LineCapStyle>(std::clamp(value.value("currentEndCap", 4), 0, 9));
    state.currentNumberFormat = static_cast<NumberFormat>(std::clamp(value.value("currentNumberFormat", config.get<int>("/capture/markup/numberFormat", 0)), 0, static_cast<int>(NumberFormat::COUNT) - 1));
}
}
}
