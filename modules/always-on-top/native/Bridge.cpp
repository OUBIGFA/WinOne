// The DLL owns a dedicated STA; callers never run PowerToys window code on Go threads.
#include "pch.h"
#include "AlwaysOnTop.h"
#include <chrono>
#include <condition_variable>
#include <deque>
#include <filesystem>
#include <future>
#include <mutex>
#include <sstream>
#include <thread>
#include <spdlog/sinks/base_sink.h>

void WinOneSetOwnershipKey(const wchar_t* key);

namespace
{
    constexpr UINT DispatchMessageId = WM_APP + 0x421;
    struct Work { std::function<HRESULT(AlwaysOnTop&)> action; std::promise<HRESULT> result; std::atomic<bool> cancelled{ false }; };
    std::mutex queueMutex;
    std::deque<std::shared_ptr<Work>> queue;
    std::thread uiThread;
    std::atomic<DWORD> threadId{ 0 };
    std::atomic<bool> started{ false };
    using LogCallback = uintptr_t (*)(uintptr_t, const char*, size_t);
    // PowerToys keeps its existing logging API, while this sink forwards each
    // event to WinOne's bounded, synchronized writer without a native log file.
    class WinOneLogSink final : public spdlog::sinks::base_sink<std::mutex>
    {
        LogCallback callback;
        uintptr_t context;
    public:
        WinOneLogSink(LogCallback callback, uintptr_t context) : callback(callback), context(context) {}
    protected:
        void sink_it_(const spdlog::details::log_msg& message) override {
            const auto text = fmt::format("[{}] {}", spdlog::level::to_string_view(message.level), message.payload);
            if (callback(context, text.data(), text.size()) != 0) throw spdlog::spdlog_ex("WinOne diagnostic write failed");
        }
        void flush_() override {}
    };

    HRESULT Invoke(std::function<HRESULT(AlwaysOnTop&)> action)
    {
        if (!threadId.load()) return HRESULT_FROM_WIN32(ERROR_NOT_READY);
        auto work = std::make_shared<Work>(); work->action = std::move(action);
        auto result = work->result.get_future();
        { std::lock_guard lock(queueMutex); queue.push_back(work); }
        if (!PostThreadMessageW(threadId.load(), DispatchMessageId, 0, 0)) { work->cancelled = true; return HRESULT_FROM_WIN32(GetLastError()); }
        if (result.wait_for(std::chrono::seconds(4)) != std::future_status::ready) { work->cancelled = true; return HRESULT_FROM_WIN32(ERROR_TIMEOUT); }
        return result.get();
    }

    bool Matches(HWND window, DWORD pid, unsigned long long created)
    {
        DWORD actual{}; GetWindowThreadProcessId(window, &actual);
        if (actual != pid || !IsWindow(window)) return false;
        if (GetAncestor(window, GA_ROOT) != window) return false;
        wchar_t className[256]{}; GetClassNameW(window, className, 256);
        for (const auto blocked : { L"Progman", L"WorkerW", L"Shell_TrayWnd", L"Shell_SecondaryTrayWnd" }) if (wcscmp(className, blocked) == 0) return false;
        wil::unique_handle process(OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid));
        if (!process) return false;
        FILETIME creation{}, exit{}, kernel{}, user{};
        if (!GetProcessTimes(process.get(), &creation, &exit, &kernel, &user)) return false;
        return ((static_cast<unsigned long long>(creation.dwHighDateTime) << 32) | creation.dwLowDateTime) == created;
    }
}

extern "C" __declspec(dllexport) HRESULT WinOneAotStartWithLogger(const wchar_t* directory, const wchar_t* ownershipKey, LogCallback logCallback, uintptr_t logContext)
{
    if (!directory || !ownershipKey || !logCallback || !std::filesystem::path(directory).is_absolute()) return E_INVALIDARG;
    if (started.exchange(true)) return HRESULT_FROM_WIN32(ERROR_ALREADY_EXISTS);
    std::promise<HRESULT> ready; auto result = ready.get_future();
    const std::wstring path(directory), key(ownershipKey);
    uiThread = std::thread([path, key, logCallback, logContext, ready = std::move(ready)]() mutable {
        bool signalled = false;
        try
        {
            winrt::init_apartment(winrt::apartment_type::single_threaded);
            if (!SetEnvironmentVariableW(L"WINONE_AOT_DATA", path.c_str())) throw winrt::hresult_error(HRESULT_FROM_WIN32(GetLastError()));
            std::filesystem::create_directories(path);
            Logger::init(std::vector<spdlog::sink_ptr>{std::make_shared<WinOneLogSink>(logCallback, logContext)});
            Logger::info("WinOne.AlwaysOnTop initialized");
            WinOneSetOwnershipKey(key.c_str());
            MSG message{}; PeekMessageW(&message, nullptr, WM_USER, WM_USER, PM_NOREMOVE);
            threadId = GetCurrentThreadId();
            {
                AlwaysOnTop core(true, threadId.load());
                if (!core.IsReadyForWinOne()) throw winrt::hresult_error(E_FAIL);
                ready.set_value(S_OK); signalled = true;
                while (GetMessageW(&message, nullptr, 0, 0) > 0)
                {
                    if (message.message == DispatchMessageId)
                    {
                        std::deque<std::shared_ptr<Work>> pending;
                        { std::lock_guard lock(queueMutex); pending.swap(queue); }
                        for (auto& work : pending)
                        {
                            HRESULT status = HRESULT_FROM_WIN32(ERROR_CANCELLED);
                            if (!work->cancelled) { try { status = work->action(core); } catch (...) { status = winrt::to_hresult(); } }
                            work->result.set_value(status);
                        }
                    }
                    else { TranslateMessage(&message); DispatchMessageW(&message); }
                }
            }
            threadId = 0;
            winrt::uninit_apartment();
        }
        catch (...)
        {
            const auto error = winrt::to_hresult();
            threadId = 0;
            if (!signalled) ready.set_value(error);
        }
    });
    // Startup is bounded by the worker process lifetime; no window action is issued before this returns.
    return result.get();
}

extern "C" __declspec(dllexport) HRESULT WinOneAotPin(HWND window, DWORD pid, unsigned long long created, BOOL enabled)
{
    return Invoke([=](AlwaysOnTop& core) { if (!Matches(window, pid, created)) return HRESULT_FROM_WIN32(ERROR_INVALID_WINDOW_HANDLE); return core.SetOwnedPin(window, enabled != FALSE); });
}

extern "C" __declspec(dllexport) HRESULT WinOneAotConfigure(DWORD flags, DWORD thickness, DWORD color, DWORD opacity, const wchar_t* excluded)
{
    if (thickness < 1 || thickness > 15 || opacity < 10 || opacity > 100 || !excluded) return E_INVALIDARG;
    const std::wstring exclusions(excluded);
    return Invoke([=](AlwaysOnTop&) {
        Settings options;
        options.enableFrame = (flags & 1) != 0; options.frameAccentColor = (flags & 2) != 0;
        options.roundCornersEnabled = (flags & 4) != 0; options.blockInGameMode = (flags & 8) != 0;
        options.enableSound = (flags & 16) != 0; options.showInSystemMenu = false;
        options.frameThickness = thickness; options.frameColor = color; options.frameOpacity = opacity;
        std::wistringstream lines(exclusions); std::wstring line;
        while (std::getline(lines, line)) { if (!line.empty()) { CharUpperBuffW(line.data(), static_cast<DWORD>(line.size())); options.excludedApps.push_back(line); } }
        AlwaysOnTopSettings::instance().ApplyWinOneSettings(options);
        return S_OK;
    });
}

extern "C" __declspec(dllexport) HRESULT WinOneAotClear() { return Invoke([](AlwaysOnTop& core) { return core.ClearOwnedPins(); }); }

extern "C" __declspec(dllexport) HRESULT WinOneAotStop()
{
    if (!uiThread.joinable()) return S_OK;
    if (threadId.load())
    {
        const auto status = WinOneAotClear(); if (FAILED(status)) return status;
        PostThreadMessageW(threadId.load(), WM_QUIT, 0, 0);
    }
    uiThread.join(); return S_OK;
}
