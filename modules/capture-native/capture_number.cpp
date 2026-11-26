#include "capture_app.h"
#include "number_label.h"
#include <windowsx.h>
#include "core/events/MainThreadDispatcher.h"

namespace winone::capture {
namespace up = tools3000::capture;

void CaptureApp::editNumber(uint32_t id) {
    closeNumberEditor(false);
    if (numberEdit_) return;
    auto& overlay = up::CaptureOverlay::instance();
    auto& state = overlay.winoneState();
    auto* element = state.markup.getElementById(id);
    if (!overlay.isOverlayVisible() || !element || element->tool != up::MarkupTool::Number) return;
    editingNumber_ = id; previousNumberText_ = element->text;
    numberComposing_ = false;
    numberOverflow_ = false;
    state.currentTool = up::MarkupTool::Number; state.isMarkupToolActive = true;
    numberBefore_ = state.markup.captureSnapshot();
    element->textOutline = false;
    element->isEditing = true;
    if (element->text.empty()) {
        const int width=std::abs(state.dragEnd.x-state.dragStart.x),height=std::abs(state.dragEnd.y-state.dragStart.y);
        const auto metrics = number_label::layout(*element);
        const int badge = metrics.badgeR;
        const int bubbleReserve = metrics.gap + metrics.tailW + metrics.padX * 2;
        const int minLabel = std::max(72, static_cast<int>(std::ceil(element->fontSize * 4.5f)));
        const int right = width - badge - bubbleReserve - minLabel - 8;
        const int top = std::max(badge + 4, static_cast<int>(std::ceil(element->fontSize * .95f)) + 6);
        const int bottom = height - std::max(badge + 4, static_cast<int>(std::ceil(element->fontSize * .95f))) - 8;
        if(right<badge+2 || bottom<top){notice("选区无法容纳此字号的序号说明，请调小字号或扩大选区");return;}
        const cv::Point fitted{std::clamp(element->startPt.x,badge+2,right),std::clamp(element->startPt.y,top,bottom)};
        element->moveBy(fitted.x-element->startPt.x,fitted.y-element->startPt.y);
    }
    const auto boxLayout = number_label::layout(*element);
    const float remaining = static_cast<float>(std::abs(state.dragEnd.x - state.dragStart.x) - boxLayout.textRect.x - boxLayout.padX - 8);
    element->textMaxWidth = std::max(32.f, remaining);
    const auto bg = number_label::calloutColor();
    if (numberBackground_) DeleteObject(numberBackground_);
    numberBackground_ = CreateSolidBrush(RGB(bg.r, bg.g, bg.b));
    const auto parent = overlay.winoneWindow();
    numberEdit_ = CreateWindowExW(0, L"EDIT", wide(element->text).c_str(), WS_CHILD | WS_VISIBLE | WS_TABSTOP |
        ES_MULTILINE | ES_AUTOVSCROLL | ES_WANTRETURN, 0, 0, 120, 30, parent, reinterpret_cast<HMENU>(3101), GetModuleHandleW(nullptr), nullptr);
    if (!numberEdit_) { notice("无法打开序号说明输入框"); return; }
    SetWindowSubclass(parent, numberProcedure, 3100, reinterpret_cast<DWORD_PTR>(this));
    SetWindowSubclass(numberEdit_, numberProcedure, 3101, reinterpret_cast<DWORD_PTR>(this));
    const float zoom = state.isLongImageMode() ? state.longCanvas.zoom : 1.f;
    numberFont_ = CreateFontW(-static_cast<int>(std::lround(element->fontSize * zoom)), 0, 0, 0, FW_NORMAL, FALSE, FALSE, FALSE,
        DEFAULT_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY, DEFAULT_PITCH, L"Segoe UI");
    SendMessageW(numberEdit_, WM_SETFONT, reinterpret_cast<WPARAM>(numberFont_), TRUE);
    SendMessageW(numberEdit_, EM_SETLIMITTEXT, 4096, 0);
    SendMessageW(numberEdit_, EM_SETMARGINS, EC_LEFTMARGIN | EC_RIGHTMARGIN, MAKELPARAM(0, 0));
    updateNumberEditor();
    SetFocus(numberEdit_); SendMessageW(numberEdit_, EM_SETSEL, 0, -1);
}

void CaptureApp::updateNumberEditor() {
    if (!numberEdit_ || closingNumber_) return;
    auto& overlay = up::CaptureOverlay::instance();
    auto& state = overlay.winoneState();
    auto* element = state.markup.getElementById(editingNumber_);
    if (!element) { closeNumberEditor(false); return; }
    const int count = GetWindowTextLengthW(numberEdit_);
    std::wstring text(static_cast<size_t>(count) + 1, L'\0');
    const auto copied = GetWindowTextW(numberEdit_, text.data(), static_cast<int>(text.size()));
    text.resize(copied);
    element->text = utf8(text); element->textRenderSize = {};
    auto label = number_label::textElement(*element);
    const auto bounds = number_label::textBounds(*element);
    const int remainingHeight=std::max(1,static_cast<int>(std::abs(state.dragEnd.y-state.dragStart.y))-bounds.y-5);
    numberOverflow_=!text.empty() && bounds.height>remainingHeight;
    const int originX = std::min(state.dragStart.x, state.dragEnd.x), originY = std::min(state.dragStart.y, state.dragEnd.y);
    const auto position = state.docToScreen(static_cast<float>(originX + bounds.x), static_cast<float>(originY + bounds.y));
    const float zoom = state.isLongImageMode() ? state.longCanvas.zoom : 1.f;
    SetWindowPos(numberEdit_, HWND_TOP, static_cast<int>(position.x), static_cast<int>(position.y),
        std::max(36, static_cast<int>(std::ceil(bounds.width * zoom))), std::max(24, static_cast<int>(std::ceil(std::min(bounds.height,remainingHeight) * zoom))), SWP_NOACTIVATE);
    overlay.winoneInvalidate();
}

void CaptureApp::closeNumberEditor(bool cancel) {
    if (!numberEdit_ || closingNumber_) return;
    if (!cancel && numberOverflow_) { notice(language_=="en-US"?"Text exceeds the capture area. Shorten it before saving.":"说明超出截图范围，请缩短内容后再确认"); return; }
    closingNumber_ = true;
    auto& overlay = up::CaptureOverlay::instance();
    auto* element = overlay.winoneState().markup.getElementById(editingNumber_);
    if (element) element->isEditing = false;
    if (element && cancel) { element->text = previousNumberText_; element->textRenderSize = {}; }
    if (element && !cancel && element->text != previousNumberText_ && numberBefore_)
        overlay.winoneState().markup.commitUndoSnapshot(std::move(*numberBefore_));
    numberBefore_.reset();
    const auto window = numberEdit_; numberEdit_ = nullptr;
    RemoveWindowSubclass(window, numberProcedure, 3101); DestroyWindow(window);
    if (numberFont_) { DeleteObject(numberFont_); numberFont_ = nullptr; }
    closingNumber_ = false; editingNumber_ = 0; previousNumberText_.clear();
    overlay.winoneInvalidate();
}

LRESULT CALLBACK CaptureApp::numberProcedure(HWND window, UINT message, WPARAM w, LPARAM l, UINT_PTR id, DWORD_PTR data) {
    auto* app = reinterpret_cast<CaptureApp*>(data);
    if (id == 3100) {
        if (message == WM_CTLCOLOREDIT && reinterpret_cast<HWND>(l) == app->numberEdit_) {
            const auto bg = number_label::calloutColor();
            const auto fg = number_label::textForegroundColor();
            SetTextColor(reinterpret_cast<HDC>(w), RGB(fg.r, fg.g, fg.b));
            SetBkColor(reinterpret_cast<HDC>(w), RGB(bg.r, bg.g, bg.b));
            return reinterpret_cast<LRESULT>(app->numberBackground_);
        }
        if (message == WM_COMMAND && reinterpret_cast<HWND>(l) == app->numberEdit_ && HIWORD(w) == EN_CHANGE) app->updateNumberEditor();
        if (message == WM_NCDESTROY) RemoveWindowSubclass(window, numberProcedure, id);
        return DefSubclassProc(window, message, w, l);
    }
    if (message == WM_IME_STARTCOMPOSITION) app->numberComposing_ = true;
    if (message == WM_IME_ENDCOMPOSITION) app->numberComposing_ = false;
    if (message == WM_PAINT && GetWindowTextLengthW(window)==0) {
        const auto result=DefSubclassProc(window,message,w,l);
        const auto dc=GetDC(window);const auto old=SelectObject(dc,app->numberFont_);
        RECT bounds{};GetClientRect(window,&bounds);
        const auto ph = number_label::placeholderColor();
        SetBkMode(dc,TRANSPARENT);SetTextColor(dc,RGB(ph.r,ph.g,ph.b));
        DrawTextW(dc,app->language_=="en-US"?L"Add a note":L"输入说明",-1,&bounds,DT_LEFT|DT_TOP|DT_NOPREFIX);
        SelectObject(dc,old);ReleaseDC(window,dc);return result;
    }
    if (message == WM_KEYDOWN && !app->numberComposing_ && w == 'S' && (GetKeyState(VK_CONTROL) & 0x8000)) {
        const bool saveAs = (GetKeyState(VK_SHIFT) & 0x8000) != 0;
        app->closeNumberEditor(false);
        if(app->numberEdit_) return 0;
        tools3000::core::MainThreadDispatcher::instance().postDeferred([app, saveAs] {
            try {
                const auto path = saveAs ? app->chooseSavePath(app->suggestedPath()) : std::filesystem::path{};
                if (saveAs && path.empty()) return;
                up::CaptureOverlay::instance().confirmCurrentSelection({saveAs ? up::CaptureCompletionAction::SaveAs : up::CaptureCompletionAction::QuickSave, utf8(path.wstring())});
            } catch (const std::exception& error) { app->notice(error.what()); }
        });
        return 0;
    }
    if (message == WM_KEYDOWN && !app->numberComposing_ && (w == VK_ESCAPE || (w == VK_RETURN && (GetKeyState(VK_CONTROL) & 0x8000)))) {
        app->closeNumberEditor(w == VK_ESCAPE);
        SetFocus(up::CaptureOverlay::instance().winoneWindow()); return 0;
    }
    if (message == WM_KILLFOCUS) { app->closeNumberEditor(false); return 0; }
    if (message == WM_NCDESTROY) {
        RemoveWindowSubclass(window, numberProcedure, id);
        if (app->numberEdit_ == window) {
            app->numberEdit_ = nullptr; app->numberBefore_.reset();
            if (app->numberFont_) { DeleteObject(app->numberFont_); app->numberFont_ = nullptr; }
        }
    }
    return DefSubclassProc(window, message, w, l);
}
}
