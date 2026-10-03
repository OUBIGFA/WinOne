#pragma once
#include <windows.h>

// A provider may hang inside COM even after its client disappears. This guard
// terminates only this short-lived query process; it never touches the target app.
class CaptureQueryLifetime final {
    HANDLE stop_ = nullptr;
    HANDLE parent_ = nullptr;
    HANDLE watcher_ = nullptr;
    DWORD deadline_ = 5000;
    static DWORD WINAPI Watch(void* context) {
        auto* self = static_cast<CaptureQueryLifetime*>(context);
        HANDLE handles[] = {self->stop_, self->parent_};
        const auto result = WaitForMultipleObjects(self->parent_ ? 2 : 1, handles, FALSE, self->deadline_);
        if (result != WAIT_OBJECT_0) TerminateProcess(GetCurrentProcess(), 8);
        return 0;
    }
public:
    CaptureQueryLifetime() = default;
    CaptureQueryLifetime(const CaptureQueryLifetime&) = delete;
    CaptureQueryLifetime& operator=(const CaptureQueryLifetime&) = delete;
    bool Start(DWORD parent, DWORD deadline = 5000) {
        if (stop_ || deadline == 0) return false;
        deadline_ = deadline;
        if (parent) {
            parent_ = OpenProcess(SYNCHRONIZE, FALSE, parent);
            if (!parent_) return false;
        }
        stop_ = CreateEventW(nullptr, TRUE, FALSE, nullptr);
        if (!stop_) return false;
        watcher_ = CreateThread(nullptr, 0, Watch, this, 0, nullptr);
        return watcher_ != nullptr;
    }
    ~CaptureQueryLifetime() {
        if (stop_) SetEvent(stop_);
        if (watcher_) { WaitForSingleObject(watcher_, INFINITE); CloseHandle(watcher_); }
        if (stop_) CloseHandle(stop_);
        if (parent_) CloseHandle(parent_);
    }
};
