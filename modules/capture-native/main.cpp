#include "capture_app.h"
#include "core/utils/OutOfProcessFileDialog.h"
#include <shellapi.h>
#include <iostream>
#include <fstream>
#include <opencv2/imgcodecs.hpp>
#ifdef WINONE_CAPTURE_PRIVATE_PROBE
#include "../../tests/capture-native/scroll_fixture.h"
#include "../../tests/capture-native/render_readback.h"
#endif

namespace { winone::capture::CaptureApp* current = nullptr; }
#ifdef WINONE_CAPTURE_PRIVATE_PROBE
extern "C" __declspec(dllexport) void WinOneNativeRenderedFrame(void* target) noexcept {
    try { winone::probe::readFrame(static_cast<ID2D1RenderTarget*>(target)); }
    catch (const std::exception& error) { std::cerr << error.what() << '\n'; }
}
extern "C" __declspec(dllexport) bool WinOneNativeCaptureFixture(void* output) noexcept {
    try {
        if (!output) return false;
        // Generated pixels are available only in the separately named test
        // executable. The production worker fails when OS capture fails.
        auto& image = *static_cast<cv::Mat*>(output);
        wchar_t fixture[32768]{};
        if (GetEnvironmentVariableW(L"WINONE_CAPTURE_FIXTURE", fixture, 32768)) {
            std::ifstream file(std::filesystem::path(fixture), std::ios::binary);
            std::vector<unsigned char> bytes((std::istreambuf_iterator<char>(file)), std::istreambuf_iterator<char>());
            image = cv::imdecode(bytes, cv::IMREAD_UNCHANGED);
            return !image.empty();
        }
        image = cv::Mat(GetSystemMetrics(SM_CYVIRTUALSCREEN), GetSystemMetrics(SM_CXVIRTUALSCREEN), CV_8UC4);
        for (int y = 0; y < image.rows; ++y) for (int x = 0; x < image.cols; ++x)
            image.at<cv::Vec4b>(y, x) = {static_cast<uchar>(x % 255), static_cast<uchar>(y % 255), 180, 255};
        return true;
    } catch (...) { return false; }
}
#endif
extern "C" __declspec(dllexport) bool WinOneNativeOcrAvailable() { return current && current->ocrAvailable(); }
extern "C" __declspec(dllexport) bool WinOneNativeSavePin(HWND owner, const void* image) noexcept {
    try { return current && image && current->savePin(owner,*static_cast<const cv::Mat*>(image)); }
    catch(const std::exception& error) { OutputDebugStringA(error.what()); return false; }
}
extern "C" __declspec(dllexport) bool WinOneNativeRecordingStatus(void* stats, int* state) noexcept {
    try { return current && current->recordingStatus(stats, state); } catch (...) { return false; }
}
extern "C" __declspec(dllexport) void WinOneNativeRecordingAction(int action) noexcept {
    try { if (current) current->recordingAction(action); }
    catch (const std::exception& error) { OutputDebugStringA(error.what()); }
}
// Both sides use the same pinned MSVC/OpenCV libraries. Exceptions never cross
// these loader-resolved callback boundaries or escape an extern-C function.
extern "C" __declspec(dllexport) bool WinOneNativeOcr(void* output, const void* input) noexcept {
    try {
        if (!current || !output || !input) return false;
        *static_cast<std::vector<tools3000::ocr::OcrResult>*>(output) = current->recognizeLayout(*static_cast<const cv::Mat*>(input));
        return true;
    } catch (...) { return false; }
}
extern "C" __declspec(dllexport) bool WinOneNativeTranslate(void* output, const void* input, const void* cancelled) noexcept {
    try {
        if (!current || !output || !input) return false;
        *static_cast<tools3000::core::translate::TranslationBatchResult*>(output) = current->translateBatch(
            *static_cast<const tools3000::core::translate::TranslationBatchRequest*>(input), static_cast<const std::atomic<bool>*>(cancelled));
        return true;
    } catch (...) { return false; }
}

int WINAPI wWinMain(HINSTANCE instance, HINSTANCE, PWSTR, int) {
    SetErrorMode(SEM_FAILCRITICALERRORS | SEM_NOGPFAULTERRORBOX);
    SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
    const auto com = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED | COINIT_DISABLE_OLE1DDE);
    if (FAILED(com)) return 1;
    int exitCode = 1;
    try {
        int count = 0;
        auto raw = CommandLineToArgvW(GetCommandLineW(), &count);
        if (!raw) throw winone::capture::systemError("Cannot read capture arguments");
        std::vector<std::wstring> arguments(raw, raw + count); LocalFree(raw);
        std::wstring pipe, hostPipe, directory, worker, job;
        DWORD parent = 0; bool standby = false;
        for (size_t index = 1; index < arguments.size(); ++index) {
            const auto& argument = arguments[index];
            auto next = [&]() -> std::wstring { if (++index == arguments.size()) throw std::invalid_argument("Missing capture argument"); return arguments[index]; };
            if (argument == L"--pipe") pipe = next();
            else if (argument == L"--host-pipe") hostPipe = next();
            else if (argument == L"--data-dir") directory = next();
            else if (argument == L"--parent") parent = static_cast<DWORD>(std::stoul(next()));
            else if (argument == L"--stay-ready") standby = true;
            else if (argument.starts_with(L"--worker=")) worker = argument.substr(9);
            else if (argument.starts_with(L"--job=")) job = argument.substr(6);
            else throw std::invalid_argument("Unknown native capture argument");
        }
        if (worker == L"file-dialog" && !job.empty()) exitCode = tools3000::core::OutOfProcessFileDialog::runWorkerFromJobFile(job);
        else {
            if (pipe.empty() || directory.empty()) throw std::invalid_argument("Missing capture pipe or data directory");
#ifdef WINONE_CAPTURE_PRIVATE_PROBE
            tools3000::capture::setCaptureBackendFactoryForTesting([] { return std::make_unique<ScrollPageFixture>(); });
#endif
            winone::capture::CaptureApp app(pipe, directory, parent, hostPipe, standby);
            current = &app;
            exitCode = app.run(instance);
            current = nullptr;
        }
    } catch (const std::exception& error) {
        OutputDebugStringA(error.what());
        std::cerr << error.what() << '\n';
    }
    current = nullptr;
    CoUninitialize();
    return exitCode;
}
