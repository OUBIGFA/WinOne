#pragma once
#include "host_bridge.h"
#include "ocr_client.h"
#include "draft_store.h"
#include "recording_controller.h"
#include "capture/CaptureOverlay.h"
#include "core/translate/TranslationTypes.h"
#include <condition_variable>
#include <deque>
#include <map>
#include <mutex>
#include <future>
#include <commctrl.h>
#include "../../third_party/win32xx/richedit.h"

namespace winone::capture {
class CaptureApp {
public:
    CaptureApp(std::wstring pipe, std::filesystem::path directory, DWORD parent, std::wstring hostPipe, bool standby);
    ~CaptureApp();
    int run(HINSTANCE instance);
    std::vector<tools3000::ocr::OcrResult> recognizeLayout(const cv::Mat& image);
    tools3000::core::translate::TranslationBatchResult translateBatch(
        const tools3000::core::translate::TranslationBatchRequest&, const std::atomic<bool>* cancelled);
    bool ocrAvailable() const;
    bool recordingStatus(void* stats, int* state) const;
    void recordingAction(int action);
    bool savePin(HWND owner, const cv::Mat& image);
private:
    static LRESULT CALLBACK windowProcedure(HWND, UINT, WPARAM, LPARAM);
    LRESULT message(UINT, WPARAM, LPARAM);
    std::string handle(std::string_view request);
    json command(std::string method, json parameters);
    json job(std::string_view id) const;
    json start(std::function<void(std::string)> action);
    void complete(const std::string& id, json result);
    void fail(const std::string& id, const std::string& error);
    void background(std::function<void()> action);
    void loadSettings();
    void applySettings(const std::string& language);
    const json& captureSettings() const { return active_.empty() ? settings_ : sessionSettings_; }
    void beginCapture(const std::string& id, const std::string& mode);
    bool deliver(const tools3000::capture::CaptureRegion&, const cv::Mat&,
                 const tools3000::capture::CaptureCompletion&);
    json publish(const cv::Mat&, const tools3000::capture::CaptureRegion&,
                 const tools3000::capture::CaptureCompletion&);
    void textAction(const cv::Mat& image, bool translate);
    void stop(bool preserve);
    void notice(std::string text);
    std::filesystem::path saveImage(const cv::Mat&, std::filesystem::path destination, bool replace);
    std::filesystem::path chooseSavePath(const std::filesystem::path& suggested, HWND owner = nullptr);
    std::filesystem::path suggestedPath(std::string_view format = "png") const;
    void startScrolling(const tools3000::capture::CaptureRegion& region);
    void prepareScrolling(const tools3000::capture::CaptureRegion& region);
    tools3000::capture::CaptureRegion pendingScrollRegion_{};
    void retakeScrolling();
    void snapshotDraft(bool flush = false);
    void resumeDraft(const std::string& jobId, const std::string& expectedId);
    void retireDraft(bool deferred = false);
    bool beginRecording(const tools3000::capture::CaptureRegion&, const tools3000::capture::RecordSetupOptions&);
    bool prepareRecording(bool gif, bool forceDialog = false);
    std::filesystem::path recordingDestination_;
    bool recordingDestinationGif_ = false;
    void finishRecording(bool cancel);
    void updateRecording();
    void editNumber(uint32_t id);
    void updateNumberEditor();
    void closeNumberEditor(bool cancel);
    static LRESULT CALLBACK numberProcedure(HWND, UINT, WPARAM, LPARAM, UINT_PTR, DWORD_PTR);
    std::wstring pipe_;
    std::filesystem::path directory_, executableDirectory_;
    DWORD parent_;
    bool standby_, shuttingDown_ = false, initialized_ = false;
    Handle owner_;
    Handle instanceMutex_;
    HWND window_ = nullptr;
    std::unique_ptr<PipeServer> server_;
    std::unique_ptr<HostBridge> host_;
    std::unique_ptr<OcrClient> ocr_;
    std::unique_ptr<DraftStore> drafts_;
    std::unique_ptr<RecordingController> recording_;
    bool recordingFinishing_ = false, exitAfterRecording_ = false;
    std::optional<Draft> draft_;
    std::future<void> draftWrite_;
    std::future<void> retirement_;
    std::string draftFingerprint_;
    bool preserving_ = false;
    bool retaking_ = false;
    bool textBusy_ = false;
    HWND numberEdit_ = nullptr;
    std::unique_ptr<winone::community::RichEdit> numberEditor_;
    HFONT numberFont_ = nullptr;
    uint32_t editingNumber_ = 0;
    std::string previousNumberText_;
    std::optional<tools3000::capture::MarkupEngine::MarkupSnapshot> numberBefore_;
    bool closingNumber_ = false;
    bool numberComposing_ = false;
    bool numberOverflow_ = false;
    bool sizingNumber_ = false;
    int numberRequestedHeight_ = 0;
    json settings_, sessionSettings_, last_ = nullptr;
    bool configurationBusy_ = false;
    mutable std::mutex jobsMutex_;
    std::condition_variable jobsChanged_;
    std::map<std::string, json> jobs_;
    std::deque<std::string> completed_;
    std::string active_, mode_, language_ = "zh-CN";
    cv::Mat lastImage_;
    struct BackgroundTask { std::shared_ptr<std::atomic<bool>> finished; std::jthread thread; };
    std::vector<BackgroundTask> tasks_;
    std::shared_ptr<std::atomic<bool>> operationCancelled_ = std::make_shared<std::atomic<bool>>(false);
    ULONGLONG activity_ = 0;
    std::atomic<ULONGLONG> uiAlive_{0};
    std::atomic<bool> stoppingServices_{false};
};
}
