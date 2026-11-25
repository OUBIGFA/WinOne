#pragma once
#include "host_bridge.h"
#include "capture/ScreenRecorder.h"
#include <mutex>

namespace winone::capture {
class RecordingController {
public:
    explicit RecordingController(std::filesystem::path directory);
    ~RecordingController();
    json command(std::string_view method, const json& configuration = nullptr);
    json status() const;
    bool active() const;
    tools3000::capture::RecordStats communityStats() const;
    tools3000::capture::RecordState communityState() const;
private:
    HMODULE library_ = nullptr;
    using Command = char*(*)(const uint8_t*, size_t);
    using Free = void(*)(char*);
    Command execute_ = nullptr;
    Free free_ = nullptr;
    mutable std::mutex mutex_;
    json status_{{"phase", "idle"}};
};
}
