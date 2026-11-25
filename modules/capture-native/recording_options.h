#pragma once
#include <windows.h>
#include <optional>
#include <string>

namespace winone::capture {
struct RecordingOptions {
    bool gif = false, cursor = true, hevc = false;
    int fps = 30, quality = 85, seconds = 120;
};
// Windows common controls provide the product-specific options absent from
// the community overlay; capture, drawing, and the status bar stay upstream.
std::optional<RecordingOptions> chooseRecordingOptions(HWND owner, RecordingOptions current, bool english);
}
