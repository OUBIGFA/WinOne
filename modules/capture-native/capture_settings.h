#pragma once
#include "win32_support.h"
#include <set>

namespace winone::capture {
// Validate the Host projection before replacing the last accepted settings.
// Defaults and one-time toolbar adoption match the shipped SettingsStore.
inline json normalizeCaptureSettings(json value) {
    if (!value.is_object()) throw std::runtime_error("截图配置必须为对象");
    const std::set<std::string> fields{"schemaVersion","revision","outputDirectory","copyToClipboard","askWhereToSave","openEditor",
        "pinAfterCapture","pinTranslation","theme","scrollDelay","scrollFromTop","toolbarTools","toolbarVersion","fileNameTemplate"};
    for (const auto& [key, unused] : value.items()) if (!fields.contains(key)) throw std::runtime_error("截图配置包含未知字段：" + key);
    json defaults{{"schemaVersion",1},{"revision",0},{"copyToClipboard",true},{"askWhereToSave",false},{"openEditor",true},
        {"pinAfterCapture",false},{"pinTranslation",false},{"theme","system"},{"scrollDelay",300},{"scrollFromTop",false},
        {"toolbarTools",nullptr},{"toolbarVersion",0},{"fileNameTemplate",nullptr}};
    for (const auto& [key, fallback] : defaults.items()) if (!value.contains(key)) value[key] = fallback;
    if (!value["schemaVersion"].is_number_integer() || value["schemaVersion"] != 1 ||
        !value["revision"].is_number_integer() || value["revision"].get<int64_t>() < 0 ||
        !value.contains("outputDirectory") || !value["outputDirectory"].is_string() ||
        !std::filesystem::path(wide(value["outputDirectory"].get<std::string>())).is_absolute())
        throw std::runtime_error("截图配置版本或保存目录无效");
    for (const auto* key : {"copyToClipboard","askWhereToSave","openEditor","pinAfterCapture","pinTranslation","scrollFromTop"})
        if (!value[key].is_boolean()) throw std::runtime_error("截图开关配置无效");
    if (value["theme"] != "system" && value["theme"] != "dark" && value["theme"] != "light") throw std::runtime_error("截图主题无效");
    if (!value["scrollDelay"].is_number_integer() || value["scrollDelay"].get<int>() < 100 || value["scrollDelay"].get<int>() > 2000)
        throw std::runtime_error("滚动间隔须为 100–2000 毫秒");
    const auto version = value["toolbarVersion"].get<int>();
    if (version != 0 && version != 2 && version != 3) throw std::runtime_error("截图工具栏版本无效");
    auto& tools = value["toolbarTools"];
    if (!tools.is_null()) {
        if (!tools.is_array()) throw std::runtime_error("截图工具栏配置无效");
        std::set<std::string> allowed{"crop","select","rectangle","ellipse","arrow","pen","cover","text","step","mosaic","blur","ocr","translate"};
        for (const auto& tool : tools) if (!tool.is_string() || !allowed.erase(tool.get<std::string>())) throw std::runtime_error("截图工具栏包含未知或重复工具");
        if (version == 0) for (const auto* tool : {"ocr","translate"}) if (allowed.erase(tool)) tools.push_back(tool);
        if (version != 3 && allowed.erase("step")) tools.push_back("step");
    }
    value["toolbarVersion"] = 3;
    if (!value["fileNameTemplate"].is_null() && !value["fileNameTemplate"].is_string()) throw std::runtime_error("文件名模板无效");
    return value;
}
}
