#pragma once
#include "host_bridge.h"
#include <optional>
namespace winone::capture {
std::optional<json> chooseToolbarOptions(HWND owner, const json& current, bool english);
}
