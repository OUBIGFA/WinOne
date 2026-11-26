#include "capture_app.h"
#include "capture/RecordingIndicator.h"
#include "core/events/MainThreadDispatcher.h"
#include "core/utils/OutOfProcessFileDialog.h"
#include "core/logger/Logger.h"
#include "recording_options.h"

namespace winone::capture {
namespace up = tools3000::capture;
using tools3000::core::MainThreadDispatcher;

bool CaptureApp::recordingStatus(void* output, int* state) const {
    if (output) *static_cast<up::RecordStats*>(output) = recording_ ? recording_->communityStats() : up::RecordStats{};
    if (state) *state = static_cast<int>(recording_ ? recording_->communityState() : up::RecordState::Idle);
    return true;
}
void CaptureApp::recordingAction(int action) {
    if (action == 1) finishRecording(true);
    else if (action == 2) finishRecording(false);
}

bool CaptureApp::prepareRecording(bool gif, bool forceDialog) {
    const auto suggested = suggestedPath(gif ? "gif" : "mp4");
    if (!forceDialog && !captureSettings().value(gif ? "gifAskWhereToSave" : "videoAskWhereToSave", true)) {
        recordingDestination_ = suggested; recordingDestinationGif_ = gif; return true;
    }
    tools3000::core::FileDialogRequest dialog;
    dialog.mode = tools3000::core::FileDialogMode::SaveFile;
    dialog.ownerHwnd = up::CaptureOverlay::instance().winoneWindow();
    dialog.title = language_ == "en-US" ? L"Save recording" : L"保存录制";
    dialog.initialDirectory = suggested.parent_path(); dialog.defaultFileName = suggested.filename().wstring();
    dialog.defaultExtension = gif ? L"gif" : L"mp4";
    dialog.filters = {{gif ? L"GIF" : L"MP4", gif ? L"*.gif" : L"*.mp4"}};
#ifdef WINONE_CAPTURE_PRIVATE_PROBE
    wchar_t generated[32768]{};
    if (GetEnvironmentVariableW(L"WINONE_CAPTURE_RECORDING_TEST",generated,32768)) {
        std::filesystem::create_directories(suggested.parent_path()); dialog.autoSelectPathForTest = suggested;
    }
#endif
    const auto started = GetTickCount64();
    LOG_INFO("WinOne recording destination: opening format={}", gif ? "gif" : "mp4");
    const auto chosen = tools3000::core::OutOfProcessFileDialog::show(dialog);
    // Record lifecycle and timing without logging a user's selected filename.
    LOG_INFO("WinOne recording destination: accepted={} cancelled={} worker={} elapsedMs={}",
        chosen.accepted, chosen.cancelled, chosen.ranOutOfProcess, GetTickCount64() - started);
    if (chosen.cancelled) return false;
    if (!chosen.accepted || chosen.selectedPath.empty()) throw std::runtime_error("无法选择录制保存位置");
    if (std::filesystem::exists(chosen.selectedPath)) throw std::runtime_error("文件已存在，请选择新的录制文件名");
    recordingDestination_ = chosen.selectedPath; recordingDestinationGif_ = gif;
    return true;
}

bool CaptureApp::beginRecording(const up::CaptureRegion& region, const up::RecordSetupOptions& options) {
    try {
        if (!up::CaptureOverlay::instance().winoneState().winoneLiveCapture)
            throw std::runtime_error("请重新截取屏幕后再录制");
        const bool gif = options.format == up::RecordFormat::GIF;
        if (!gif && options.format != up::RecordFormat::MP4_H264 && options.format != up::RecordFormat::MP4_H265)
            throw std::runtime_error("请选择 MP4 或 GIF 录制格式");
        // Choosing the destination never starts recording. The toolbar's Start
        // action reaches this method only after its inline settings are ready.
        if (recordingDestination_.empty() || recordingDestinationGif_ != gif) {
            prepareRecording(gif); return false;
        }
        if (std::filesystem::exists(recordingDestination_)) throw std::runtime_error("保存位置已有同名文件，请重新选择保存地址");
        const auto& state = up::CaptureOverlay::instance().winoneState();
        const RecordingOptions selected{gif, state.winoneRecordCursor, options.format == up::RecordFormat::MP4_H265,
            std::clamp(options.fps,1,gif?30:60),state.winoneGifQuality,state.winoneGifSeconds};
        const int width = region.width & ~1, height = region.height & ~1;
        if (width < 16 || height < 16) throw std::runtime_error("录制选区不能小于 16×16 像素");
        RECT bounds{region.x, region.y, region.x + width, region.y + height};
        const auto monitor = MonitorFromRect(&bounds, MONITOR_DEFAULTTONEAREST);
        MONITORINFOEXW info{}; info.cbSize = sizeof(info);
        if (!GetMonitorInfoW(monitor, &info) || bounds.left < info.rcMonitor.left || bounds.top < info.rcMonitor.top ||
            bounds.right > info.rcMonitor.right || bounds.bottom > info.rcMonitor.bottom)
            throw std::runtime_error("请选择同一显示器内的录制区域");
        std::filesystem::create_directories(recordingDestination_.parent_path());
        snapshotDraft(true);
        if (!recording_) recording_ = std::make_unique<RecordingController>(executableDirectory_);
        auto& indicator = up::RecordingIndicator::instance();
        if (!indicator.initialize(GetModuleHandleW(nullptr))) throw std::runtime_error("无法初始化录制控制栏");
        indicator.onPause([this] {
            try {
                const bool paused = recording_->status().value("phase", "idle") == "paused";
                recording_->command(paused ? "resume_recording" : "pause_recording");
                up::RecordingIndicator::instance().setPaused(!paused);
            } catch (const std::exception& error) { notice(error.what()); }
        });
        indicator.onStop([this] { finishRecording(false); });
        indicator.onSnapshot([] { up::RecordingIndicator::instance().toggleWinOnePinned(); });
        indicator.setRecordingRegion(region.x, region.y, width, height);
        indicator.show();
        std::vector<uint64_t> windows;
        EnumThreadWindows(GetCurrentThreadId(), [](HWND window, LPARAM data) -> BOOL {
            static_cast<std::vector<uint64_t>*>(reinterpret_cast<void*>(data))->push_back(reinterpret_cast<uintptr_t>(window));
            return TRUE;
        }, reinterpret_cast<LPARAM>(&windows));
        json configuration{{"x", region.x}, {"y", region.y}, {"width", width}, {"height", height},
            {"monitor", utf8(info.szDevice)}, {"output", utf8(recordingDestination_.wstring())},
            {"format", gif ? "gif" : "mp4"}, {"codec", selected.hevc ? "hevc" : "h264"},
            {"coordinates", "physicalDesktop"}, {"scaleFactor", 1}, {"fps", selected.fps}, {"quality", selected.quality},
            {"gifMaxSeconds", selected.seconds}, {"gifMaxFrames", 1800}, {"cursor", selected.cursor}, {"excludeWindows", windows}};
        try { recording_->command("start_recording", configuration); }
        catch (...) { indicator.hide(); indicator.clearRecordingRegion(); throw; }
        preserving_ = draft_ && !draft_->elements.empty();
        if (!preserving_) retireDraft();
        recordingFinishing_ = false;
        return true;
    } catch (const std::exception& error) { notice(error.what()); return false; }
}

void CaptureApp::finishRecording(bool cancel) {
    if (!recording_ || !recording_->active() || recordingFinishing_) return;
    if (cancel && MessageBoxW(nullptr, language_ == "en-US" ? L"Discard this recording?" : L"放弃本次录制？", L"WinOne",
                              MB_YESNO | MB_ICONQUESTION | MB_DEFBUTTON2) != IDYES) return;
    recordingFinishing_ = true;
    up::RecordingIndicator::instance().showSaving();
    background([this, cancel] {
        try {
            const auto result = recording_->command(cancel ? "cancel_recording" : "stop_recording");
            MainThreadDispatcher::instance().post([this, result] {
                recordingFinishing_ = false;
                auto& indicator = up::RecordingIndicator::instance();
                const auto id = active_; active_.clear();
                if (result.contains("error") && result["error"].is_string()) { indicator.hide(); indicator.clearRecordingRegion(); fail(id, result["error"].get<std::string>()); if (exitAfterRecording_) stop(true); return; }
                complete(id, {{"mode", "recording"}, {"recording", result}});
                // Reuse the current centered control window for completion;
                // destroying it first loses the picture's monitor and anchor.
                if (result.contains("path") && result["path"].is_string())
                    indicator.showCompleted(result["path"].get<std::string>(), result.value("durationMs", 0.0) / 1000);
                else indicator.hide();
                indicator.clearRecordingRegion();
                if (exitAfterRecording_) stop(true);
            });
        } catch (const std::exception& error) {
            const std::string message = error.what();
            MainThreadDispatcher::instance().post([this, message] { recordingFinishing_ = false; notice(message); });
        }
    });
}

void CaptureApp::updateRecording() {
    if (!recording_ || !recording_->active() || recordingFinishing_) return;
    try {
        const auto result = recording_->command("status");
        if (!recording_->active()) {
            auto& indicator = up::RecordingIndicator::instance();
            const auto id = active_; active_.clear();
            if (result.contains("error") && result["error"].is_string()) { indicator.hide(); fail(id, result["error"].get<std::string>()); }
            else {
                complete(id, {{"mode", "recording"}, {"recording", result}});
                if (result.contains("path") && result["path"].is_string())
                    indicator.showCompleted(result["path"].get<std::string>(), result.value("durationMs", 0.0) / 1000);
                else indicator.hide();
            }
            indicator.clearRecordingRegion();
            if (exitAfterRecording_) stop(true);
        }
    } catch (const std::exception& error) { notice(error.what()); }
}
}
