#pragma once
#include "community_hooks.h"
#include "capture/ScreenRecorder.h"

namespace winone::community {
inline tools3000::capture::RecordStats recordingStats() {
    tools3000::capture::RecordStats stats;
    if (!endpoint<bool(*)(void*, int*)>("WinOneNativeRecordingStatus")(&stats, nullptr))
        throw std::runtime_error("Cannot read WinOne recording status");
    return stats;
}
inline tools3000::capture::RecordState recordingState() {
    int state = 0;
    if (!endpoint<bool(*)(void*, int*)>("WinOneNativeRecordingStatus")(nullptr, &state))
        throw std::runtime_error("Cannot read WinOne recording state");
    return static_cast<tools3000::capture::RecordState>(state);
}
}
