#pragma once
#include "host_bridge.h"
#include "ocr/OcrEngine.h"
#include <filesystem>

namespace winone::capture {
class OcrClient {
public:
    explicit OcrClient(std::filesystem::path captureDirectory) : directory_(std::move(captureDirectory)) {}
    void setHost(const HostBridge* host) { host_ = host; }
    json recognize(const cv::Mat& image, bool layout, const std::atomic<bool>* cancelled = nullptr) const;
    std::vector<tools3000::ocr::OcrResult> layout(const cv::Mat& image, const std::atomic<bool>* cancelled = nullptr) const;
private:
    std::filesystem::path directory_;
    const HostBridge* host_ = nullptr;
};
}
