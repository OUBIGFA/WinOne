#include "collection_window.h"
#include "capture_collection.h"
#include <dwmapi.h>
#include <opencv2/imgproc.hpp>
#include <uxtheme.h>

namespace winone::capture {
CollectionWindow::~CollectionWindow() {
    if (window_) DestroyWindow(window_);
    if (images_) ImageList_Destroy(images_);
    if (font_) DeleteObject(font_);
    if (background_) DeleteObject(background_);
}
void CollectionWindow::show(HWND owner, bool english, bool dark) {
    const bool changedTheme = dark_ != dark;
    english_ = english;
    dark_ = dark;
    if (window_ && changedTheme) {
        DeleteObject(background_);
        background_ = CreateSolidBrush(dark_ ? RGB(35, 35, 35) : RGB(250, 250, 250));
        for (auto button : buttons_)
            SetWindowTheme(button, dark_ ? L"DarkMode_Explorer" : L"Explorer", nullptr);
        SetWindowTheme(list_, dark_ ? L"DarkMode_Explorer" : L"Explorer", nullptr);
        ListView_SetBkColor(list_, dark_ ? RGB(35, 35, 35) : RGB(250, 250, 250));
        ListView_SetTextColor(list_, dark_ ? RGB(235, 235, 235) : RGB(35, 35, 35));
        renderedRevision_ = static_cast<size_t>(-1);
        InvalidateRect(window_, nullptr, TRUE);
    }
    if (!window_) {
        INITCOMMONCONTROLSEX controls{sizeof(controls), ICC_LISTVIEW_CLASSES};
        InitCommonControlsEx(&controls);
        WNDCLASSW klass{};
        klass.hInstance = GetModuleHandleW(nullptr);
        klass.lpfnWndProc = procedure;
        klass.lpszClassName = L"WinOne.Capture.Collection";
        klass.hCursor = LoadCursor(nullptr, IDC_ARROW);
        RegisterClassW(&klass);
        window_ = CreateWindowExW(WS_EX_TOPMOST | WS_EX_TOOLWINDOW, klass.lpszClassName, L"WinOne",
                                  WS_POPUP | WS_BORDER | WS_CLIPCHILDREN, 0, 0, 600, 100, owner, nullptr,
                                  klass.hInstance, this);
        if (!window_) throw systemError("Cannot create capture collection");
        scale_ = static_cast<int>(GetDpiForWindow(window_));
        font_ = CreateFontW(-MulDiv(13, scale_, 96), 0, 0, 0, FW_NORMAL, FALSE, FALSE, FALSE, DEFAULT_CHARSET, 0, 0,
                            CLEARTYPE_QUALITY, 0, L"Segoe UI");
        background_ = CreateSolidBrush(dark_ ? RGB(35, 35, 35) : RGB(250, 250, 250));
        BOOL themed = dark_;
        DwmSetWindowAttribute(window_, 20, &themed, sizeof(themed));
        DWORD corner = 2;
        DwmSetWindowAttribute(window_, 33, &corner, sizeof(corner));
        SetWindowDisplayAffinity(window_, WDA_EXCLUDEFROMCAPTURE);
        status_ = CreateWindowW(L"STATIC", L"", WS_CHILD | WS_VISIBLE, 0, 0, 0, 0, window_, nullptr, klass.hInstance,
                                nullptr);
        for (int id = Continue; id <= Delete; ++id) {
            auto button =
                CreateWindowW(L"BUTTON", L"", WS_CHILD | WS_VISIBLE | WS_TABSTOP | BS_OWNERDRAW, 0, 0, 0, 0, window_,
                              reinterpret_cast<HMENU>(static_cast<INT_PTR>(id)), klass.hInstance, nullptr);
            buttons_.push_back(button);
            SendMessageW(button, WM_SETFONT, reinterpret_cast<WPARAM>(font_), TRUE);
            SetWindowTheme(button, dark_ ? L"DarkMode_Explorer" : L"Explorer", nullptr);
            SetWindowSubclass(button, keys, 1, reinterpret_cast<DWORD_PTR>(this));
        }
        list_ = CreateWindowExW(WS_EX_CLIENTEDGE, WC_LISTVIEWW, L"",
                                WS_CHILD | WS_TABSTOP | LVS_ICON | LVS_SINGLESEL | LVS_SHOWSELALWAYS | LVS_AUTOARRANGE,
                                0, 0, 0, 0, window_, reinterpret_cast<HMENU>(100), klass.hInstance, nullptr);
        ListView_SetExtendedListViewStyle(list_, LVS_EX_DOUBLEBUFFER | LVS_EX_LABELTIP);
        ListView_SetBkColor(list_, dark_ ? RGB(35, 35, 35) : RGB(250, 250, 250));
        ListView_SetTextBkColor(list_, CLR_NONE);
        ListView_SetTextColor(list_, dark_ ? RGB(235, 235, 235) : RGB(35, 35, 35));
        SetWindowTheme(list_, dark_ ? L"DarkMode_Explorer" : L"Explorer", nullptr);
        SendMessageW(list_, WM_SETFONT, reinterpret_cast<WPARAM>(font_), TRUE);
        SendMessageW(status_, WM_SETFONT, reinterpret_cast<WPARAM>(font_), TRUE);
        SetWindowSubclass(list_, keys, 1, reinterpret_cast<DWORD_PTR>(this));
        POINT cursor{};
        GetCursorPos(&cursor);
        MONITORINFO monitor{sizeof(monitor)};
        GetMonitorInfoW(MonitorFromPoint(cursor, MONITOR_DEFAULTTONEAREST), &monitor);
        width_ = std::min(MulDiv(600, scale_, 96), static_cast<int>(monitor.rcWork.right - monitor.rcWork.left) - 16);
        SetWindowPos(window_, HWND_TOPMOST,
                     monitor.rcWork.left + (monitor.rcWork.right - monitor.rcWork.left - width_) / 2,
                     monitor.rcWork.top + 12, width_, 100, SWP_NOACTIVATE);
    }
    const wchar_t *zh[] = {L"继续截图",   L"切换界面", L"查看片段", L"撤销", L"识别并编辑",
                           L"识别并复制", L"取消",     L"上移",     L"下移", L"删除"};
    const wchar_t *en[] = {L"Continue",      L"Switch window", L"Fragments", L"Undo",      L"Read and edit",
                           L"Read and copy", L"Cancel",        L"Move up",   L"Move down", L"Delete"};
    for (size_t i = 0; i < buttons_.size(); ++i)
        SetWindowTextW(buttons_[i], english_ ? en[i] : zh[i]);
    layout();
    ShowWindow(window_, SW_SHOWNOACTIVATE);
    SetWindowPos(window_, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE);
}
void CollectionWindow::hide() {
    if (window_) ShowWindow(window_, SW_HIDE);
}
void CollectionWindow::layout() {
    if (!window_) return;
    auto px = [this](int value) { return MulDiv(value, scale_, 96); };
    const int margin = px(10), gap = px(5), buttonWidth = px(english_ ? 104 : 88), height = px(30);
    MoveWindow(status_, margin, px(8), width_ - 2 * margin, px(24), TRUE);
    int x = margin, y = px(36);
    for (int id = Continue; id <= Cancel; ++id) {
        bool visible = id != Continue || suspended_;
        if (id == Switch) visible = !suspended_;
        ShowWindow(buttons_[id - 1], visible ? SW_SHOW : SW_HIDE);
        if (!visible) continue;
        if (x + buttonWidth + margin > width_) {
            x = margin;
            y += height + gap;
        }
        MoveWindow(buttons_[id - 1], x, y, buttonWidth, height, TRUE);
        x += buttonWidth + gap;
    }
    y += height + margin;
    ShowWindow(list_, drawer_ ? SW_SHOW : SW_HIDE);
    if (drawer_) {
        MoveWindow(list_, margin, y, width_ - 2 * margin, px(240), TRUE);
        y += px(248);
    }
    x = margin;
    for (int id = Up; id <= Delete; ++id) {
        ShowWindow(buttons_[id - 1], drawer_ ? SW_SHOW : SW_HIDE);
        if (drawer_) {
            MoveWindow(buttons_[id - 1], x, y, buttonWidth, height, TRUE);
            x += buttonWidth + gap;
        }
    }
    if (drawer_) y += height + margin;
    RECT bounds{};
    GetWindowRect(window_, &bounds);
    MONITORINFO monitor{sizeof(monitor)};
    GetMonitorInfoW(MonitorFromWindow(window_, MONITOR_DEFAULTTONEAREST), &monitor);
    const int left =
        std::clamp(static_cast<int>(bounds.left), static_cast<int>(monitor.rcWork.left),
                   std::max(static_cast<int>(monitor.rcWork.left), static_cast<int>(monitor.rcWork.right) - width_));
    const int top =
        std::clamp(static_cast<int>(bounds.top), static_cast<int>(monitor.rcWork.top),
                   std::max(static_cast<int>(monitor.rcWork.top), static_cast<int>(monitor.rcWork.bottom) - y));
    SetWindowPos(window_, nullptr, left, top, width_, y, SWP_NOZORDER | SWP_NOACTIVATE);
}
void CollectionWindow::refresh(const CaptureCollection &collection, bool suspended, bool busy,
                               const std::wstring &status) {
    if (!window_) return;
    count_ = collection.items().size();
    suspended_ = suspended;
    busy_ = busy;
    std::wstring label = status.empty() ? (english_ ? L"Collected " : L"已收集 ") + std::to_wstring(count_) +
                                              (english_ ? L" fragments" : L" 个片段")
                                        : status;
    SetWindowTextW(status_, label.c_str());
    // OCR progress changes only the status. Keep existing previews and list
    // scroll/selection rather than recreating every bitmap for every fragment.
    if (renderedRevision_ != collection.revision()) {
        const int selectedIndex = ListView_GetNextItem(list_, -1, LVNI_SELECTED);
        SendMessageW(list_, WM_SETREDRAW, FALSE, 0);
        ListView_DeleteAllItems(list_);
        if (images_) {
            ListView_SetImageList(list_, nullptr, LVSIL_NORMAL);
            ImageList_Destroy(images_);
        }
        const int w = MulDiv(144, scale_, 96), h = MulDiv(84, scale_, 96);
        images_ = ImageList_Create(w, h, ILC_COLOR32, static_cast<int>(count_), 1);
        ListView_SetImageList(list_, images_, LVSIL_NORMAL);
        ListView_SetIconSpacing(list_, w + MulDiv(22, scale_, 96), h + MulDiv(38, scale_, 96));
        int index = 0;
        for (const auto &fragment : collection.items()) {
            cv::Mat image;
            if (fragment.image.channels() == 4)
                image = fragment.image;
            else
                cv::cvtColor(fragment.image, image, cv::COLOR_BGR2BGRA);
            cv::Mat thumb(h, w, CV_8UC4, dark_ ? cv::Scalar(35, 35, 35, 255) : cv::Scalar(245, 245, 245, 255));
            double ratio = std::min(static_cast<double>(w) / image.cols, static_cast<double>(h) / image.rows);
            cv::Mat resized;
            cv::resize(
                image, resized,
                {std::max(1, static_cast<int>(image.cols * ratio)), std::max(1, static_cast<int>(image.rows * ratio))},
                0, 0, cv::INTER_AREA);
            resized.copyTo(thumb(cv::Rect((w - resized.cols) / 2, (h - resized.rows) / 2, resized.cols, resized.rows)));
            BITMAPINFO info{};
            info.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
            info.bmiHeader.biWidth = w;
            info.bmiHeader.biHeight = -h;
            info.bmiHeader.biPlanes = 1;
            info.bmiHeader.biBitCount = 32;
            void *pixels = nullptr;
            auto bitmap = CreateDIBSection(nullptr, &info, DIB_RGB_COLORS, &pixels, nullptr, 0);
            if (!bitmap) throw systemError("Cannot create fragment preview");
            std::memcpy(pixels, thumb.data, thumb.total() * 4);
            ImageList_Add(images_, bitmap, nullptr);
            DeleteObject(bitmap);
            std::wstring text = std::to_wstring(index + 1) + L" · " + std::to_wstring(fragment.image.cols) + L" × " +
                                std::to_wstring(fragment.image.rows);
            LVITEMW item{};
            item.mask = LVIF_TEXT | LVIF_IMAGE;
            item.iItem = index;
            item.iImage = index;
            item.pszText = text.data();
            ListView_InsertItem(list_, &item);
            ++index;
        }
        if (count_)
            ListView_SetItemState(list_, std::clamp(selectedIndex, 0, static_cast<int>(count_) - 1),
                                  LVIS_SELECTED | LVIS_FOCUSED, LVIS_SELECTED | LVIS_FOCUSED);
        SendMessageW(list_, WM_SETREDRAW, TRUE, 0);
        InvalidateRect(list_, nullptr, TRUE);
        renderedRevision_ = collection.revision();
    }
    for (int id = Continue; id <= Delete; ++id)
        EnableWindow(buttons_[id - 1], !busy || id == Cancel);
    EnableWindow(buttons_[Undo - 1], !busy && collection.canUndo());
    EnableWindow(buttons_[Recognize - 1], !busy && count_ > 0);
    EnableWindow(buttons_[Copy - 1], !busy && count_ > 0);
    selected();
    layout();
}
void CollectionWindow::selected() {
    if (!list_) return;
    int index = ListView_GetNextItem(list_, -1, LVNI_SELECTED);
    EnableWindow(buttons_[Up - 1], !busy_ && index > 0);
    EnableWindow(buttons_[Down - 1], !busy_ && index >= 0 && index + 1 < static_cast<int>(count_));
    EnableWindow(buttons_[Delete - 1], !busy_ && index >= 0);
}
void CollectionWindow::toggleDrawer() {
    drawer_ = !drawer_;
    layout();
    if (drawer_) SetFocus(list_);
}
void CollectionWindow::select(int index) {
    if (index < 0 || index >= static_cast<int>(count_)) return;
    ListView_SetItemState(list_, -1, 0, LVIS_SELECTED | LVIS_FOCUSED);
    ListView_SetItemState(list_, index, LVIS_SELECTED | LVIS_FOCUSED, LVIS_SELECTED | LVIS_FOCUSED);
    ListView_EnsureVisible(list_, index, FALSE);
}
void CollectionWindow::invoke(Action action) {
    if (busy_ && action != Cancel) return;
    if (action == Drawer) {
        toggleDrawer();
        return;
    }
    if (action_) action_(action, ListView_GetNextItem(list_, -1, LVNI_SELECTED));
}
LRESULT CALLBACK CollectionWindow::keys(HWND hwnd, UINT message, WPARAM wp, LPARAM lp, UINT_PTR, DWORD_PTR data) {
    auto *self = reinterpret_cast<CollectionWindow *>(data);
    if (message == WM_KEYDOWN) {
        if (wp == VK_RETURN) {
            self->invoke(hwnd == self->list_ ? Recognize : static_cast<Action>(GetDlgCtrlID(hwnd)));
            return 0;
        }
        if (wp == 'Z' && (GetKeyState(VK_CONTROL) & 0x8000)) {
            self->invoke(Undo);
            return 0;
        }
        if (wp == VK_DELETE && hwnd == self->list_) {
            self->invoke(Delete);
            return 0;
        }
        if (wp == VK_ESCAPE) {
            self->invoke(Cancel);
            return 0;
        }
    }
    return DefSubclassProc(hwnd, message, wp, lp);
}
LRESULT CALLBACK CollectionWindow::procedure(HWND hwnd, UINT message, WPARAM wp, LPARAM lp) {
    auto *self = reinterpret_cast<CollectionWindow *>(GetWindowLongPtrW(hwnd, GWLP_USERDATA));
    if (message == WM_NCCREATE) {
        self = static_cast<CollectionWindow *>(reinterpret_cast<CREATESTRUCTW *>(lp)->lpCreateParams);
        SetWindowLongPtrW(hwnd, GWLP_USERDATA, reinterpret_cast<LONG_PTR>(self));
    }
    if (self) {
        try {
            if (message == WM_DRAWITEM) {
                // Keep native button semantics and accessibility; paint with
                // the same neutral roles used by the existing capture toolbar.
                auto &item = *reinterpret_cast<DRAWITEMSTRUCT *>(lp);
                if (item.CtlType != ODT_BUTTON) return FALSE;
                const bool disabled = (item.itemState & ODS_DISABLED) != 0,
                           pressed = (item.itemState & ODS_SELECTED) != 0;
                const auto face = self->dark_ ? (pressed ? RGB(59, 59, 56) : RGB(41, 41, 40))
                                              : (pressed ? RGB(222, 222, 218) : RGB(240, 240, 238));
                const auto border = self->dark_ ? RGB(69, 69, 65) : RGB(206, 206, 202);
                const auto foreground = disabled ? (self->dark_ ? RGB(130, 130, 125) : RGB(120, 120, 115))
                                                 : (self->dark_ ? RGB(231, 231, 226) : RGB(41, 41, 39));
                FillRect(item.hDC, &item.rcItem, self->background_);
                auto brush = CreateSolidBrush(face);
                auto pen = CreatePen(PS_SOLID, 1, border);
                auto oldBrush = SelectObject(item.hDC, brush), oldPen = SelectObject(item.hDC, pen),
                     oldFont = SelectObject(item.hDC, self->font_);
                const int radius = MulDiv(10, self->scale_, 96);
                RoundRect(item.hDC, item.rcItem.left, item.rcItem.top, item.rcItem.right, item.rcItem.bottom, radius,
                          radius);
                wchar_t label[128]{};
                GetWindowTextW(item.hwndItem, label, 128);
                SetTextColor(item.hDC, foreground);
                SetBkMode(item.hDC, TRANSPARENT);
                RECT textRect = item.rcItem;
                InflateRect(&textRect, -4, 0);
                DrawTextW(item.hDC, label, -1, &textRect, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_END_ELLIPSIS);
                if ((item.itemState & ODS_FOCUS) != 0) {
                    auto focus = item.rcItem;
                    InflateRect(&focus, -3, -3);
                    DrawFocusRect(item.hDC, &focus);
                }
                SelectObject(item.hDC, oldFont);
                SelectObject(item.hDC, oldBrush);
                SelectObject(item.hDC, oldPen);
                DeleteObject(brush);
                DeleteObject(pen);
                return TRUE;
            }
            if (message == WM_COMMAND) {
                self->invoke(static_cast<Action>(LOWORD(wp)));
                return 0;
            }
            if (message == WM_NOTIFY) {
                self->selected();
                return 0;
            }
            if (message == WM_CLOSE) {
                self->invoke(Cancel);
                return 0;
            }
            if (message == WM_ERASEBKGND) {
                RECT r{};
                GetClientRect(hwnd, &r);
                FillRect(reinterpret_cast<HDC>(wp), &r, self->background_);
                return 1;
            }
            if (message == WM_CTLCOLORSTATIC) {
                auto dc = reinterpret_cast<HDC>(wp);
                SetTextColor(dc, self->dark_ ? RGB(235, 235, 235) : RGB(35, 35, 35));
                SetBkMode(dc, TRANSPARENT);
                return reinterpret_cast<LRESULT>(self->background_);
            }
            if (message == WM_NCHITTEST) {
                auto hit = DefWindowProcW(hwnd, message, wp, lp);
                return hit == HTCLIENT ? HTCAPTION : hit;
            }
        } catch (const std::exception &e) {
            SetWindowTextW(self->status_, wide(e.what()).c_str());
        }
    }
    return DefWindowProcW(hwnd, message, wp, lp);
}
} // namespace winone::capture
