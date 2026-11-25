#include "win32_support.h"
#include <atomic>
#include <future>
#include <iostream>

using namespace winone::capture;
int main() {
    try {
        const auto name = L"WinOne-NativeCapture-Test-" + wide(identity());
        std::atomic<unsigned> calls = 0;
        const auto started = GetTickCount64();
        {
            PipeServer server(name, [&](std::string_view request) { ++calls; return std::string(request); });
            const std::string text = "{\"text\":\"中文😀\"}";
            if (callHost(name, GetCurrentProcessId(), text, nullptr) != text)
                throw std::runtime_error("Actual named-pipe UTF-8 response differs");
            const std::string large(maximumFrameBytes, 'x');
            if (callHost(name, GetCurrentProcessId(), large, nullptr) != large)
                throw std::runtime_error("Maximum named-pipe message was truncated");
            std::vector<std::future<std::string>> pending;
            for (unsigned index = 0; index < 8; ++index)
                pending.push_back(std::async(std::launch::async, [&] { return callHost(name, GetCurrentProcessId(), text, nullptr); }));
            for (auto& result : pending)
                if (result.get() != text) throw std::runtime_error("Concurrent pipe request was lost");
            bool rejected = false;
            try { callHost(name, GetCurrentProcessId() + 1, text, nullptr); }
            catch (const std::runtime_error&) { rejected = true; }
            if (!rejected || calls != 10) throw std::runtime_error("Host identity check did not reject before dispatch");
            const auto stopStarted = GetTickCount64();
            server.stop();
            if (GetTickCount64() - stopStarted > 2000) throw std::runtime_error("Idle native pipe threads did not stop promptly");
        }
        std::cout << "PASS: actual named-pipe round trips, concurrent requests, Host identity and idle shutdown in "
                  << GetTickCount64() - started << " ms\n";
        return 0;
    } catch (const std::exception& error) { std::cerr << error.what() << '\n'; return 1; }
}
