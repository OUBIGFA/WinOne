// Adapted from Win32++ 10.2, include/wxx_richedit.h (David Nash).
// Copyright (c) 2005-2025 David Nash. MIT; see ../licenses/win32xx-MIT.txt.
// This subset retains the native RichEdit message wrappers and DLL lifetime.
// CWnd/CString are replaced with HWND/std::wstring; no app/message loop is added.
#pragma once
#include <windows.h>
#include <richedit.h>
#include <cassert>
#include <stdexcept>
#include <string>

namespace winone::community {
class RichEdit {
public:
    RichEdit() {
        module_ = LoadLibraryExW(L"Msftedit.dll", nullptr, LOAD_LIBRARY_SEARCH_SYSTEM32);
        if (!module_) throw std::runtime_error("Windows RichEdit is unavailable");
    }
    ~RichEdit() {
        // As in CRichEdit, destroy the control before releasing its DLL.
        if (IsWindow()) DestroyWindow(window_);
        FreeLibrary(module_);
    }
    RichEdit(const RichEdit&) = delete;
    RichEdit& operator=(const RichEdit&) = delete;
    HWND Create(HWND parent, int id) {
        // Configure font/background/position before the first visible paint.
        window_ = CreateWindowExW(0, L"RICHEDIT50W", L"", WS_CHILD | WS_TABSTOP |
            ES_MULTILINE | ES_AUTOVSCROLL | ES_WANTRETURN | ES_NOHIDESEL,
            0, 0, 1, 1, parent, reinterpret_cast<HMENU>(static_cast<INT_PTR>(id)), GetModuleHandleW(nullptr), nullptr);
        if (!window_) throw std::runtime_error("Cannot create the number note editor");
        return window_;
    }
    void Detach() { window_ = nullptr; }
    long GetTextLengthEx(DWORD flags, UINT codePage) const {
        assert(IsWindow());
        GETTEXTLENGTHEX gtle = {};
        gtle.flags = flags;
        gtle.codepage = codePage;
        WPARAM wparam = reinterpret_cast<WPARAM>(&gtle);
        return static_cast<long>(SendMessage(EM_GETTEXTLENGTHEX, wparam, 0));
    }
    std::wstring GetTextRange(int first, int last) const {
        assert(IsWindow());
        CHARRANGE range = {};
        range.cpMin = first;
        range.cpMax = last;
        int lastChar = last == -1 ? GetTextLengthEx(GTL_NUMCHARS | GTL_PRECISE, 1200) : last;
        std::wstring rangeString(static_cast<size_t>(lastChar - first) + 1, L'\0');
        TEXTRANGEW tr = {};
        tr.chrg = range;
        tr.lpstrText = rangeString.data();
        LPARAM lparam = reinterpret_cast<LPARAM>(&tr);
        rangeString.resize(static_cast<size_t>(SendMessage(EM_GETTEXTRANGE, 0, lparam)));
        return rangeString;
    }
    void RequestResize() const {
        assert(IsWindow());
        SendMessage(EM_REQUESTRESIZE, 0, 0);
    }
    COLORREF SetBackgroundColor(BOOL isSysColor, COLORREF color) const {
        assert(IsWindow());
        WPARAM wparam = static_cast<WPARAM>(isSysColor);
        LPARAM lparam = static_cast<LPARAM>(color);
        return static_cast<COLORREF>(SendMessage(EM_SETBKGNDCOLOR, wparam, lparam));
    }
    BOOL SetDefaultCharFormat(CHARFORMAT2W format) const {
        assert(IsWindow());
        LPARAM lparam = reinterpret_cast<LPARAM>(&format);
        return static_cast<BOOL>(SendMessage(EM_SETCHARFORMAT, 0, lparam));
    }
    DWORD SetEventMask(DWORD mask) const {
        assert(IsWindow());
        LPARAM lparam = static_cast<LPARAM>(mask);
        return static_cast<DWORD>(SendMessage(EM_SETEVENTMASK, 0, lparam));
    }
    void SetRect(RECT rc) const {
        assert(IsWindow());
        LPARAM lparam = reinterpret_cast<LPARAM>(&rc);
        SendMessage(EM_SETRECT, 0, lparam);
    }
    void SetSel(long startChar, long endChar) const {
        assert(IsWindow());
        CHARRANGE cr = {};
        cr.cpMin = startChar;
        cr.cpMax = endChar;
        LPARAM lparam = reinterpret_cast<LPARAM>(&cr);
        SendMessage(EM_EXSETSEL, 0, lparam);
    }
    BOOL SetTextMode(UINT mode) const {
        assert(IsWindow());
        WPARAM wparam = static_cast<WPARAM>(mode);
        return static_cast<BOOL>(SendMessage(EM_SETTEXTMODE, wparam, 0));
    }
private:
    bool IsWindow() const { return ::IsWindow(window_) != FALSE; }
    LRESULT SendMessage(UINT message, WPARAM wparam, LPARAM lparam) const {
        return ::SendMessageW(window_, message, wparam, lparam);
    }
    HMODULE module_ = nullptr;
    HWND window_ = nullptr;
};
}
