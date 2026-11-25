#include "draft_store.h"
#include <opencv2/imgcodecs.hpp>
#include <bcrypt.h>
#include <fstream>
#include <cmath>
#include <set>
#include <objidl.h>
#include <gdiplus.h>

namespace winone::capture {
namespace up = tools3000::capture;
namespace {
std::string digest(std::span<const uint8_t> bytes) {
    uint8_t hash[32]{};
    if (BCryptHash(BCRYPT_SHA256_ALG_HANDLE, nullptr, 0, const_cast<PUCHAR>(bytes.data()),
                   static_cast<ULONG>(bytes.size()), hash, sizeof(hash)) < 0) throw std::runtime_error("Cannot hash capture draft");
    constexpr char digits[] = "0123456789abcdef";
    std::string result;
    for (auto byte : hash) { result += digits[byte >> 4]; result += digits[byte & 15]; }
    return result;
}
bool validId(std::string_view id) { return id.size() == 32 && id.find_first_not_of("0123456789abcdef") == std::string_view::npos; }
std::vector<uint8_t> readBytes(const std::filesystem::path& path, uintmax_t maximum) {
    const auto length = std::filesystem::file_size(path);
    if (length == 0 || length > maximum) throw std::runtime_error("Invalid capture draft size");
    std::ifstream input(path, std::ios::binary);
    std::vector<uint8_t> data(static_cast<size_t>(length));
    if (!input.read(reinterpret_cast<char*>(data.data()), static_cast<std::streamsize>(data.size()))) throw std::runtime_error("Cannot read capture draft");
    return data;
}
void atomicWrite(const std::filesystem::path& path, std::span<const uint8_t> data, bool replace) {
    std::filesystem::create_directories(path.parent_path());
    auto temporary = path; temporary += L"." + wide(identity()) + L".tmp";
    Handle file(CreateFileW(temporary.c_str(), GENERIC_WRITE, 0, nullptr, CREATE_NEW, FILE_FLAG_WRITE_THROUGH, nullptr));
    if (!file) throw systemError("Cannot create capture draft");
    try {
        size_t offset = 0;
        while (offset < data.size()) {
            DWORD written = 0;
            if (!WriteFile(file.get(), data.data() + offset, static_cast<DWORD>(std::min<size_t>(data.size() - offset, 1 << 20)), &written, nullptr) || !written)
                throw systemError("Cannot write capture draft");
            offset += written;
        }
        if (!FlushFileBuffers(file.get())) throw systemError("Cannot finish capture draft");
        file.reset();
        if (!MoveFileExW(temporary.c_str(), path.c_str(), MOVEFILE_WRITE_THROUGH | (replace ? MOVEFILE_REPLACE_EXISTING : 0)))
            throw systemError("Cannot commit capture draft");
    } catch (...) { file.reset(); DeleteFileW(temporary.c_str()); throw; }
}
void atomicJson(const std::filesystem::path& path, const json& value) {
    const auto text = value.dump();
    if (text.size() > 2 * 1024 * 1024) throw std::runtime_error("Capture draft metadata is too large");
    atomicWrite(path, {reinterpret_cast<const uint8_t*>(text.data()), text.size()}, true);
}
json point(cv::Point p) { return {p.x, p.y}; }
cv::Point point(const json& v) { if (!v.is_array() || v.size() != 2) throw std::runtime_error("Invalid capture point"); return {v[0].get<int>(), v[1].get<int>()}; }
json rect(cv::Rect r) { return {r.x, r.y, r.width, r.height}; }
cv::Rect rect(const json& v) { if (!v.is_array() || v.size() != 4) throw std::runtime_error("Invalid capture rectangle"); return {v[0].get<int>(), v[1].get<int>(), v[2].get<int>(), v[3].get<int>()}; }
json color(up::MarkupColor c) { return {c.r, c.g, c.b, c.a}; }
up::MarkupColor color(const json& v) { if (!v.is_array() || v.size() != 4) throw std::runtime_error("Invalid capture color"); return {v[0].get<uint8_t>(), v[1].get<uint8_t>(), v[2].get<uint8_t>(), v[3].get<uint8_t>()}; }
}

json DraftStore::encodeElements(const std::vector<up::MarkupElement>& elements) {
    if (elements.size() > 512) throw std::runtime_error("Too many capture annotations");
    json list = json::array();
    for (const auto& e : elements) {
        json v;
#define FIELD(name) v[#name] = e.name
        FIELD(tool); FIELD(thickness); FIELD(lineStyle); FIELD(fill); FIELD(cornerRadius); FIELD(arrowStyle);
        FIELD(smoothCurve); FIELD(isMultiPoint); FIELD(hasCustomCaps); FIELD(startCap); FIELD(endCap);
        FIELD(text); FIELD(fontSize); FIELD(textMaxWidth); FIELD(hasCustomTextWidth); FIELD(customTextBoxHeight);
        FIELD(textOutline); FIELD(outlineThickness); FIELD(hasCustomOutline); FIELD(textCaret); FIELD(textSelAnchor);
        FIELD(numberValue); FIELD(dpiScale); FIELD(numberShape); FIELD(numberFormat); FIELD(hasLeaderArrow);
        FIELD(mosaicBlockSize); FIELD(magnifierScale); FIELD(magnifierRadius); FIELD(magnifierRadiusY); FIELD(magnifierCalloutStyle);
        FIELD(spotlightDimAlpha); FIELD(spotlightEllipse); FIELD(spotlightShowBorder);
        FIELD(watermarkText); FIELD(watermarkOpacity); FIELD(watermarkAngle); FIELD(watermarkSpacing); FIELD(inpaintRadius);
        FIELD(replaceOriginalText); FIELD(replaceFontClass); FIELD(replaceFontWeight); FIELD(replaceOffsetX); FIELD(replaceOffsetY);
        FIELD(replaceBgMode); FIELD(replaceStyleInitialized); FIELD(replaceReflowRight); FIELD(id); FIELD(isEditing);
#undef FIELD
        v["color"] = color(e.color); v["textOutlineColor"] = color(e.textOutlineColor);
        v["winoneShadow"] = e.winoneShadow; v["winoneShadowColor"] = color(e.winoneShadowColor); v["winoneShadowOffset"] = point(e.winoneShadowOffset);
        v["winoneLegacyText"] = e.winoneLegacyText; v["winoneLegacyLines"] = json::array();
        for(const auto& [text,offset]:e.winoneLegacyLines) v["winoneLegacyLines"].push_back({{"text",text},{"offset",point(offset)}});
        v["startPt"] = point(e.startPt); v["endPt"] = point(e.endPt);
        v["textRenderSize"] = {e.textRenderSize.width, e.textRenderSize.height};
        v["replaceSourceRect"] = rect(e.replaceSourceRect); v["replaceRightTailRect"] = rect(e.replaceRightTailRect);
        v["penPoints"] = json::array(); for (auto p : e.penPoints) v["penPoints"].push_back(point(p));
        v["textSpans"] = json::array(); for (auto r : e.textSpans) v["textSpans"].push_back(rect(r));
        v["replaceCharBoxes"] = json::array(); for (auto r : e.replaceCharBoxes) v["replaceCharBoxes"].push_back(rect(r));
        list.push_back(std::move(v));
    }
    return list;
}

std::vector<up::MarkupElement> DraftStore::decodeElements(const json& values) {
    if (!values.is_array() || values.size() > 512) throw std::runtime_error("Invalid capture annotation list");
    std::vector<up::MarkupElement> result;
    size_t pointCount = 0;
    std::set<uint32_t> identities;
    for (const auto& v : values) {
        up::MarkupElement e;
#define FIELD(name) e.name = v.at(#name).get<decltype(e.name)>()
        FIELD(tool); FIELD(thickness); FIELD(lineStyle); FIELD(fill); FIELD(cornerRadius); FIELD(arrowStyle);
        FIELD(smoothCurve); FIELD(isMultiPoint); FIELD(hasCustomCaps); FIELD(startCap); FIELD(endCap);
        FIELD(text); FIELD(fontSize); FIELD(textMaxWidth); FIELD(hasCustomTextWidth); FIELD(customTextBoxHeight);
        FIELD(textOutline); FIELD(outlineThickness); FIELD(hasCustomOutline); FIELD(textCaret); FIELD(textSelAnchor);
        FIELD(numberValue); FIELD(dpiScale); FIELD(numberShape); FIELD(numberFormat); FIELD(hasLeaderArrow);
        FIELD(mosaicBlockSize); FIELD(magnifierScale); FIELD(magnifierRadius); FIELD(magnifierRadiusY); FIELD(magnifierCalloutStyle);
        FIELD(spotlightDimAlpha); FIELD(spotlightEllipse); FIELD(spotlightShowBorder);
        FIELD(watermarkText); FIELD(watermarkOpacity); FIELD(watermarkAngle); FIELD(watermarkSpacing); FIELD(inpaintRadius);
        FIELD(replaceOriginalText); FIELD(replaceFontClass); FIELD(replaceFontWeight); FIELD(replaceOffsetX); FIELD(replaceOffsetY);
        FIELD(replaceBgMode); FIELD(replaceStyleInitialized); FIELD(replaceReflowRight); FIELD(id); FIELD(isEditing);
#undef FIELD
        e.color = color(v.at("color")); e.textOutlineColor = color(v.at("textOutlineColor"));
        e.winoneShadow = v.value("winoneShadow", false);
        if (v.contains("winoneShadowColor")) e.winoneShadowColor = color(v.at("winoneShadowColor"));
        if (v.contains("winoneShadowOffset")) e.winoneShadowOffset = point(v.at("winoneShadowOffset"));
        e.winoneLegacyText = v.value("winoneLegacyText", "");
        for(const auto& line:v.value("winoneLegacyLines",json::array())) e.winoneLegacyLines.emplace_back(line.at("text").get<std::string>(),point(line.at("offset")));
        e.startPt = point(v.at("startPt")); e.endPt = point(v.at("endPt"));
        e.textRenderSize = {v.at("textRenderSize")[0].get<int>(), v.at("textRenderSize")[1].get<int>()};
        e.replaceSourceRect = rect(v.at("replaceSourceRect")); e.replaceRightTailRect = rect(v.at("replaceRightTailRect"));
        for (const auto& p : v.at("penPoints")) e.penPoints.push_back(point(p));
        for (const auto& r : v.at("textSpans")) e.textSpans.push_back(rect(r));
        for (const auto& r : v.at("replaceCharBoxes")) e.replaceCharBoxes.push_back(rect(r));
        pointCount += e.penPoints.size() + e.textSpans.size() + e.replaceCharBoxes.size();
        if (static_cast<int>(e.tool) < 0 || static_cast<int>(e.tool) > static_cast<int>(up::MarkupTool::TextReplace) ||
            !std::isfinite(e.thickness) || !std::isfinite(e.fontSize) || e.thickness <= 0 || e.thickness > 256 ||
            e.fontSize < 6 || e.fontSize > 512 || e.text.size() > 16384 || pointCount > 16384 || !identities.insert(e.id).second)
            throw std::runtime_error("Invalid capture annotation data");
        auto coordinate = [](cv::Point p) { return std::abs(static_cast<int64_t>(p.x)) <= 1'000'000 && std::abs(static_cast<int64_t>(p.y)) <= 1'000'000; };
        if (!coordinate(e.startPt) || !coordinate(e.endPt) || std::any_of(e.penPoints.begin(), e.penPoints.end(), [&](auto p) { return !coordinate(p); }))
            throw std::runtime_error("Capture annotation coordinates exceed bounds");
        result.push_back(std::move(e));
    }
    return result;
}

std::optional<Draft> DraftStore::read() const {
    const auto metadata = directory_ / L"native-capture-cpp" / L"current.json";
    if (!std::filesystem::exists(metadata)) {
        // Read the last shipped native-capture document without modifying it.
        // The first successful new-format write becomes authoritative; a failed
        // conversion leaves the old metadata and original pixels intact.
        auto legacyPath = directory_ / L"native-capture" / L"current.json";
        const bool pending = !std::filesystem::exists(legacyPath);
        if (pending) legacyPath = legacyPath.parent_path() / L"pending.json";
        if (!std::filesystem::exists(legacyPath)) return {};
        const auto legacy = json::parse(readBytes(legacyPath, 2 * 1024 * 1024));
        Draft draft;
        draft.id = legacy.at("id").get<std::string>();
        if (!validId(draft.id) || legacy.at("version") != 1) throw std::runtime_error("Invalid previous capture draft");
        if (std::filesystem::exists(directory_ / L"native-capture-cpp/retired" / (wide(draft.id) + L".json"))) return {};
        const auto retiredPath = directory_ / L"native-capture" / L"retired.json";
        if (std::filesystem::exists(retiredPath)) {
            const auto retired = json::parse(readBytes(retiredPath, 2 * 1024 * 1024));
            const auto entries = retired.is_array() ? retired : retired.value("entries", json::array());
            for (const auto& entry : entries) if (entry.value("id", "") == draft.id) return {};
        }
        draft.revision = legacy.at("revision").get<uint64_t>(); draft.mode = legacy.at("mode").get<std::string>(); draft.settings = legacy.at("settings");
        // Match the old encoder lease. A pending PNG is safe only after its
        // writer has exited and atomically published the entire image.
        Handle encodingLease;
        if (pending) {
            const auto leasePath = directory_ / L"native-capture" / L"encoding" / (wide(draft.id) + L".lock");
            if (std::filesystem::exists(leasePath)) {
                encodingLease.reset(CreateFileW(leasePath.c_str(), GENERIC_READ, 0, nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr));
                if (!encodingLease) throw std::runtime_error("旧截图原图仍在保存，请稍后重试");
            }
        }
        auto bytes = readBytes(directory_ / L"native-capture" / L"images" / (wide(draft.id) + L".png"), 256 * 1024 * 1024);
        auto expected = legacy.at("imageHash").get<std::string>();
        std::transform(expected.begin(), expected.end(), expected.begin(), [](unsigned char c) { return static_cast<char>(tolower(c)); });
        if ((!pending && digest(bytes) != expected) || (pending && (!expected.empty() || draft.revision != 0)))
            throw std::runtime_error("Previous capture original image changed");
        draft.original = cv::imdecode(bytes, cv::IMREAD_UNCHANGED);
        if (draft.original.empty() || draft.original.cols != legacy.at("width").get<int>() || draft.original.rows != legacy.at("height").get<int>())
            throw std::runtime_error("Previous capture dimensions changed");
        const auto& document = legacy.at("document");
        auto rectangle = [](const json& r) { return cv::Rect(static_cast<int>(std::lround(r.at("x").get<double>())),
            static_cast<int>(std::lround(r.at("y").get<double>())), static_cast<int>(std::lround(r.at("width").get<double>())),
            static_cast<int>(std::lround(r.at("height").get<double>()))); };
        draft.selection = document.contains("selection") && !document["selection"].is_null()
            ? rectangle(document["selection"]) : cv::Rect(0, 0, draft.original.cols, draft.original.rows);
        if (draft.selection.width < 1 || draft.selection.height < 1 ||
            (draft.selection & cv::Rect(0,0,draft.original.cols,draft.original.rows)) != draft.selection)
            throw std::runtime_error("Previous capture selection is outside original pixels");
        if (legacy.contains("placement") && legacy["placement"].is_object()) {
            const auto& placement = legacy["placement"];
            draft.origin.x = placement.value("x", 0); draft.origin.y = placement.value("y", 0);
        }
        uint32_t id = 1; int number = 1;
        std::map<std::string,uint32_t> textIds;
        for (const auto& layer : document.at("layers")) {
            up::MarkupElement element;
            const auto kind = layer.at("kind").get<std::string>();
            const std::map<std::string, up::MarkupTool> tools{{"rectangle",up::MarkupTool::Rectangle},{"ellipse",up::MarkupTool::Ellipse},
                {"arrow",up::MarkupTool::Arrow},{"pen",up::MarkupTool::Pen},{"cover",up::MarkupTool::Rectangle},
                {"text",up::MarkupTool::Text},{"step",up::MarkupTool::Number},{"mosaic",up::MarkupTool::Mosaic},{"blur",up::MarkupTool::Blur}};
            if (!tools.contains(kind)) throw std::runtime_error("Previous draft contains an unknown annotation");
            element.tool = tools.at(kind); element.id = id++;
            if (layer.contains("textId") && layer["textId"].is_string()) textIds[layer["textId"].get<std::string>()] = element.id;
            auto bounds = rectangle(layer.at("bounds")); bounds.x -= draft.selection.x; bounds.y -= draft.selection.y;
            element.startPt = bounds.tl(); element.endPt = bounds.br();
            const auto argb = static_cast<uint32_t>(layer.value("strokeColor", static_cast<int64_t>(0xffe23b30)));
            element.color = {static_cast<uint8_t>(argb >> 16), static_cast<uint8_t>(argb >> 8), static_cast<uint8_t>(argb), static_cast<uint8_t>(argb >> 24)};
            element.thickness = static_cast<float>(std::max(1, layer.value("strokeWidth", 2)));
            if (layer.contains("legacyStrokeWidth") && !layer["legacyStrokeWidth"].is_null()) element.thickness = layer["legacyStrokeWidth"].get<float>();
            element.cornerRadius = layer.value("cornerRadius", 0.f);
            const int dash = layer.value("dash", 0);
            if (dash < 0 || dash > 4) throw std::runtime_error("Previous line style is invalid");
            element.lineStyle = dash == 4 ? up::LineStyle::DashDotDot : static_cast<up::LineStyle>(dash);
            element.winoneShadow = layer.value("shadow", false);
            const auto shadow = static_cast<uint32_t>(layer.value("shadowColor", int64_t{0x80000000}));
            element.winoneShadowColor = {static_cast<uint8_t>(shadow >> 16),static_cast<uint8_t>(shadow >> 8),static_cast<uint8_t>(shadow),static_cast<uint8_t>(shadow >> 24)};
            if (layer.contains("shadowOffset")) element.winoneShadowOffset = {layer["shadowOffset"].value("x",0),layer["shadowOffset"].value("y",0)};
            element.text = layer.value("text", ""); element.fontSize = layer.value("fontPixels", 24.f); element.textMaxWidth = static_cast<float>(bounds.width);
            element.hasCustomTextWidth = kind == "text"; element.textOutline = false;
            if(layer.contains("lines") && layer["lines"].is_array()) {
                element.winoneLegacyText = element.text;
                for(const auto& line:layer["lines"]) element.winoneLegacyLines.emplace_back(line.at("text").get<std::string>(),cv::Point(line.at("x").get<int>(),line.at("y").get<int>()));
            }
            element.mosaicBlockSize = std::max(1, layer.value("effectSize", 12));
            element.fill = kind == "cover" || kind == "step";
            const auto fill = static_cast<uint32_t>(layer.value("fillColor", int64_t{0}));
            if ((kind == "rectangle" || kind == "ellipse" || kind == "cover" || kind == "step") && (fill >> 24)) {
                element.fill = true; element.hasCustomOutline = true;
                element.textOutlineColor = element.color; element.outlineThickness = layer.value("strokeWidth",0.f);
                element.color = {static_cast<uint8_t>(fill >> 16),static_cast<uint8_t>(fill >> 8),static_cast<uint8_t>(fill),static_cast<uint8_t>(fill >> 24)};
            }
            if (kind == "step") {
                element.numberValue = number++;
                // The shipped document stores the badge/bubble union. Recover
                // its center with the same Segoe UI pixel metrics, not the
                // rectangle's visual center (which shifts on multiline text).
                const int halfBadge = std::max(4, static_cast<int>(std::nearbyint(element.fontSize * .625f)));
                Gdiplus::Font font(L"Segoe UI", element.fontSize, Gdiplus::FontStyleRegular, Gdiplus::UnitPixel);
                if (font.GetLastStatus() != Gdiplus::Ok) throw std::runtime_error("Cannot read previous numbered-step font metrics");
                const int line = static_cast<int>(std::ceil(font.GetHeight(96.f)));
                const int padY = std::max(2, static_cast<int>(std::nearbyint(element.fontSize * .3f)));
                element.startPt = {bounds.x + halfBadge, bounds.y + (bounds.width > 2 * halfBadge ? padY + line / 2 : halfBadge)};
                element.endPt = element.startPt;
            }
            if (kind == "text") {
                const auto textArgb = static_cast<uint32_t>(layer.value("textColor", static_cast<int64_t>(0xffe23b30)));
                element.color = {static_cast<uint8_t>(textArgb >> 16), static_cast<uint8_t>(textArgb >> 8), static_cast<uint8_t>(textArgb), static_cast<uint8_t>(textArgb >> 24)};
            }
            for (const auto& p : layer.value("points", json::array())) element.penPoints.emplace_back(
                static_cast<int>(std::lround(p.at("x").get<double>())) - draft.selection.x,
                static_cast<int>(std::lround(p.at("y").get<double>())) - draft.selection.y);
            if (kind == "arrow" && !element.penPoints.empty()) {
                element.startPt = element.penPoints.front(); element.endPt = element.penPoints.back(); element.arrowStyle = up::ArrowStyle::Standard;
                element.isMultiPoint = element.penPoints.size() > 2; element.smoothCurve = element.isMultiPoint;
                const auto direction = layer.value("arrowHead", 0);
                if (direction < 0 || direction > 2) throw std::runtime_error("Previous arrow direction is invalid");
                element.hasCustomCaps = true;
                element.startCap = direction == 0 ? up::LineCapStyle::None : up::LineCapStyle::SolidTriangle;
                element.endCap = direction == 1 ? up::LineCapStyle::None : up::LineCapStyle::SolidTriangle;
            }
            if (kind == "pen") element.smoothCurve = layer.value("curveInterpolation", false);
            draft.elements.push_back(element);
        }
        if (legacy.contains("text") && legacy["text"].is_object()) {
            const auto& input = legacy["text"];
            up::MarkupElement* element = nullptr;
            if (input.contains("target") && input["target"].is_string()) {
                const auto target = input["target"].get<std::string>();
                if (!textIds.contains(target)) throw std::runtime_error("Previous unfinished text target is missing");
                for (auto& value : draft.elements) if (value.id == textIds.at(target)) element = &value;
            } else {
                up::MarkupElement fresh; fresh.tool = up::MarkupTool::Text; fresh.id = id++;
                draft.elements.push_back(fresh); element = &draft.elements.back();
            }
            if (!element || (element->tool != up::MarkupTool::Text && element->tool != up::MarkupTool::Number))
                throw std::runtime_error("Previous unfinished text target is invalid");
            element->text = input.at("value").get<std::string>();
            element->fontSize = input.at("fontPixels").get<float>();
            element->isEditing = true;
            element->textSelAnchor = input.at("selectionStart").get<size_t>();
            element->textCaret = element->textSelAnchor + input.at("selectionLength").get<size_t>();
            if (element->tool == up::MarkupTool::Text) {
                element->startPt = {input.at("x").get<int>() - draft.selection.x, input.at("y").get<int>() - draft.selection.y};
                element->endPt = element->startPt + cv::Point(input.at("width").get<int>(), input.at("height").get<int>());
                element->textMaxWidth = input.at("width").get<float>(); element->hasCustomTextWidth = true;
                const auto argb = static_cast<uint32_t>(input.at("colorArgb").get<int64_t>());
                element->color = {static_cast<uint8_t>(argb >> 16),static_cast<uint8_t>(argb >> 8),static_cast<uint8_t>(argb),static_cast<uint8_t>(argb >> 24)};
            }
        }
        return draft;
    }
    const auto bytes = readBytes(metadata, 2 * 1024 * 1024);
    const auto value = json::parse(bytes);
    if (value.at("version") != 1 || !validId(value.at("id").get<std::string>())) throw std::runtime_error("Invalid native capture draft identity");
    if (value.value("retired", false)) return {};
    if (std::filesystem::exists(metadata.parent_path() / L"retired" / (wide(value.at("id").get<std::string>()) + L".json"))) return {};
    Draft draft;
    draft.id = value.at("id"); draft.revision = value.at("revision"); draft.mode = value.at("mode"); draft.settings = value.at("settings");
    draft.selection = rect(value.at("selection"));
    draft.origin = {value.at("origin")[0].get<LONG>(), value.at("origin")[1].get<LONG>()};
    const auto image = readBytes(metadata.parent_path() / L"images" / (wide(draft.id) + L".png"), 256 * 1024 * 1024);
    if (digest(image) != value.at("imageSHA256").get<std::string>()) throw std::runtime_error("Native capture original image changed");
    draft.original = cv::imdecode(image, cv::IMREAD_UNCHANGED);
    if (draft.original.empty() || draft.original.cols != value.at("width").get<int>() || draft.original.rows != value.at("height").get<int>())
        throw std::runtime_error("Native capture draft dimensions changed");
    if (draft.selection.width < 0 || draft.selection.height < 0 ||
        (!draft.selection.empty() && (draft.selection & cv::Rect(0,0,draft.original.cols,draft.original.rows)) != draft.selection))
        throw std::runtime_error("Native capture selection is outside original pixels");
    draft.elements = decodeElements(value.at("elements"));
    return draft;
}

void DraftStore::save(const Draft& draft) {
    if (!validId(draft.id) || draft.original.empty()) throw std::runtime_error("Invalid native capture draft");
    const auto folder = directory_ / L"native-capture-cpp";
    const auto retirement = folder / L"retired" / (wide(draft.id) + L".json");
    if (std::filesystem::exists(retirement)) throw std::runtime_error("Capture generation was cancelled before encoding");
    const auto metadata = folder / L"current.json";
    if (std::filesystem::exists(metadata)) {
        const auto current = json::parse(readBytes(metadata, 2 * 1024 * 1024));
        if (!current.value("retired", false) &&
            (current.at("id") != draft.id || current.at("revision").get<uint64_t>() > draft.revision))
            throw std::runtime_error("A newer capture draft must not be overwritten");
        if (current.value("retired", false) && current.at("id") == draft.id)
            throw std::runtime_error("A completed capture draft must not be restored by a stale write");
    }
    const auto image = folder / L"images" / (wide(draft.id) + L".png");
    if (imageId_ != draft.id) {
        std::vector<uchar> encoded;
        if (std::filesystem::exists(image)) encoded = readBytes(image, 256 * 1024 * 1024);
        else {
            if (!cv::imencode(".png", draft.original, encoded)) throw std::runtime_error("Cannot encode capture draft");
            atomicWrite(image, encoded, false);
        }
        imageDigest_ = digest(encoded);
        imageId_ = draft.id;
    }
    // A cancellation marker is committed independently of the large image.
    // It remains authoritative even if a crash interrupts this last check.
    if (std::filesystem::exists(retirement)) return;
    atomicJson(folder / L"current.json", {{"version", 1}, {"id", draft.id}, {"revision", draft.revision},
        {"mode", draft.mode}, {"settings", draft.settings}, {"width", draft.original.cols}, {"height", draft.original.rows},
        {"origin", {draft.origin.x, draft.origin.y}}, {"selection", rect(draft.selection)}, {"imageSHA256", imageDigest_},
        {"elements", encodeElements(draft.elements)}});
}

void DraftStore::retire(std::string_view id, uint64_t revision) {
    const auto current = read();
    if (!current || current->id != id || current->revision != revision) throw std::runtime_error("Capture draft changed before completion");
    // Retirement is committed before deferred image cleanup, so a locked image
    // cannot reappear as an unfinished screenshot at the next launch.
    markRetired(id, revision); finishRetirement(id, revision);
}
void DraftStore::markRetired(std::string_view id, uint64_t revision) const {
    if (!validId(id)) throw std::runtime_error("Invalid retired capture identity");
    atomicJson(directory_ / L"native-capture-cpp/retired" / (wide(id) + L".json"), {{"id",id},{"revision",revision}});
}
void DraftStore::finishRetirement(std::string_view id, uint64_t revision) const {
    const auto folder = directory_ / L"native-capture-cpp";
    const auto metadata = folder / L"current.json";
    if (std::filesystem::exists(metadata)) {
        const auto value = json::parse(readBytes(metadata, 2 * 1024 * 1024));
        if (value.at("id").get<std::string>() != id && !value.value("retired", false)) throw std::runtime_error("Cannot finalize retirement over another draft");
    }
    atomicJson(metadata, {{"version",1},{"id",id},{"revision",revision},{"retired",true}});
    std::filesystem::remove(folder / L"retired" / (wide(id) + L".json"));
}
void DraftStore::cleanupImage(std::string_view id) const {
    if (!validId(id)) throw std::runtime_error("Invalid retired capture identity");
    // Delete only the app-owned immutable cache for the retired generation.
    // Imported source documents and user output images remain separate.
    const auto image = directory_ / L"native-capture-cpp" / L"images" / (wide(id) + L".png");
    std::error_code error; std::filesystem::remove(image, error);
    if (error) throw std::runtime_error("Retired capture image cleanup failed: " + error.message());
}
void DraftStore::recoverRetirements() const {
    const auto folder = directory_ / L"native-capture-cpp";
    const auto pending = folder / L"retired";
    if (!std::filesystem::exists(pending)) return;
    for (const auto& entry : std::filesystem::directory_iterator(pending)) {
        if (!entry.is_regular_file() || entry.path().extension() != L".json") continue;
        const auto value = json::parse(readBytes(entry.path(), 4096));
        const auto id = value.at("id").get<std::string>();
        if (!validId(id) || entry.path().stem() != wide(id)) throw std::runtime_error("Invalid interrupted capture retirement");
        const auto metadata = folder / L"current.json";
        bool ownsCurrent = !std::filesystem::exists(metadata);
        if (!ownsCurrent) ownsCurrent = json::parse(readBytes(metadata, 2 * 1024 * 1024)).at("id") == id;
        if (ownsCurrent) finishRetirement(id, value.at("revision").get<uint64_t>());
        cleanupImage(id);
        if (std::filesystem::exists(entry.path())) std::filesystem::remove(entry.path());
    }
}
}
