// WinOne's existing Go Host protocol: a four-byte little-endian length followed
// by UTF-8 JSON. This transport layer deliberately does not interpret the JSON.
#pragma once
#include <algorithm>
#include <cstdint>
#include <span>
#include <stdexcept>
#include <string>
#include <string_view>
#include <vector>

namespace winone::capture {
inline constexpr uint32_t maximumFrameBytes = 1024 * 1024;

inline std::vector<uint8_t> encodeFrame(std::string_view message) {
    if (message.empty() || message.size() > maximumFrameBytes)
        throw std::invalid_argument("Invalid Host message length");
    const auto length = static_cast<uint32_t>(message.size());
    std::vector<uint8_t> bytes;
    bytes.reserve(message.size() + 4);
    for (unsigned shift = 0; shift < 32; shift += 8)
        bytes.push_back(static_cast<uint8_t>(length >> shift));
    bytes.insert(bytes.end(), message.begin(), message.end());
    return bytes;
}

class FrameDecoder {
public:
    std::vector<std::string> push(std::span<const uint8_t> bytes) {
        std::vector<std::string> frames;
        while (!bytes.empty()) {
            if (prefixBytes_ < 4) {
                length_ |= static_cast<uint32_t>(bytes.front()) << (8 * prefixBytes_++);
                bytes = bytes.subspan(1);
                if (prefixBytes_ != 4) continue;
                // Check the complete prefix before reserving payload storage.
                if (length_ == 0 || length_ > maximumFrameBytes)
                    throw std::invalid_argument("Invalid Host message length");
                payload_.reserve(length_);
            }
            const auto count = std::min(bytes.size(), static_cast<size_t>(length_) - payload_.size());
            if (count != 0) payload_.append(reinterpret_cast<const char*>(bytes.data()), count);
            bytes = bytes.subspan(count);
            if (payload_.size() == length_) {
                frames.push_back(std::move(payload_));
                payload_.clear();
                prefixBytes_ = 0;
                length_ = 0;
            }
        }
        return frames;
    }

    void finish() const {
        if (prefixBytes_ != 0) throw std::invalid_argument("Truncated Host message");
    }

private:
    uint32_t length_ = 0;
    unsigned prefixBytes_ = 0;
    std::string payload_;
};
} // namespace winone::capture
