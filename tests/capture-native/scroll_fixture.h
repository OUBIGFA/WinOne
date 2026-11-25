#pragma once
#include "capture/CaptureBackend.h"
#include <opencv2/core.hpp>
#include <algorithm>

// Only the private probe links this generated scrolling page. Capture, overlap
// analysis, stitching, progress and editor delivery remain the community code.
class ScrollPageFixture final : public tools3000::capture::ICaptureBackend {
    tools3000::capture::CaptureBackendInfo info_{"memory", "Private scrolling page", true, false, {}};
    cv::Mat document_, frame_;
    int height_ = 0, offset_ = 0;
public:
    const tools3000::capture::CaptureBackendInfo& info() const noexcept override { return info_; }
    bool initialize(const tools3000::capture::CaptureRegion& region, std::string&) override {
        height_ = region.height; offset_ = 0;
        document_.create(region.height * 4, region.width, CV_8UC3);
        cv::RNG random(0x57494e4f);
        random.fill(document_, cv::RNG::UNIFORM, 20, 235);
        return true;
    }
    bool capture(tools3000::capture::CaptureFrameView& frame, std::string&) override {
        frame_ = document_(cv::Rect(0, offset_, document_.cols, height_));
        frame = {frame_.data, static_cast<int>(frame_.step), frame_.cols, frame_.rows, tools3000::capture::CapturePixelFormat::Bgr24};
        offset_ = std::min(document_.rows - height_, offset_ + height_ / 3);
        return true;
    }
    void releaseFrame() noexcept override {}
    void shutdown() noexcept override { frame_.release(); document_.release(); }
};
