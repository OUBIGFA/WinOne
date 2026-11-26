#pragma once
#include "community_hooks.h"
#include "core/config/ConfigManager.h"

namespace winone::community {
// Metadata for the bundled PP-OCRv6 small model. Extra upstream language packs
// are not installed by WinOne; do not query/download a second model tree.
struct OcrModelState {
    std::string installedEngineGeneration() const { return ocrAvailable() ? "v6" : ""; }
    std::string configuredScriptMode() const {
        return tools3000::core::ConfigManager::instance().get<std::string>("/ocr/scriptMode", "auto");
    }
    bool isScriptPackInstalled(const std::string&) const { return false; }
};
inline const OcrModelState& ocrModelState() { static const OcrModelState state; return state; }
}
