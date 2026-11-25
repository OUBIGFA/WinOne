#include "capture_app.h"
#include "capture/PinWindow.h"
#include "capture/ScrollCaptureOverlay.h"
#include "capture/ShortcutHintOverlay.h"
#include "capture/InPlaceTranslateEngine.h"
#include "capture/RecordingIndicator.h"
#include "core/config/ConfigManager.h"
#include "core/events/MainThreadDispatcher.h"
#include "core/logger/Logger.h"
#include "core/utils/OutOfProcessFileDialog.h"
#include "core/utils/WinUtils.h"
#include <opencv2/imgcodecs.hpp>
#include <fstream>
#include <future>
#include <iomanip>
#include <sstream>
#include <iostream>
#include "toolbar_options.h"
#include "toolbar_icons.h"
#include "capture_settings.h"
#ifdef WINONE_CAPTURE_PRIVATE_PROBE
#include "../../tests/capture-native/render_readback.h"
#endif

namespace winone::capture {
namespace upstream = tools3000::capture;
using tools3000::core::MainThreadDispatcher;

CaptureApp::CaptureApp(std::wstring pipe, std::filesystem::path directory, DWORD parent, std::wstring hostPipe, bool standby)
    : pipe_(std::move(pipe)), directory_(std::move(directory)), parent_(parent), standby_(standby) {
    if (!directory_.is_absolute()) throw std::invalid_argument("Capture data directory must be absolute");
    wchar_t executable[32768]{};
    const auto count = GetModuleFileNameW(nullptr, executable, 32768);
    if (count == 0 || count == 32768) throw systemError("Cannot locate native capture");
    executableDirectory_ = std::filesystem::path(executable).parent_path();
    ocr_ = std::make_unique<OcrClient>(executableDirectory_);
    drafts_ = std::make_unique<DraftStore>(directory_);
    if (parent_) {
        owner_.reset(OpenProcess(SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION, FALSE, parent_));
        if (!owner_) throw systemError("Cannot observe WinOne Host");
    }
    if (!hostPipe.empty()) {
        if (!parent_) throw std::invalid_argument("Host pipe requires an owning process");
        host_ = std::make_unique<HostBridge>(std::move(hostPipe), parent_);
    }
    if (standby_ && !host_) throw std::invalid_argument("Capture standby requires WinOne Host");
}

CaptureApp::~CaptureApp() {
    stoppingServices_.store(true);
    closeNumberEditor(false);
    if (numberBackground_) DeleteObject(numberBackground_);
    operationCancelled_->store(true);
    server_.reset();
    tasks_.clear();
    recording_.reset();
    if (draftWrite_.valid()) { try { draftWrite_.get(); } catch (const std::exception& error) { std::cerr << error.what() << '\n'; } }
    if (retirement_.valid()) { try { retirement_.get(); } catch (const std::exception& error) { std::cerr << error.what() << '\n'; } }
    if (initialized_) {
        upstream::ScrollCapture::instance().shutdown();
        upstream::RecordingIndicator::instance().shutdown();
        upstream::PinWindow::beginModuleShutdown();
        upstream::PinWindow::closeAll();
        if (!upstream::PinWindow::waitForPendingOcrWorkers(5000)) std::cerr << "Capture pin OCR did not finish during shutdown\n";
        upstream::ScreenCapture::instance().shutdown();
        winone::icons::release();
        upstream::shutdownMarkupTextRenderer();
        tools3000::core::WinUtils::releaseThreadDesktopManager();
        tools3000::core::ConfigManager::instance().shutdown();
        MainThreadDispatcher::instance().shutdown();
        tools3000::core::Logger::shutdown();
    }
    if (window_ && IsWindow(window_)) DestroyWindow(window_);
}

int CaptureApp::run(HINSTANCE instance) {
    std::filesystem::create_directories(directory_);
    auto mutexName = L"Local\\WinOneNativeCapture-" + pipe_;
    std::replace(mutexName.begin() + 6, mutexName.end(), L'\\', L'_');
    instanceMutex_.reset(CreateMutexW(nullptr, TRUE, mutexName.c_str()));
    if (!instanceMutex_ || GetLastError() == ERROR_ALREADY_EXISTS) throw std::runtime_error("Capture worker already owns this pipe");
    drafts_->recoverRetirements();
    const auto nativeData = directory_ / L"community-capture";
    std::filesystem::create_directories(nativeData);
    if (!SetEnvironmentVariableW(L"TOOLS3000_DATA_ROOT", nativeData.c_str())) throw systemError("Cannot isolate capture data");
    tools3000::core::LoggerConfig log;
    log.logDir = directory_ / L"logs"; log.logFileName = "capture-native"; log.enableConsole = false;
    tools3000::core::Logger::initialize(log);
    WNDCLASSW type{};
    type.hInstance = instance; type.lpfnWndProc = windowProcedure; type.lpszClassName = L"WinOne.NativeCapture.Dispatcher";
    if (!RegisterClassW(&type) && GetLastError() != ERROR_CLASS_ALREADY_EXISTS) throw systemError("Cannot register capture dispatcher");
    window_ = CreateWindowExW(0, type.lpszClassName, L"WinOne", 0, 0, 0, 0, 0, HWND_MESSAGE, nullptr, instance, this);
    if (!window_) throw systemError("Cannot create capture dispatcher");
    MainThreadDispatcher::instance().initialize(window_);
    if (!tools3000::core::ConfigManager::instance().initialize(nativeData)) throw std::runtime_error("Cannot initialize capture preferences");
    loadSettings(); applySettings(language_);
    if (!upstream::initializeMarkupTextRenderer()) throw std::runtime_error("Cannot initialize community text renderer");
    if (!upstream::ScreenCapture::instance().initialize(instance)) throw std::runtime_error("Cannot initialize community capture engine");
    initialized_ = true;
    auto& overlay = upstream::CaptureOverlay::instance();
    overlay.setWinOneCanvasOwner([this] { return !active_.empty(); });
    overlay.winoneState().winoneBeforeOutput = [this] {
        try { closeNumberEditor(false); if(numberEdit_) return false; snapshotDraft(true); return true; }
        catch (const std::exception& error) { notice(error.what()); return false; }
    };
    overlay.winoneState().winoneSaveAs = [this] {
        try { closeNumberEditor(false); if(numberEdit_) return std::wstring{}; return chooseSavePath(suggestedPath()).wstring(); }
        catch (const std::exception& error) { notice(error.what()); return std::wstring{}; }
    };
    overlay.setWinOneDelivery([this](const auto& region, const auto& image, const auto& completion) {
        return deliver(region, image, completion);
    });
    overlay.setClosedCallback([this] {
        if (recording_ && recording_->active()) return;
        textBusy_ = false;
        upstream::CaptureOverlay::instance().winoneState().winoneProgress = -1;
        if (retaking_ && !preserving_ && draft_) {
            const auto id = active_, expected = draft_->id;
            retaking_ = false;
            MainThreadDispatcher::instance().postDeferred([this, id, expected] {
                try { resumeDraft(id, expected); } catch (const std::exception& error) { fail(id, error.what()); }
            });
            return;
        }
        operationCancelled_->store(true);
        if (!active_.empty() && !preserving_) {
            try { retireDraft(true); } catch (const std::exception& error) { notice(error.what()); }
        }
        if (!active_.empty()) { auto id = active_; active_.clear(); complete(id, {{"cancelled", true}}); }
    });
    overlay.setOcrCallback([this](const auto&, const cv::Mat& image) { textAction(image, false); });
    overlay.setTranslateCallback([this](const auto&, const cv::Mat& image) { textAction(image, true); });
    overlay.setWinOneRecording([this](const auto& region, const auto& options) { return beginRecording(region, options); });
    overlay.winoneState().winoneEditNumber = [this](uint32_t id) {
        MainThreadDispatcher::instance().postDeferred([this, id] { editNumber(id); });
    };
    overlay.winoneState().winoneCustomize = [this] {
        MainThreadDispatcher::instance().postDeferred([this] {
            try {
                if (!host_) throw std::runtime_error("请通过 WinOne 保存工具栏设置");
                const auto selected = chooseToolbarOptions(upstream::CaptureOverlay::instance().winoneWindow(), settings_.value("toolbarTools", json(nullptr)), language_ == "en-US");
                if (!selected) return;
                if (configurationBusy_) return;
                configurationBusy_ = true;
                // Host persists settings through this worker's reload endpoint.
                // Keep the UI dispatcher free while that transaction completes.
                background([this, selected = *selected] {
                    try {
                        auto saved = host_->saveToolbar(selected);
                        MainThreadDispatcher::instance().post([this, saved = std::move(saved)] {
                            configurationBusy_ = false;
                            settings_["toolbarTools"] = saved.at("toolbarTools");
                            settings_["revision"] = saved.at("revision");
                            if (!active_.empty()) sessionSettings_["toolbarTools"] = settings_["toolbarTools"];
                            applySettings(language_);
                            auto& view = upstream::CaptureOverlay::instance();
                            view.winoneState().toolbarLayoutValid = false; view.winoneInvalidate();
                        });
                    } catch (const std::exception& error) {
                        MainThreadDispatcher::instance().post([this, message = std::string(error.what())] {
                            configurationBusy_ = false; notice(message);
                        });
                    }
                });
            } catch (const std::exception& error) { notice(error.what()); }
        });
    };
    overlay.winoneState().winoneScroll = [this](const auto& region) {
        MainThreadDispatcher::instance().postDeferred([this, region] {
            try { startScrolling(region); } catch (const std::exception& error) { notice(error.what()); }
        });
    };
    overlay.winoneState().winoneRetake = [this] {
        MainThreadDispatcher::instance().postDeferred([this] {
            try { retakeScrolling(); } catch (const std::exception& error) { notice(error.what()); }
        });
    };
    overlay.winoneState().winoneSelected = [this] {
        if (mode_ == "ocr" || mode_ == "translate" || mode_ == "scroll" || !captureSettings().value("openEditor", true))
            MainThreadDispatcher::instance().postDeferred([this] {
                if (!active_.empty()) upstream::CaptureOverlay::instance().confirmCurrentSelection(
                    {captureSettings().value("openEditor", true) ? upstream::CaptureCompletionAction::Default : upstream::CaptureCompletionAction::QuickSave});
            });
    };
    activity_ = GetTickCount64(); uiAlive_.store(activity_);
    if (!SetTimer(window_, 1, 250, nullptr)) throw systemError("Cannot start capture lifecycle timer");
    server_ = std::make_unique<PipeServer>(pipe_, [this](auto request) { return handle(request); },
        [this](auto request, auto response) {
            // Shutdown must not destroy the server before its acknowledgment
            // reaches Host. The peer closes after consuming the complete reply.
            if (json::parse(request).value("method", "") == "shutdown" && json::parse(response).contains("result"))
                MainThreadDispatcher::instance().post([this] { stop(false); });
        });
    MSG message{};
    int status;
    while ((status = GetMessageW(&message, nullptr, 0, 0)) > 0) { TranslateMessage(&message); DispatchMessageW(&message); }
    if (status < 0) throw systemError("Capture message loop failed");
    return static_cast<int>(message.wParam);
}

LRESULT CALLBACK CaptureApp::windowProcedure(HWND window, UINT message, WPARAM w, LPARAM l) {
    CaptureApp* app = reinterpret_cast<CaptureApp*>(GetWindowLongPtrW(window, GWLP_USERDATA));
    if (message == WM_NCCREATE) {
        app = static_cast<CaptureApp*>(reinterpret_cast<CREATESTRUCTW*>(l)->lpCreateParams);
        SetWindowLongPtrW(window, GWLP_USERDATA, reinterpret_cast<LONG_PTR>(app)); app->window_ = window;
    }
    return app ? app->message(message, w, l) : DefWindowProcW(window, message, w, l);
}

LRESULT CaptureApp::message(UINT message, WPARAM w, LPARAM l) {
    if (message == MainThreadDispatcher::MessageId) { MainThreadDispatcher::instance().drain(); return 0; }
    if (message == WM_TIMER) {
        std::erase_if(tasks_, [](const auto& task) { return task.finished->load(); });
        uiAlive_.store(GetTickCount64());
        updateRecording();
        if (!active_.empty() && upstream::CaptureOverlay::instance().isOverlayVisible()) {
            try { snapshotDraft(); } catch (const std::exception& error) { notice(error.what()); }
        }
        if (owner_ && WaitForSingleObject(owner_.get(), 0) == WAIT_OBJECT_0) stop(true);
        if (!standby_ && active_.empty() && !configurationBusy_ && !textBusy_ && upstream::PinWindow::count() == 0 && GetTickCount64() - activity_ > 60000) stop(false);
        return 0;
    }
    if (message == WM_CLOSE) { stop(true); return 0; }
    return DefWindowProcW(window_, message, w, l);
}

std::string CaptureApp::handle(std::string_view bytes) {
    std::string requestId;
    try {
        const auto request = json::parse(bytes);
        requestId = request.at("id").get<std::string>();
        if (request.at("version") != 1 || requestId.empty() || requestId.size() > 128)
            throw std::invalid_argument("Invalid capture request identity");
        const auto method = request.at("method").get<std::string>();
        const auto parameters = request.value("params", json::object());
        if (GetTickCount64() - uiAlive_.load() > 20000)
            return json{{"id", requestId}, {"error", {{"code", "unresponsive"}, {"message", "截图界面线程无响应"}}}}.dump();
        if (method == "jobs.get") {
            const auto id = parameters.at("id").get<std::string>();
            const auto wait = std::clamp(parameters.value("waitMs", 0), 0, 2000);
            std::unique_lock lock(jobsMutex_);
            if (!jobs_.contains(id)) throw std::runtime_error("Capture job does not exist");
            if (wait) jobsChanged_.wait_for(lock, std::chrono::milliseconds(wait), [&] { return jobs_.at(id).at("state") != "running"; });
            return json{{"id", requestId}, {"result", jobs_.at(id)}}.dump();
        }
        auto promise = std::make_shared<std::promise<json>>();
        auto expired = std::make_shared<std::atomic<bool>>(false);
        auto future = promise->get_future();
        if (!MainThreadDispatcher::instance().post([this, promise, method, parameters, expired] {
            if (expired->load()) return;
            try { promise->set_value(command(method, parameters)); }
            catch (...) { promise->set_exception(std::current_exception()); }
        })) throw std::runtime_error("Capture UI is stopping");
        if (future.wait_for(std::chrono::seconds(3)) != std::future_status::ready) {
            expired->store(true);
            throw std::runtime_error("Capture UI did not accept the command");
        }
        return json{{"id", requestId}, {"result", future.get()}}.dump();
    } catch (const std::exception& error) {
        return json{{"id", requestId}, {"error", {{"code", "capture"}, {"message", error.what()}}}}.dump();
    }
}

json CaptureApp::job(std::string_view id) const { std::lock_guard lock(jobsMutex_); return jobs_.at(std::string(id)); }
void CaptureApp::background(std::function<void()> action) {
    auto finished = std::make_shared<std::atomic<bool>>(false);
    tasks_.push_back({finished, std::jthread([action = std::move(action), finished] {
        action(); finished->store(true);
    })});
}
json CaptureApp::start(std::function<void(std::string)> action) {
    const auto id = identity();
    { std::lock_guard lock(jobsMutex_); jobs_[id] = {{"id", id}, {"state", "running"}}; }
    MainThreadDispatcher::instance().postDeferred([this, id, action = std::move(action)] {
        try { action(id); } catch (const std::exception& error) { fail(id, error.what()); }
    });
    return job(id);
}
void CaptureApp::complete(const std::string& id, json result) {
    std::lock_guard lock(jobsMutex_);
    jobs_[id] = {{"id", id}, {"state", "completed"}, {"result", std::move(result)}};
    completed_.push_back(id);
    while (completed_.size() > 128) { jobs_.erase(completed_.front()); completed_.pop_front(); }
    jobsChanged_.notify_all();
}
void CaptureApp::fail(const std::string& id, const std::string& error) {
    {
        std::lock_guard lock(jobsMutex_);
        jobs_[id] = {{"id", id}, {"state", "failed"}, {"error", error}};
        completed_.push_back(id);
        while (completed_.size() > 128) { jobs_.erase(completed_.front()); completed_.pop_front(); }
        jobsChanged_.notify_all();
    }
    if (active_ == id) active_.clear();
    notice(error);
}

void CaptureApp::loadSettings() {
    const auto managed = directory_ / L"managed-settings.json";
    const auto filename = std::filesystem::exists(managed) ? managed : directory_ / L"settings.json";
    if (!std::filesystem::exists(filename)) throw std::runtime_error("Capture settings were not supplied by WinOne");
    if (std::filesystem::file_size(filename) > 2 * 1024 * 1024) throw std::runtime_error("Capture settings are too large");
    std::ifstream file(filename, std::ios::binary);
    if (!file) throw std::runtime_error("Cannot read capture settings");
    json next;
    file >> next;
    settings_ = normalizeCaptureSettings(std::move(next));
}

void CaptureApp::applySettings(const std::string& language) {
    if (language != "zh-CN" && language != "en-US") throw std::invalid_argument("Unsupported capture language");
    language_ = language;
    auto& config = tools3000::core::ConfigManager::instance();
    const json capture{{"copyToClipboard", settings_.value("copyToClipboard", true)}, {"saveToFile", false},
        {"pinOnCapture", settings_.value("pinAfterCapture", false)}, {"pinTranslation", settings_.value("pinTranslation", false)}, {"format", "png"},
        {"saveDirectory", settings_.at("outputDirectory")}, {"autoBypassFullscreen", false},
        {"winoneToolbar", settings_.value("toolbarTools", json(nullptr))}};
    config.mergePatch({{"general", {{"language", language}, {"theme", settings_.value("theme", "system")}}}, {"capture", capture}});
}

json CaptureApp::command(std::string method, json parameters) {
#ifdef WINONE_CAPTURE_PRIVATE_PROBE
    if (method == "probe.layout") {
        auto& view = upstream::CaptureOverlay::instance();
        const int width = parameters.at("width"), height = parameters.at("height");
        if (width < 640 || width > 4096 || height < 600 || height > 2160) throw std::runtime_error("Invalid private layout dimensions");
        const float scale = parameters.at("scale");
        if (scale < 1 || scale > 3) throw std::runtime_error("Invalid private layout scale");
        SetWindowPos(view.winoneWindow(), nullptr, 0, 0, width, height, SWP_NOZORDER | SWP_NOACTIVATE);
        if (!view.winoneResizeRenderSurface(width, height)) throw std::runtime_error("Cannot resize private renderer surface");
        view.winoneState().dpiScale = scale; view.winoneState().toolbarLayoutValid = false; view.winoneInvalidate();
        return {{"resized", true}};
    }
    if (method == "probe.discardRecording") {
        MainThreadDispatcher::instance().postDeferred([this] { recordingAction(1); });
        return {{"queued",true}};
    }
    if (method == "probe.render") {
        winone::probe::renderDestination = directory_ / L"rendered.png";
        upstream::CaptureOverlay::instance().winoneInvalidate();
        return {{"queued", true}};
    }
    if (method == "probe.state") {
        auto& overlay = upstream::CaptureOverlay::instance();
        auto& state = overlay.winoneState();
        json buttons = json::array();
        for (const auto& button : state.toolbarButtons)
            buttons.push_back({{"label", utf8(button.tooltipHint.empty() ? button.label : button.tooltipHint)},
                {"command", static_cast<int>(button.command)}, {"x", (button.rect.left + button.rect.right) / 2},
                {"rect", {button.rect.left,button.rect.top,button.rect.right,button.rect.bottom}},
                {"y", (button.rect.top + button.rect.bottom) / 2}, {"secondary", button.isSecondary}});
        const auto pin = upstream::PinWindow::latestInstance();
        const json pinState = pin ? json{{"hwnd",reinterpret_cast<uintptr_t>(pin->hwnd())},{"translated",pin->isShowingTranslation()},
            {"busy",pin->isTranslationInFlight()},{"elements",DraftStore::encodeElements(pin->markupElements())}} : json(nullptr);
        auto& indicator = upstream::RecordingIndicator::instance();
        json controls=json::array();for(int i=0;i<3;++i){const auto point=indicator.winoneControlCenter(i);controls.push_back({point.x,point.y});}
        const json recordingUI{{"hwnd",reinterpret_cast<uintptr_t>(indicator.winoneWindow())},{"pinned",indicator.winonePinned()},{"buttons",controls}};
        auto& scroll = upstream::ScrollCaptureOverlay::instance();
        json scrollButtons=json::array();for(int i=0;i<5;++i){const auto r=scroll.buttonRectForTest(i);scrollButtons.push_back({(r.left+r.right)/2,(r.top+r.bottom)/2});}
        const json scrollUI{{"hwnd",reinterpret_cast<uintptr_t>(scroll.controlWindowHandleForTest())},{"buttons",scrollButtons}};
        return {{"scrollUI",scrollUI},{"recordingUI",recordingUI},{"pin",pinState},{"buttons", buttons}, {"selection", {state.dragStart.x, state.dragStart.y, state.dragEnd.x, state.dragEnd.y}},
                {"moveTool", state.winoneMoveTool}, {"regionTool", state.winoneRegionTool},
                {"longImage", state.isLongImageMode()}, {"zoom", state.longCanvas.zoom},
                {"imageSize", {state.frozenScreen.cols, state.frozenScreen.rows}},
                {"scrolling", upstream::ScrollCapture::instance().isRunning()}, {"scrollFrames", upstream::ScrollCapture::instance().currentFrameCount()},
                {"elements", DraftStore::encodeElements(state.markup.exportElements())}, {"translated", state.winoneShowTranslated},
                {"progress", state.winoneProgress}, {"notice", utf8(state.loupeToastMessage)}, {"textBusy", textBusy_}, {"recording", recording_ ? recording_->status() : json(nullptr)}};
    }
#endif
    if (shuttingDown_) throw std::runtime_error("截图正在退出");
    if (method != "snapshot" && method != "prepare" && method != "settings.reload") activity_ = GetTickCount64();
    if (parameters.contains("language")) applySettings(parameters.at("language").get<std::string>());
    if (method == "prepare") return {{"prepared", initialized_}, {"preparations", initialized_ ? 1 : 0}};
    if (method == "snapshot") return {{"processId", GetCurrentProcessId()}, {"prepared", initialized_}, {"preparations", initialized_ ? 1 : 0}, {"configurationManaged", true},
        {"settings", settings_}, {"lastCapture", last_}, {"busy", !active_.empty() || configurationBusy_}, {"configurationBusy", configurationBusy_},
        {"pins", upstream::PinWindow::count()}};
    if (method == "settings.reload") {
        const bool translated = settings_.value("pinTranslation", false);
        loadSettings(); applySettings(language_);
        if (!active_.empty()) {
            // Appearance, toolbar and the explicit translation switch are live;
            // output and capture behavior belong to the current session snapshot.
            for (const auto* key : {"theme", "toolbarTools", "pinTranslation"})
                if (settings_.contains(key)) sessionSettings_[key] = settings_[key];
        }
        upstream::CaptureOverlay::instance().winoneState().toolbarLayoutValid = false;
        upstream::CaptureOverlay::instance().winoneInvalidate();
        if (translated && !settings_.value("pinTranslation", false)) {
            upstream::PinWindow::winoneResetTranslations();
            auto& state = upstream::CaptureOverlay::instance().winoneState();
            state.winoneTranslated.release(); state.winoneShowTranslated = false;
            operationCancelled_->store(true); operationCancelled_ = std::make_shared<std::atomic<bool>>(false);
            textBusy_ = false; upstream::CaptureOverlay::instance().winoneInvalidate();
        }
        return settings_;
    }
    if (method == "draft.get") {
        if (draft_) return {{"draft", {{"id", draft_->id}, {"revision", draft_->revision}}}};
        if (retirement_.valid()) return {{"draft", nullptr}};
        const auto saved = drafts_->read();
        return {{"draft", saved ? json{{"id", saved->id}, {"revision", saved->revision}} : json(nullptr)}};
    }
    if (method == "draft.resume") {
        const auto expected = parameters.at("expectedId").get<std::string>();
        if (!active_.empty()) {
            if (!draft_ || draft_->id != expected) throw std::runtime_error("截图草稿已变化");
            SetForegroundWindow(upstream::CaptureOverlay::instance().winoneWindow());
            return job(active_);
        }
        auto created = start([this, expected](auto id) { resumeDraft(id, expected); });
        active_ = created.at("id"); return created;
    }
    if (method == "capture.activate") {
        const auto window = upstream::CaptureOverlay::instance().winoneWindow();
        if (window) SetForegroundWindow(window);
        return {{"active", window != nullptr}};
    }
    if (method == "capture.start" || method == "capture.unified") {
        if (!active_.empty()) { const auto window = upstream::CaptureOverlay::instance().winoneWindow(); if (window) SetForegroundWindow(window); return job(active_); }
        const auto mode = parameters.at("mode").get<std::string>();
        if (mode != "region" && mode != "region-native" && mode != "window" && mode != "fullscreen" && mode != "scroll" && mode != "ocr" && mode != "translate")
            throw std::invalid_argument("Unsupported capture mode");
        auto created = start([this, mode](auto id) { beginCapture(id, mode); });
        // Reserve before dispatching the expensive screen acquisition. Another
        // shortcut in the same message batch must activate this same job.
        active_ = created.at("id"); return created;
    }
    if (method == "pins.close") return start([this](auto id) { upstream::PinWindow::closeAll(); complete(id, {{"closed", true}}); });
    if (method == "copy.last" || method == "pin.last" || method == "save.last" || method == "edit.last") {
        const auto expected = parameters.value("expectedId", "");
        if (last_.is_null() || lastImage_.empty() || expected != last_.at("id").get<std::string>()) throw std::runtime_error("截图已变化，请重新加载");
        if (method == "edit.last") {
            if (!active_.empty()) throw std::runtime_error("请先完成当前截图");
            if (!retirement_.valid() && drafts_->read()) throw std::runtime_error("请先完成或放弃当前截图草稿");
            auto created = start([this](auto id) {
                active_ = id; sessionSettings_ = settings_; mode_ = "fullscreen"; draft_.reset(); draftFingerprint_.clear();
                const upstream::CaptureRegion region{0, 0, lastImage_.cols, lastImage_.rows};
                upstream::CaptureOverlay::instance().startEditPinnedMarkup(lastImage_, {}, region,
                    [](bool, const cv::Mat&, const cv::Mat&, std::vector<upstream::MarkupElement>) {});
            });
            active_ = created.at("id"); return created;
        }
        return start([this, method, parameters](auto id) {
            if (method == "copy.last") {
                if (!upstream::ScreenCapture::instance().copyToClipboard(lastImage_)) throw std::runtime_error("无法复制图片");
                complete(id, {{"copied", true}});
            } else if (method == "pin.last") {
                if (!upstream::PinWindow::create(lastImage_, 0, 0, 0, {}, {}, {}, false)) throw std::runtime_error("无法创建贴图");
                complete(id, {{"pinned", true}});
            } else {
                const auto path = saveImage(lastImage_, std::filesystem::path(wide(parameters.at("path").get<std::string>())), true);
                complete(id, {{"path", utf8(path.wstring())}, {"width", lastImage_.cols}, {"height", lastImage_.rows}});
            }
        });
    }
    if (method == "scroll.stop") { upstream::ScrollCapture::instance().requestFinish(); return {{"stopping", true}}; }
    if (method == "scroll.retake") {
        if (active_.empty() || !upstream::CaptureOverlay::instance().isLongImageEditing()) throw std::runtime_error("当前没有可重拍的长截图");
        retakeScrolling(); return job(active_);
    }
    if (method == "shutdown") {
        if (!active_.empty() || configurationBusy_) throw std::runtime_error("请先完成或取消当前截图及设置保存");
        return {{"stopping", true}};
    }
    throw std::runtime_error("Capture operation is not connected: " + method);
}

void CaptureApp::beginCapture(const std::string& id, const std::string& mode) {
    // Discarded generations may still be finishing their first PNG encoding.
    // They no longer own the UI and must not be offered again while retiring.
    if (!retirement_.valid()) if (const auto saved = drafts_->read()) { resumeDraft(id, saved->id); return; }
    active_ = id; sessionSettings_ = settings_; mode_ = mode;
    draft_.reset(); draftFingerprint_.clear(); preserving_ = false;
    textBusy_ = false;
    operationCancelled_->store(true); operationCancelled_ = std::make_shared<std::atomic<bool>>(false);
    upstream::CaptureOptions options;
    options.copyToClipboard = false; options.saveToFile = false; options.recordHistory = false;
    options.autoBypassFullscreen = false;
    if (mode == "fullscreen") {
        options.hasInitialRegion = true;
        options.initialRegion = {GetSystemMetrics(SM_XVIRTUALSCREEN), GetSystemMetrics(SM_YVIRTUALSCREEN),
            GetSystemMetrics(SM_XVIRTUALSCREEN) + GetSystemMetrics(SM_CXVIRTUALSCREEN),
            GetSystemMetrics(SM_YVIRTUALSCREEN) + GetSystemMetrics(SM_CYVIRTUALSCREEN)};
    }
    upstream::CaptureOverlay::instance().startSelection(options);
    if (!upstream::CaptureOverlay::instance().isOverlayVisible()) throw std::runtime_error("无法取得屏幕图像");
    auto& state = upstream::CaptureOverlay::instance().winoneState();
    state.winoneLiveCapture = true;
    state.currentArrowStyle = upstream::ArrowStyle::Standard;
    state.currentTextOutline = false; state.currentTextOutlineColor = {0, 0, 0, 0};
    if (!settings_.value("openEditor", true) && mode == "fullscreen")
        upstream::CaptureOverlay::instance().confirmCurrentSelection({upstream::CaptureCompletionAction::QuickSave});
}

bool CaptureApp::deliver(const upstream::CaptureRegion& region, const cv::Mat& image, const upstream::CaptureCompletion& completion) {
    try {
        if (mode_ == "scroll") { startScrolling(region); return false; }
        if (mode_ == "ocr" || mode_ == "translate") { textAction(image, mode_ == "translate"); return false; }
        const auto result = publish(image, region, completion);
        if (result.is_null()) return false;
        snapshotDraft(true); retireDraft();
        const auto id = active_; active_.clear();
        if (!id.empty()) complete(id, result);
        return true;
    } catch (const std::exception& error) { notice(error.what()); return false; }
}

void CaptureApp::notice(std::string text) {
    auto& overlay = upstream::CaptureOverlay::instance();
    if (overlay.isOverlayVisible()) {
        overlay.winoneState().loupeToastMessage = wide(text);
        overlay.winoneState().loupeToastUntil = GetTickCount() + 5000;
        overlay.winoneInvalidate();
    } else {
        MessageBoxW(nullptr, wide(text).c_str(), L"WinOne", MB_OK | MB_ICONWARNING);
    }
}

void CaptureApp::stop(bool preserve) {
    if (shuttingDown_) return;
    if (recording_ && recording_->active()) { exitAfterRecording_ = true; finishRecording(false); return; }
    if (preserve && !active_.empty()) {
        snapshotDraft(true);
        preserving_ = true;
        upstream::CaptureOverlay::instance().cancel();
    }
    shuttingDown_ = true; operationCancelled_->store(true); KillTimer(window_, 1); PostQuitMessage(0);
}

void CaptureApp::snapshotDraft(bool flush) {
    auto& overlay = upstream::CaptureOverlay::instance();
    auto& state = overlay.winoneState();
    if (retirement_.valid()) {
        if (!flush && retirement_.wait_for(std::chrono::milliseconds(0)) != std::future_status::ready) return;
        retirement_.get();
    }
    if (draftWrite_.valid()) {
        if (!flush && draftWrite_.wait_for(std::chrono::milliseconds(0)) != std::future_status::ready) return;
        try { draftWrite_.get(); }
        catch (...) { draftFingerprint_.clear(); throw; }
    }
    if (state.frozenScreen.empty() || active_.empty() || retaking_) return;
    const auto elements = state.markup.exportElements();
    const cv::Rect selection(std::min(state.dragStart.x, state.dragEnd.x), std::min(state.dragStart.y, state.dragEnd.y),
                            std::abs(state.dragEnd.x - state.dragStart.x), std::abs(state.dragEnd.y - state.dragStart.y));
    const auto fingerprint = json{{"elements", DraftStore::encodeElements(elements)},
        {"selection", {selection.x, selection.y, selection.width, selection.height}}}.dump();
    if (fingerprint == draftFingerprint_) return;
    if (!draftFingerprint_.empty()) {
        operationCancelled_->store(true); operationCancelled_ = std::make_shared<std::atomic<bool>>(false);
        textBusy_ = false;
        state.winoneProgress = -1;
        if (!state.winoneTranslated.empty()) {
            state.winoneTranslated.release(); state.winoneShowTranslated = false; overlay.winoneInvalidate();
        }
    }
    if (!draft_) {
        Draft next;
        next.id = identity(); next.mode = mode_; next.settings = captureSettings();
        next.original = state.frozenScreen.clone();
        RECT bounds{}; GetWindowRect(overlay.winoneWindow(), &bounds); next.origin = {bounds.left, bounds.top};
        draft_ = std::move(next);
    } else ++draft_->revision;
    draft_->selection = selection; draft_->elements = elements;
    draftFingerprint_ = fingerprint;
    const auto snapshot = *draft_;
    draftWrite_ = std::async(std::launch::async, [this, snapshot] { drafts_->save(snapshot); });
    if (flush) draftWrite_.get();
}

void CaptureApp::retireDraft(bool deferred) {
    if (deferred && draft_) {
        const auto id = draft_->id;
        const auto revision = draft_->revision;
        drafts_->markRetired(id, revision);
        auto encoding = std::move(draftWrite_);
        draft_.reset(); draftFingerprint_.clear();
        // Serialize retirement after the in-flight writer, without holding the
        // capture UI. The next snapshot waits for this tail before writing.
        retirement_ = std::async(std::launch::async, [this, id, revision, encoding = std::move(encoding)]() mutable {
            if (encoding.valid()) try { encoding.get(); } catch (const std::exception& error) { std::cerr << error.what() << '\n'; }
            drafts_->finishRetirement(id, revision); drafts_->cleanupImage(id);
        });
        return;
    }
    if (draftWrite_.valid()) draftWrite_.get();
    if (!draft_) return;
    drafts_->retire(draft_->id, draft_->revision);
    const auto retired = draft_->id;
    draft_.reset(); draftFingerprint_.clear();
    background([this, retired] {
        try { drafts_->cleanupImage(retired); }
        catch (const std::exception& error) { std::cerr << error.what() << '\n'; }
    });
}

void CaptureApp::resumeDraft(const std::string& jobId, const std::string& expectedId) {
    auto restored = drafts_->read();
    if (!restored || restored->id != expectedId) throw std::runtime_error("截图草稿已变化，请重新加载");
    active_ = jobId; mode_ = restored->mode; sessionSettings_ = restored->settings;
    for (const auto* key : {"theme", "toolbarTools", "pinTranslation"})
        if (settings_.contains(key)) sessionSettings_[key] = settings_[key];
    applySettings(language_);
    operationCancelled_->store(true); operationCancelled_ = std::make_shared<std::atomic<bool>>(false);
    auto& overlay = upstream::CaptureOverlay::instance();
    preserving_ = false;
    const upstream::CaptureRegion placement{restored->origin.x, restored->origin.y, restored->original.cols, restored->original.rows};
    if (mode_ == "scroll-edit" || mode_ == "scroll") {
        mode_ = "scroll-edit";
        overlay.startEditLongImage(restored->original, 0, [](const cv::Mat&, upstream::ScrollFinishAction, int) {});
    } else {
        overlay.startEditPinnedMarkup(restored->original, {}, placement,
            [](bool, const cv::Mat&, const cv::Mat&, std::vector<upstream::MarkupElement>) {});
    }
    if (!overlay.isOverlayVisible()) throw std::runtime_error("无法恢复截图窗口");
    auto& state = overlay.winoneState();
    state.winoneLiveCapture = false;
    state.dragStart = {restored->selection.x, restored->selection.y};
    state.dragEnd = {restored->selection.x + restored->selection.width, restored->selection.y + restored->selection.height};
    if (restored->selection.width > 0 && restored->selection.height > 0) {
        state.markup.setBaseImage(restored->original(restored->selection));
        state.markup.importElements(restored->elements); state.markupBaseReady = true; state.markupBaseRect = restored->selection;
        state.state = upstream::OverlayState::Selected;
        for (const auto& original : restored->elements) if (original.isEditing) {
            auto* editing = state.markup.getElementById(original.id);
            if (!editing) continue;
            state.activeElement = editing; editing->isActive = true;
            state.currentTool = editing->tool; state.isMarkupToolActive = true;
            if (editing->tool == upstream::MarkupTool::Text) editing->isEditing = true;
            else if (editing->tool == upstream::MarkupTool::Number)
                MainThreadDispatcher::instance().postDeferred([this, id = editing->id] { editNumber(id); });
            break;
        }
    }
    draft_ = std::move(restored); draftFingerprint_.clear(); overlay.winoneInvalidate();
}

bool CaptureApp::ocrAvailable() const {
    return std::filesystem::is_regular_file(executableDirectory_ / L"WinOne.Ocr.exe") &&
        std::filesystem::is_regular_file(executableDirectory_ / L"Ocr" / L"rapidocrlite.dll");
}
std::vector<tools3000::ocr::OcrResult> CaptureApp::recognizeLayout(const cv::Mat& image) { return ocr_->layout(image, &stoppingServices_); }
tools3000::core::translate::TranslationBatchResult CaptureApp::translateBatch(
    const tools3000::core::translate::TranslationBatchRequest& request, const std::atomic<bool>* cancelled) {
    tools3000::core::translate::TranslationBatchResult result;
    if (!host_) { result.errorCode = "host_unavailable"; return result; }
    try {
        result.effectiveTargetLang = request.targetLangOverride.empty() || request.targetLangOverride == "auto"
            ? host_->target(request.texts.empty() ? "" : request.texts.front()) : request.targetLangOverride;
        for (const auto& text : request.texts) {
            if (stoppingServices_.load()) throw std::runtime_error("WinOne is stopping");
            result.translations.push_back(host_->translate(text, result.effectiveTargetLang, cancelled));
        }
        result.success = true; result.providerUsed = "winone";
    } catch (const std::exception& error) { result.errorCode = "host"; result.errorDetail = error.what(); }
    return result;
}
}
