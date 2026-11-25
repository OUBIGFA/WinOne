#pragma once
#include <windows.h>
#include <objbase.h>
#include <algorithm>
#include <chrono>
#include <cwctype>
#include <filesystem>
#include <functional>
#include <memory>
#include <stdexcept>
#include <string>
#include <string_view>
#include <thread>
#include <vector>
#include "pipe_frames.h"

namespace winone::capture {
class Handle {
public:
    explicit Handle(HANDLE value = nullptr) noexcept : value_(value) {}
    ~Handle() { reset(); }
    Handle(const Handle&) = delete;
    Handle& operator=(const Handle&) = delete;
    Handle(Handle&& other) noexcept : value_(other.release()) {}
    Handle& operator=(Handle&& other) noexcept { if (this != &other) reset(other.release()); return *this; }
    HANDLE get() const noexcept { return value_; }
    explicit operator bool() const noexcept { return value_ && value_ != INVALID_HANDLE_VALUE; }
    HANDLE release() noexcept { auto value = value_; value_ = nullptr; return value; }
    void reset(HANDLE value = nullptr) noexcept { if (*this) CloseHandle(value_); value_ = value; }
private:
    HANDLE value_;
};

inline std::runtime_error systemError(std::string_view action, DWORD error = GetLastError()) {
    return std::runtime_error(std::string(action) + " (Windows " + std::to_string(error) + ")");
}

inline std::wstring wide(std::string_view value) {
    if (value.empty()) return {};
    const int count = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, value.data(), static_cast<int>(value.size()), nullptr, 0);
    if (count <= 0) throw systemError("Invalid UTF-8");
    std::wstring result(count, L'\0');
    if (MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, value.data(), static_cast<int>(value.size()), result.data(), count) != count)
        throw systemError("UTF-8 conversion failed");
    return result;
}

inline std::string utf8(std::wstring_view value) {
    if (value.empty()) return {};
    const int count = WideCharToMultiByte(CP_UTF8, WC_ERR_INVALID_CHARS, value.data(), static_cast<int>(value.size()), nullptr, 0, nullptr, nullptr);
    if (count <= 0) throw systemError("Invalid Unicode");
    std::string result(count, '\0');
    if (WideCharToMultiByte(CP_UTF8, WC_ERR_INVALID_CHARS, value.data(), static_cast<int>(value.size()), result.data(), count, nullptr, nullptr) != count)
        throw systemError("Unicode conversion failed");
    return result;
}

inline std::wstring pipePath(std::wstring name) {
    if (!name.starts_with(L"\\\\.\\pipe\\")) name = L"\\\\.\\pipe\\" + name;
    return name;
}

inline std::string identity() {
    GUID id{};
    if (FAILED(CoCreateGuid(&id))) throw std::runtime_error("Cannot create capture identity");
    wchar_t text[40]{};
    if (!StringFromGUID2(id, text, 40)) throw std::runtime_error("Cannot format capture identity");
    std::string result;
    for (const auto character : std::wstring_view(text))
        if (character != L'{' && character != L'}' && character != L'-')
            result.push_back(static_cast<char>(towlower(character)));
    return result;
}

inline DWORD finishIO(HANDLE file, OVERLAPPED& pending, HANDLE stop, DWORD timeout) {
    HANDLE waits[]{pending.hEvent, stop};
    const auto state = WaitForMultipleObjects(stop ? 2 : 1, waits, FALSE, timeout);
    if (state != WAIT_OBJECT_0) {
        CancelIoEx(file, &pending);
        DWORD ignored = 0;
        GetOverlappedResult(file, &pending, &ignored, TRUE);
        throw systemError(state == WAIT_TIMEOUT ? "Capture communication timed out" : "Capture communication cancelled",
                          state == WAIT_TIMEOUT ? ERROR_TIMEOUT : ERROR_OPERATION_ABORTED);
    }
    DWORD transferred = 0;
    if (!GetOverlappedResult(file, &pending, &transferred, FALSE)) throw systemError("Capture communication failed");
    return transferred;
}

inline void pipeTransfer(HANDLE pipe, std::span<uint8_t> bytes, bool write, HANDLE stop, DWORD timeout = 5000) {
    const auto deadline = GetTickCount64() + timeout;
    Handle event(CreateEventW(nullptr, TRUE, FALSE, nullptr));
    if (!event) throw systemError("Cannot create pipe event");
    while (!bytes.empty()) {
        OVERLAPPED pending{};
        pending.hEvent = event.get();
        ResetEvent(event.get());
        DWORD transferred = 0;
        const auto length = static_cast<DWORD>(bytes.size());
        const BOOL finished = write ? WriteFile(pipe, bytes.data(), length, &transferred, &pending)
                                    : ReadFile(pipe, bytes.data(), length, &transferred, &pending);
        if (!finished) {
            if (GetLastError() != ERROR_IO_PENDING) throw systemError("Capture pipe failed");
            const auto now = GetTickCount64();
            transferred = finishIO(pipe, pending, stop, now >= deadline ? 0 : static_cast<DWORD>(deadline - now));
        }
        if (transferred == 0) throw std::runtime_error("Capture pipe closed before the complete message");
        bytes = bytes.subspan(transferred);
    }
}

inline std::string readPipeMessage(HANDLE pipe, HANDLE stop, DWORD timeout = 5000) {
    uint8_t prefix[4]{};
    pipeTransfer(pipe, prefix, false, stop, timeout);
    const uint32_t length = static_cast<uint32_t>(prefix[0]) | static_cast<uint32_t>(prefix[1]) << 8 |
                            static_cast<uint32_t>(prefix[2]) << 16 | static_cast<uint32_t>(prefix[3]) << 24;
    if (length == 0 || length > maximumFrameBytes) throw std::invalid_argument("Invalid Host message length");
    std::vector<uint8_t> bytes(length);
    pipeTransfer(pipe, bytes, false, stop, timeout);
    return {reinterpret_cast<const char*>(bytes.data()), bytes.size()};
}

inline void writePipeMessage(HANDLE pipe, std::string_view message, HANDLE stop) {
    auto bytes = encodeFrame(message);
    pipeTransfer(pipe, bytes, true, stop);
}

inline std::string callHost(std::wstring_view name, DWORD parent, std::string_view request, HANDLE stop, DWORD timeout = 5000) {
    const auto path = pipePath(std::wstring(name));
    Handle pipe;
    const auto deadline = GetTickCount64() + 5000;
    for (;;) {
        pipe.reset(CreateFileW(path.c_str(), GENERIC_READ | GENERIC_WRITE, 0, nullptr, OPEN_EXISTING, FILE_FLAG_OVERLAPPED, nullptr));
        if (pipe) break;
        const auto error = GetLastError();
        if ((error != ERROR_PIPE_BUSY && error != ERROR_FILE_NOT_FOUND) || GetTickCount64() >= deadline)
            throw systemError("Cannot connect to WinOne Host", error);
        // A server may recreate its single-use pipe after the previous reply.
        // WaitNamedPipe does not wait while no instance exists, so cover that
        // documented gap without retrying an already submitted request.
        if (stop && WaitForSingleObject(stop, 5) == WAIT_OBJECT_0) throw systemError("Host connection cancelled", ERROR_OPERATION_ABORTED);
        if (!stop) Sleep(5);
        WaitNamedPipeW(path.c_str(), 25);
    }
    ULONG server = 0;
    if (!GetNamedPipeServerProcessId(pipe.get(), &server)) throw systemError("Cannot verify WinOne Host");
    if (parent == 0 || server != parent) throw std::runtime_error("WinOne Host identity changed");
    writePipeMessage(pipe.get(), request, stop);
    return readPipeMessage(pipe.get(), stop, timeout);
}

// Each waiting instance blocks in the kernel. Closing the owning stop event
// cancels both connection and request I/O before the threads are joined.
class PipeServer {
public:
    using Handler = std::function<std::string(std::string_view)>;
    using Delivered = std::function<void(std::string_view, std::string_view)>;
    PipeServer(std::wstring name, Handler handler, Delivered delivered = {}) : name_(pipePath(std::move(name))), handler_(std::move(handler)), delivered_(std::move(delivered)),
        stop_(CreateEventW(nullptr, TRUE, FALSE, nullptr)) {
        if (!stop_) throw systemError("Cannot create capture stop event");
        // Creation is synchronous so a failed/occupied endpoint never reports
        // startup success, and the Host can connect as soon as we return.
        Handle first(createPipe(true));
        if (!first) throw systemError("Cannot own capture pipe");
        workers_.emplace_back([this, pipe = std::move(first)]() mutable { listen(std::move(pipe)); });
        for (unsigned index = 1; index < 16; ++index) workers_.emplace_back([this] { listen(Handle{}); });
    }
    ~PipeServer() { stop(); }
    void stop() {
        SetEvent(stop_.get());
        workers_.clear();
    }
private:
    HANDLE createPipe(bool first = false) const {
        return CreateNamedPipeW(name_.c_str(), PIPE_ACCESS_DUPLEX | FILE_FLAG_OVERLAPPED |
            (first ? FILE_FLAG_FIRST_PIPE_INSTANCE : 0),
            PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT | PIPE_REJECT_REMOTE_CLIENTS,
            16, 65536, 65536, 0, nullptr);
    }
    void listen(Handle pipe) {
        while (WaitForSingleObject(stop_.get(), 0) == WAIT_TIMEOUT) {
            if (!pipe) pipe.reset(createPipe());
            if (!pipe) {
                OutputDebugStringW(L"WinOne native capture: named-pipe instance could not be created\n");
                SetEvent(stop_.get());
                return;
            }
            Handle event(CreateEventW(nullptr, TRUE, FALSE, nullptr));
            if (!event) { SetEvent(stop_.get()); return; }
            OVERLAPPED pending{};
            pending.hEvent = event.get();
            try {
                if (!ConnectNamedPipe(pipe.get(), &pending)) {
                    const auto error = GetLastError();
                    if (error == ERROR_IO_PENDING) finishIO(pipe.get(), pending, stop_.get(), INFINITE);
                    else if (error != ERROR_PIPE_CONNECTED) throw systemError("Capture pipe connection failed", error);
                }
                const auto request = readPipeMessage(pipe.get(), stop_.get());
                const auto response = handler_(request);
                writePipeMessage(pipe.get(), response, stop_.get());
                // DisconnectNamedPipe discards unread reply bytes. The Host
                // closes its one-request connection after reading the response;
                // observe that close with cancellable, bounded I/O before reuse.
                uint8_t end = 0;
                try { pipeTransfer(pipe.get(), {&end, 1}, false, stop_.get()); }
                catch (const std::exception&) { /* Peer EOF is the expected acknowledgment. */ }
                if (delivered_) delivered_(request, response);
            } catch (const std::exception&) {
                // The request handler serializes business errors. A disconnected,
                // cancelled or malformed transport has no reply channel left.
            }
            DisconnectNamedPipe(pipe.get());
            pipe.reset();
        }
    }
    std::wstring name_;
    Handler handler_;
    Delivered delivered_;
    Handle stop_;
    std::vector<std::jthread> workers_;
};
} // namespace winone::capture
