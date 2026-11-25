#pragma once
#include "host_bridge.h"
#include "capture/MarkupEngine.h"
#include <optional>

namespace winone::capture {
struct Draft {
    std::string id;
    uint64_t revision = 0;
    std::string mode;
    json settings;
    cv::Mat original;
    cv::Rect selection;
    POINT origin{};
    std::vector<tools3000::capture::MarkupElement> elements;
};

class DraftStore {
public:
    explicit DraftStore(std::filesystem::path directory) : directory_(std::move(directory)) {}
    std::optional<Draft> read() const;
    void save(const Draft& draft);
    void retire(std::string_view id, uint64_t revision);
    void markRetired(std::string_view id, uint64_t revision) const;
    void finishRetirement(std::string_view id, uint64_t revision) const;
    void cleanupImage(std::string_view id) const;
    void recoverRetirements() const;
    static json encodeElements(const std::vector<tools3000::capture::MarkupElement>& elements);
    static std::vector<tools3000::capture::MarkupElement> decodeElements(const json& value);
private:
    std::filesystem::path directory_;
    std::string imageId_, imageDigest_;
};
}
