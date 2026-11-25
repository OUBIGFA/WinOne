#pragma once
#include "win32_support.h"
#include <nlohmann/json.hpp>
#include <opencv2/core.hpp>
#include <atomic>

namespace winone::capture {
using json = nlohmann::json;

// The capture engine never receives provider credentials. It uses the same
// authenticated Host pipe, image mapping, and job contracts as the old adapter.
class HostBridge {
public:
    HostBridge(std::wstring pipe, DWORD parent) : pipe_(std::move(pipe)), parent_(parent) {}
    json call(std::string_view method, const json& parameters = json::object(), HANDLE stop = nullptr,
              DWORD timeout = 5000, std::string requestId = {}) const;
    std::string recognize(const cv::Mat& image, const std::atomic<bool>* cancelled = nullptr) const;
    std::string translate(std::string_view text, std::string_view target, const std::atomic<bool>* cancelled = nullptr) const;
    std::string target(std::string_view text) const;
    std::string openResult(const cv::Mat& image, bool translate, const std::atomic<bool>* cancelled = nullptr) const;
    json saveToolbar(const json& tools) const;
    void prepareOCR() const;
private:
    json startImage(const cv::Mat& image, std::string_view method, json parameters,
                    std::string requestId = {}, DWORD timeout = 5000) const;
    std::string finishAI(const json& started, const std::atomic<bool>* cancelled) const;
    std::wstring pipe_;
    DWORD parent_;
};
}
