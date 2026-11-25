// The native worker must retain the existing Host wire format, including
// fragmented messages and rejection before allocating an oversized payload.
#include "pipe_frames.h"
#include <iostream>
#include <stdexcept>
#include <string>
#include <vector>

using winone::capture::FrameDecoder;
using winone::capture::encodeFrame;

static void require(bool condition, const char* message) {
    if (!condition) throw std::runtime_error(message);
}

template<class F> static void rejects(F&& action, const char* message) {
    try { action(); } catch (const std::invalid_argument&) { return; }
    throw std::runtime_error(message);
}

int main() {
    try {
        const std::string first = R"({"id":"one","method":"snapshot","version":1})";
        const std::string second = R"({"id":"two","result":{"text":"中文"}})";
        auto one = encodeFrame(first);
        require(one.size() == first.size() + 4, "encoded length differs from Host framing");
        require(one[0] == first.size() && one[1] == 0 && one[2] == 0 && one[3] == 0,
                "length prefix must be little endian");
        FrameDecoder decoder;
        for (size_t index = 0; index < one.size(); ++index) {
            auto frames = decoder.push({one.data() + index, 1});
            require(frames.size() == (index + 1 == one.size() ? 1 : 0), "partial frame escaped decoder");
            if (!frames.empty()) require(frames.front() == first, "fragmented UTF-8 payload changed");
        }
        decoder.finish();
        auto two = encodeFrame(second);
        auto joined = one;
        joined.insert(joined.end(), two.begin(), two.end());
        const auto frames = decoder.push(joined);
        require(frames == std::vector<std::string>{first, second}, "adjacent messages were merged or lost");
        decoder.finish();
        const std::string maximum(1024 * 1024, 'a');
        require(decoder.push(encodeFrame(maximum)) == std::vector<std::string>{maximum}, "valid maximum rejected");
        rejects([] { encodeFrame(""); }, "empty frame accepted");
        rejects([] { encodeFrame(std::string(1024 * 1024 + 1, 'x')); }, "oversized outgoing frame accepted");
        rejects([] { FrameDecoder d; d.push(std::vector<uint8_t>{0,0,0,0}); }, "zero incoming frame accepted");
        rejects([] { FrameDecoder d; d.push(std::vector<uint8_t>{1,0,16,0}); }, "oversized incoming frame accepted");
        rejects([] { FrameDecoder d; d.push(std::vector<uint8_t>{255,255,255,255}); }, "overflow frame accepted");
        rejects([] { FrameDecoder d; d.push(std::vector<uint8_t>{3,0}); d.finish(); }, "truncated prefix accepted");
        rejects([] { FrameDecoder d; d.push(std::vector<uint8_t>{3,0,0,0,'x'}); d.finish(); }, "truncated payload accepted");
        std::cout << "Native capture Host framing: 11 boundary scenarios passed\n";
        return 0;
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n';
        return 1;
    }
}
