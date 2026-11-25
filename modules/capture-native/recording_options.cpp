#include "recording_options.h"
#include <commctrl.h>
#include <vector>
#include <string_view>
#include <stdexcept>

namespace winone::capture {
namespace {
constexpr int Fps = 101, Quality = 102, Seconds = 103, Cursor = 104, Codec = 105;
struct DialogState { RecordingOptions options; bool english; HFONT font = nullptr; };

HWND control(HWND dialog, DWORD style, const wchar_t* type, const std::wstring& text,
             int x, int y, int width, int height, int id, HFONT font) {
    RECT bounds{x, y, x + width, y + height}; MapDialogRect(dialog, &bounds);
    const auto window = CreateWindowExW(type == std::wstring_view(L"EDIT") ? WS_EX_CLIENTEDGE : 0,
        type, text.c_str(), WS_CHILD | WS_VISIBLE | style, bounds.left, bounds.top,
        bounds.right - bounds.left, bounds.bottom - bounds.top, dialog,
        reinterpret_cast<HMENU>(static_cast<INT_PTR>(id)), GetModuleHandleW(nullptr), nullptr);
    if (!window) throw std::runtime_error("Cannot create recording option control");
    SendMessageW(window, WM_SETFONT, reinterpret_cast<WPARAM>(font), TRUE);
    return window;
}
void number(HWND dialog, int id, int value, int maximum, int y, HFONT font) {
    auto edit = control(dialog, WS_TABSTOP | ES_NUMBER | ES_AUTOHSCROLL, L"EDIT", std::to_wstring(value), 137, y, 95, 15, id, font);
    auto spin = control(dialog, UDS_ALIGNRIGHT | UDS_SETBUDDYINT | UDS_ARROWKEYS | UDS_NOTHOUSANDS,
        UPDOWN_CLASSW, L"", 0, 0, 0, 0, id + 100, font);
    SendMessageW(spin, UDM_SETBUDDY, reinterpret_cast<WPARAM>(edit), 0);
    SendMessageW(spin, UDM_SETRANGE32, 1, maximum);
    SendMessageW(spin, UDM_SETPOS32, 0, value);
}
INT_PTR CALLBACK procedure(HWND dialog, UINT message, WPARAM wp, LPARAM lp) {
    auto* state = reinterpret_cast<DialogState*>(GetWindowLongPtrW(dialog, DWLP_USER));
    if (message == WM_INITDIALOG) {
        state = reinterpret_cast<DialogState*>(lp); SetWindowLongPtrW(dialog, DWLP_USER, lp);
        state->font = reinterpret_cast<HFONT>(SendMessageW(dialog, WM_GETFONT, 0, 0));
        try {
            SetWindowTextW(dialog, state->english ? L"Recording options" : L"录制设置");
            control(dialog, 0, L"STATIC", state->english ? L"Frames per second" : L"每秒帧数", 14, 18, 115, 12, -1, state->font);
            number(dialog, Fps, state->options.fps, state->options.gif ? 30 : 60, 15, state->font);
            if (state->options.gif) {
                control(dialog, 0, L"STATIC", state->english ? L"Quality (1–100)" : L"画质（1–100）", 14, 44, 115, 12, -1, state->font);
                number(dialog, Quality, state->options.quality, 100, 41, state->font);
                control(dialog, 0, L"STATIC", state->english ? L"Stop after (seconds)" : L"自动停止（秒）", 14, 70, 115, 12, -1, state->font);
                number(dialog, Seconds, state->options.seconds, 600, 67, state->font);
            } else {
                control(dialog, 0, L"STATIC", state->english ? L"Video codec" : L"视频编码", 14, 44, 115, 12, -1, state->font);
                auto codec = control(dialog, WS_TABSTOP | CBS_DROPDOWNLIST | WS_VSCROLL, L"COMBOBOX", L"", 137, 41, 95, 60, Codec, state->font);
                SendMessageW(codec, CB_ADDSTRING, 0, reinterpret_cast<LPARAM>(L"H.264"));
                SendMessageW(codec, CB_ADDSTRING, 0, reinterpret_cast<LPARAM>(L"HEVC"));
                SendMessageW(codec, CB_SETCURSEL, state->options.hevc ? 1 : 0, 0);
            }
            auto cursor = control(dialog, WS_TABSTOP | BS_AUTOCHECKBOX, L"BUTTON",
                state->english ? L"Record mouse pointer" : L"录制鼠标指针", 14, 96, 218, 16, Cursor, state->font);
            SendMessageW(cursor, BM_SETCHECK, state->options.cursor ? BST_CHECKED : BST_UNCHECKED, 0);
            control(dialog, WS_TABSTOP | BS_PUSHBUTTON, L"BUTTON", state->english ? L"Cancel" : L"取消", 106, 130, 60, 20, IDCANCEL, state->font);
            control(dialog, WS_TABSTOP | BS_DEFPUSHBUTTON, L"BUTTON", state->english ? L"Continue" : L"继续", 174, 130, 60, 20, IDOK, state->font);
            return TRUE;
        } catch (...) { EndDialog(dialog, -1); return TRUE; }
    }
    if (message == WM_COMMAND && state) {
        if (LOWORD(wp) == IDCANCEL) { EndDialog(dialog, IDCANCEL); return TRUE; }
        if (LOWORD(wp) == IDOK) {
            BOOL valid = FALSE;
            const auto fps = GetDlgItemInt(dialog, Fps, &valid, FALSE);
            if (!valid || fps < 1 || fps > (state->options.gif ? 30u : 60u)) { SetFocus(GetDlgItem(dialog, Fps)); return TRUE; }
            state->options.fps = static_cast<int>(fps);
            if (state->options.gif) {
                const auto quality = GetDlgItemInt(dialog, Quality, &valid, FALSE);
                if (!valid || quality < 1 || quality > 100) { SetFocus(GetDlgItem(dialog, Quality)); return TRUE; }
                state->options.quality = static_cast<int>(quality);
                const auto seconds = GetDlgItemInt(dialog, Seconds, &valid, FALSE);
                if (!valid || seconds < 1 || seconds > 600) { SetFocus(GetDlgItem(dialog, Seconds)); return TRUE; }
                state->options.seconds = static_cast<int>(seconds);
            } else state->options.hevc = SendDlgItemMessageW(dialog, Codec, CB_GETCURSEL, 0, 0) == 1;
            state->options.cursor = SendDlgItemMessageW(dialog, Cursor, BM_GETCHECK, 0, 0) == BST_CHECKED;
            EndDialog(dialog, IDOK); return TRUE;
        }
    }
    if (message == WM_CLOSE) { EndDialog(dialog, IDCANCEL); return TRUE; }
    return FALSE;
}
}
std::optional<RecordingOptions> chooseRecordingOptions(HWND owner, RecordingOptions current, bool english) {
    INITCOMMONCONTROLSEX controls{sizeof(controls), ICC_UPDOWN_CLASS};
    if (!InitCommonControlsEx(&controls)) throw std::runtime_error("Cannot initialize native recording controls");
    std::vector<WORD> words((sizeof(DLGTEMPLATE) + 1) / 2);
    auto* dialog = reinterpret_cast<DLGTEMPLATE*>(words.data());
    dialog->style = WS_POPUP | WS_CAPTION | WS_SYSMENU | DS_MODALFRAME | DS_SETFONT | DS_CENTER;
    dialog->dwExtendedStyle = 0; dialog->cdit = 0; dialog->x = 0; dialog->y = 0; dialog->cx = 248; dialog->cy = 164;
    words.push_back(0); words.push_back(0); words.push_back(0); words.push_back(9);
    const std::wstring font = L"Segoe UI";
    for (auto c : font) words.push_back(c); words.push_back(0);
    DialogState state{current, english};
    const auto result = DialogBoxIndirectParamW(GetModuleHandleW(nullptr), reinterpret_cast<const DLGTEMPLATE*>(words.data()), owner, procedure, reinterpret_cast<LPARAM>(&state));
    if (result == -1) throw std::runtime_error("Cannot display native recording options");
    return result == IDOK ? std::optional<RecordingOptions>(state.options) : std::nullopt;
}
}
