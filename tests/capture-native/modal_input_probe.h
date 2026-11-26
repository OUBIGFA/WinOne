#pragma once
#include <windows.h>
#include <string>

namespace winone::probe {
// A second enabled window on the capture UI thread exposes input starvation
// that posted-message probes cannot detect. Only the private probe builds it.
inline HWND modalInputWindow = nullptr;
inline HWND openModalInput() {
    modalInputWindow = CreateWindowExW(WS_EX_TOPMOST, L"EDIT", L"", WS_POPUP | WS_VISIBLE | WS_BORDER,
        16, 16, 200, 40, nullptr, nullptr, GetModuleHandleW(nullptr), nullptr);
    return modalInputWindow;
}
inline std::wstring closeModalInput() {
    wchar_t text[128]{};
    GetWindowTextW(modalInputWindow, text, 128);
    DestroyWindow(modalInputWindow); modalInputWindow = nullptr;
    return text;
}
}
