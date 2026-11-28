#include "host_bridge.h"
#include <opencv2/imgcodecs.hpp>
#include <cstring>

namespace winone::capture {
namespace {
json responseResult(const json& response) {
    if (response.contains("error") && !response["error"].is_null())
        throw std::runtime_error(response["error"].value("message", "WinOne Host request failed"));
    return response.at("result");
}
void checkCancelled(const std::atomic<bool>* cancelled) {
    if (cancelled && cancelled->load()) throw std::runtime_error("Capture operation cancelled");
}
}

json HostBridge::call(std::string_view method, const json& parameters, HANDLE stop, DWORD timeout, std::string requestId) const {
    if (requestId.empty()) requestId = identity();
    const json request{{"version", 1}, {"id", requestId}, {"method", method}, {"params", parameters}};
    const auto response = json::parse(callHost(pipe_, parent_, request.dump(), stop, timeout));
    if (response.at("id") != requestId) throw std::runtime_error("WinOne Host response identity changed");
    return responseResult(response);
}

json HostBridge::startImage(const cv::Mat& image, std::string_view method, json parameters,
                           std::string requestId, DWORD timeout) const {
    std::vector<uchar> png;
    if (image.empty() || !cv::imencode(".png", image, png) || png.empty() || png.size() > 32 * 1024 * 1024)
        throw std::runtime_error("Capture image cannot be sent to WinOne");
    const auto name = L"Local\\WinOne.AI." + std::to_wstring(GetCurrentProcessId()) + L"." + wide(identity());
    Handle mapping(CreateFileMappingW(INVALID_HANDLE_VALUE, nullptr, PAGE_READWRITE, 0,
                                      static_cast<DWORD>(png.size()), name.c_str()));
    if (!mapping) throw systemError("Cannot create capture image mapping");
    void* view = MapViewOfFile(mapping.get(), FILE_MAP_WRITE, 0, 0, png.size());
    if (!view) throw systemError("Cannot write capture image mapping");
    std::memcpy(view, png.data(), png.size());
    UnmapViewOfFile(view);
    parameters["image"] = {{"mapping", utf8(name)}, {"bytes", png.size()}};
    // The Host copies the pixels before acknowledging. Keep the mapping alive
    // until that reply; its name alone is not an ownership transfer.
    return call(method, parameters, nullptr, timeout, std::move(requestId));
}

std::string HostBridge::finishAI(const json& started, const std::atomic<bool>* cancelled) const {
    const auto id = started.at("id").get<std::string>();
    auto release = [&] {
        try { call("ai.jobs.cancel", {{"id", id}}); }
        catch (const std::exception&) { OutputDebugStringW(L"WinOne capture: AI job release failed\n"); }
    };
    try {
        const auto deadline = GetTickCount64() + 60000;
        std::string output;
        for (;;) {
            checkCancelled(cancelled);
            if (GetTickCount64() >= deadline) throw std::runtime_error("Capture AI request timed out");
            const auto job = call("ai.jobs.get", {{"id", id}, {"waitMs", 500}, {"textOffset",output.size()}});
            const auto state = job.at("state").get<std::string>();
            output += job.value("text",std::string{});
            if(output.size()>64*1024*1024)throw std::runtime_error("OCR result is too large");
            if (state == "completed") { release(); return output; }
            if (state == "failed") throw std::runtime_error(job.value("error", "Capture AI request failed"));
            if (state == "cancelled") throw std::runtime_error("Capture AI request cancelled");
            if (state != "running") throw std::runtime_error("Invalid capture AI job state");
        }
    } catch (...) { release(); throw; }
}

std::string HostBridge::recognize(const cv::Mat& image, const std::atomic<bool>* cancelled, bool allowEmpty) const {
    checkCancelled(cancelled);
    return finishAI(startImage(image, "ai.recognize.start", {{"allowEmpty",allowEmpty}}), cancelled);
}

json HostBridge::recognizeLayout(const cv::Mat& image, const std::atomic<bool>* cancelled) const {
    checkCancelled(cancelled);
    return {{"layout", json::parse(finishAI(startImage(image, "ai.recognize.start", {{"layout", true}}), cancelled))}};
}

std::string HostBridge::translate(std::string_view text, std::string_view targetLanguage, const std::atomic<bool>* cancelled) const {
    checkCancelled(cancelled);
    return finishAI(call("ai.translate.start", {{"text", text}, {"target", targetLanguage}}), cancelled);
}

std::string HostBridge::target(std::string_view text) const {
    return call("ai.translate.target", {{"text", text}}).at("target").get<std::string>();
}

std::string HostBridge::openResult(const cv::Mat& image, bool translateImage, const std::atomic<bool>* cancelled) const {
    checkCancelled(cancelled);
    const auto id = identity();
    bool accepted = false;
    try {
        const auto result = startImage(image, "results.open", {{"id", id}, {"mode", translateImage ? "translate" : "ocr"}}, id, 12000);
        checkCancelled(cancelled);
        if (!result.at("opened").get<bool>()) throw std::runtime_error("Result window did not open");
        const auto window = reinterpret_cast<HWND>(std::stoull(result.at("window").get<std::string>()));
        DWORD process = 0;
        GetWindowThreadProcessId(window, &process);
        if (process != result.at("processId").get<DWORD>() || !SetForegroundWindow(window))
            throw std::runtime_error("Result window identity or activation failed");
        accepted = true;
    } catch (...) {
        if (!accepted) {
            try { call("results.cancel", {{"id", id}}); }
            catch (const std::exception&) { OutputDebugStringW(L"WinOne capture: late result cancellation failed\n"); }
        }
        throw;
    }
    return id;
}

json HostBridge::saveToolbar(const json& tools) const {
    auto settings = call("capture.settings.get");
    settings["toolbarTools"] = tools;
    const auto requestId = identity();
    auto result = call("capture.settings.save", settings, nullptr, 5000, requestId);
    if (result.contains("configurationJob")) {
        const auto jobId = result.at("configurationJob").get<std::string>();
        const auto deadline = GetTickCount64() + 30000;
        for (;;) {
            const auto job = call("configuration.jobs.get", {{"id", jobId}});
            if (job.at("id") != jobId) throw std::runtime_error("Configuration job identity changed");
            const auto state = job.at("state").get<std::string>();
            if (state == "completed" || state == "failed") {
                const auto& response = job.at("response");
                if (response.at("id") != requestId) throw std::runtime_error("Configuration response identity changed");
                result = responseResult(response);
                break;
            }
            if (GetTickCount64() >= deadline) throw std::runtime_error("Capture configuration save timed out");
            Sleep(25);
        }
    }
    return result;
}

std::string HostBridge::openTextResult(std::string_view text, const std::atomic<bool>* cancelled) const {
    checkCancelled(cancelled);
    const auto id = identity();
    try {
        const auto result = call("results.openText", {{"id",id},{"text",text}}, nullptr, 12000, id);
        checkCancelled(cancelled);
        if(!result.at("opened").get<bool>()) throw std::runtime_error("Result window did not open");
        const auto window=reinterpret_cast<HWND>(std::stoull(result.at("window").get<std::string>()));
        DWORD process=0; GetWindowThreadProcessId(window,&process);
        if(process!=result.at("processId").get<DWORD>() || !SetForegroundWindow(window)) throw std::runtime_error("Result window activation failed");
    } catch(...) { try { call("results.cancel",{{"id",id}}); } catch(...) {} throw; }
    return id;
}

void HostBridge::prepareOCR() const { call("ai.ocr.prepare"); }
}
