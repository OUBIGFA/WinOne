#include "recording_controller.h"

namespace winone::capture {
RecordingController::RecordingController(std::filesystem::path directory) {
    auto path = directory / L"WinOne.Recording.dll";
#ifdef WINONE_CAPTURE_PRIVATE_PROBE
    wchar_t generated[32768]{};
    if (GetEnvironmentVariableW(L"WINONE_CAPTURE_RECORDING_TEST", generated, 32768)) path = generated;
#endif
    library_ = LoadLibraryExW(path.c_str(), nullptr, LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | LOAD_LIBRARY_SEARCH_DEFAULT_DIRS);
    if (!library_) throw systemError("Cannot load WinOne recording engine");
#ifdef WINONE_CAPTURE_PRIVATE_PROBE
    if (path.filename() != L"WinOne.Recording.dll" && !GetProcAddress(library_, "winone_recording_synthetic_test"))
        throw std::runtime_error("Generated recording test runtime is not marked");
#endif
    execute_ = reinterpret_cast<Command>(GetProcAddress(library_, "winone_recording_command"));
    free_ = reinterpret_cast<Free>(GetProcAddress(library_, "winone_recording_response_free"));
    if (!execute_ || !free_) { FreeLibrary(library_); library_ = nullptr; throw std::runtime_error("Invalid WinOne recording interface"); }
}
RecordingController::~RecordingController() {
    if (library_) {
        // The UI first requests a normal save. Releasing an active session here
        // also stops through the engine's existing finalization path.
        try { if (active()) command("stop_recording"); } catch (const std::exception&) { OutputDebugStringW(L"WinOne recording finalization failed\n"); }
        FreeLibrary(library_);
    }
}
json RecordingController::command(std::string_view method, const json& configuration) {
    json request{{"method", method}};
    if (!configuration.is_null()) request["config"] = configuration;
    const auto bytes = request.dump();
    char* response = execute_(reinterpret_cast<const uint8_t*>(bytes.data()), bytes.size());
    if (!response) throw std::runtime_error("Recording engine returned no response");
    json value;
    try { value = json::parse(response); } catch (...) { free_(response); throw; }
    free_(response);
    if (!value.at("ok").get<bool>()) throw std::runtime_error(value.value("error", "Recording engine failed"));
    std::lock_guard lock(mutex_);
    status_ = value.at("status");
    return status_;
}
json RecordingController::status() const { std::lock_guard lock(mutex_); return status_; }
bool RecordingController::active() const {
    const auto phase = status().value("phase", "idle");
    return phase == "starting" || phase == "recording" || phase == "paused" || phase == "stopping";
}
tools3000::capture::RecordStats RecordingController::communityStats() const {
    const auto value = status();
    tools3000::capture::RecordStats stats;
    stats.frameCount = static_cast<int>(std::min<uint64_t>(value.value("encodedFrames", uint64_t{0}), INT_MAX));
    stats.droppedFrameCount = static_cast<int>(std::min<uint64_t>(value.value("droppedBackpressure", uint64_t{0}), INT_MAX));
    stats.durationSec = value.value("durationMs", 0.0) / 1000;
    stats.fileSizeBytes = value.value("bytes", int64_t{0});
    stats.captureBackend = "windows-capture";
    stats.encoderName = value.value("format", "mp4") == "gif" ? "gifski" : "Media Foundation";
    stats.stopReason = value.contains("reason") && value["reason"].is_string() ? value["reason"].get<std::string>() : "";
    return stats;
}
tools3000::capture::RecordState RecordingController::communityState() const {
    const auto phase = status().value("phase", "idle");
    if (phase == "paused") return tools3000::capture::RecordState::Paused;
    if (phase == "recording" || phase == "starting" || phase == "stopping") return tools3000::capture::RecordState::Recording;
    return tools3000::capture::RecordState::Idle;
}
}
