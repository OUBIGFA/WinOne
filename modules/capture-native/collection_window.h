#pragma once
#include "win32_support.h"
#include <commctrl.h>
#include <opencv2/core.hpp>

namespace winone::capture {
class CaptureCollection;

// Native adaptation of SnapFind's MultiSessionBarWindow. Common controls own
// keyboard/focus/list selection; the capture canvas keeps owning screen input.
class CollectionWindow {
  public:
    enum Action { Continue = 1, Switch, Drawer, Undo, Recognize, Copy, Cancel, Up, Down, Delete };
    explicit CollectionWindow(std::function<void(Action, int)> action) : action_(std::move(action)) {}
    ~CollectionWindow();
    void show(HWND owner, bool english, bool dark);
    void hide();
    void refresh(const CaptureCollection &, bool suspended, bool busy, const std::wstring &status = L"");
    void toggleDrawer();
    void select(int index);
    HWND window() const {
        return window_;
    }
    bool drawerOpen() const {
        return drawer_;
    }

  private:
    static LRESULT CALLBACK procedure(HWND, UINT, WPARAM, LPARAM);
    static LRESULT CALLBACK keys(HWND, UINT, WPARAM, LPARAM, UINT_PTR, DWORD_PTR);
    void layout();
    void selected();
    void invoke(Action);
    HWND window_ = nullptr, list_ = nullptr, status_ = nullptr;
    std::vector<HWND> buttons_;
    HFONT font_ = nullptr;
    HBRUSH background_ = nullptr;
    HIMAGELIST images_ = nullptr;
    bool english_ = false, dark_ = false, drawer_ = false, busy_ = false, suspended_ = false;
    int width_ = 600, scale_ = 96;
    size_t count_ = 0;
    size_t renderedRevision_ = static_cast<size_t>(-1);
    std::function<void(Action, int)> action_;
};
} // namespace winone::capture
