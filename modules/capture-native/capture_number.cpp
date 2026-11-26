#include "capture_app.h"
#include "number_label.h"
#include "capture/CaptureTextEditor.h"
#include <windowsx.h>
#include "core/events/MainThreadDispatcher.h"

namespace winone::capture {
namespace up = tools3000::capture;
namespace { constexpr UINT updateNumberLayout = WM_APP + 3101; }

void CaptureApp::editNumber(uint32_t id) {
    closeNumberEditor(false);
    if (numberEdit_) return;
    auto& overlay = up::CaptureOverlay::instance();
    auto& state = overlay.winoneState();
    auto* element = state.markup.getElementById(id);
    if (!overlay.isOverlayVisible() || !element || element->tool != up::MarkupTool::Number) return;
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
    const auto parent = overlay.winoneWindow();
    // Reuse Win32++'s RichEdit wrapper. Windows owns text editing, selection,
    // undo, IME, caret and painting; WinOne only places and commits the note.
    try {
        numberEditor_ = std::make_unique<winone::community::RichEdit>();
        numberEdit_ = numberEditor_->Create(parent, 3101);
    } catch (const std::exception& error) {
        numberEditor_.reset(); element->isEditing = false; notice(error.what()); return;
    }
    editingNumber_ = id; previousNumberText_ = element->text;
    numberComposing_ = false; numberOverflow_ = false; numberRequestedHeight_ = 0; numberLayoutQueued_ = false;
    state.currentTool = up::MarkupTool::Number; state.isMarkupToolActive = true;
    numberBefore_ = state.markup.captureSnapshot();
    element->textOutline = false; element->isEditing = true;
    element->winoneNumberEditorSize = {};
    element->winoneNumberEditorLineHeight = 0;
    const auto boxLayout = number_label::layout(*element);
    const float remaining = static_cast<float>(std::abs(state.dragEnd.x - state.dragStart.x) - boxLayout.textRect.x - boxLayout.padX - 8);
    element->textMaxWidth = std::max(32.f, remaining);
    const auto bg = number_label::calloutColor();
    SetWindowSubclass(parent, numberProcedure, 3100, reinterpret_cast<DWORD_PTR>(this));
    SetWindowSubclass(numberEdit_, numberProcedure, 3101, reinterpret_cast<DWORD_PTR>(this));
    const float zoom = state.isLongImageMode() ? state.longCanvas.zoom : 1.f;
    const int fontPixels = std::max(1, static_cast<int>(std::lround(element->fontSize * zoom)));
    // Match the community Text handler's family/weight in both editing and
    // committed output, so finishing a note does not change its typography.
    const auto family = up::CaptureTextEditor::resolveMarkupFontFamilyName(false);
    numberFont_ = CreateFontW(-fontPixels, 0, 0, 0, FW_BOLD, FALSE, FALSE, FALSE,
        DEFAULT_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY, DEFAULT_PITCH, family);
    numberEditor_->SetTextMode(TM_PLAINTEXT | TM_MULTILEVELUNDO | TM_MULTICODEPAGE);
    numberEditor_->SetBackgroundColor(FALSE, RGB(bg.r, bg.g, bg.b));
    CHARFORMAT2W format{}; format.cbSize = sizeof(format);
    format.dwMask = CFM_FACE | CFM_SIZE | CFM_COLOR | CFM_BOLD | CFM_ITALIC;
    format.dwEffects = CFE_BOLD;
    format.crTextColor = RGB(255, 255, 255);
    const auto dc = GetDC(numberEdit_);
    format.yHeight = MulDiv(fontPixels, 1440, GetDeviceCaps(dc, LOGPIXELSY));
    ReleaseDC(numberEdit_, dc);
    wcscpy_s(format.szFaceName, family);
    numberEditor_->SetDefaultCharFormat(format);
    SendMessageW(numberEdit_, EM_EXLIMITTEXT, 0, 4096);
    SetWindowTextW(numberEdit_, wide(element->text).c_str());
    numberEditor_->SetEventMask(ENM_CHANGE | ENM_REQUESTRESIZE);
    updateNumberEditor();
    ShowWindow(numberEdit_, SW_SHOW);
    SetFocus(numberEdit_); numberEditor_->SetSel(0, -1);
}

void CaptureApp::updateNumberEditor() {
    if (!numberEdit_ || closingNumber_ || sizingNumber_) return;
    auto& overlay = up::CaptureOverlay::instance();
    auto& state = overlay.winoneState();
    auto* element = state.markup.getElementById(editingNumber_);
    if (!element) { closeNumberEditor(false); return; }
    sizingNumber_ = true;
    auto text = numberEditor_->GetTextRange(0, -1);
    // RichEdit uses CR paragraph separators; the community document uses LF.
    std::replace(text.begin(), text.end(), L'\r', L'\n');
    element->text = utf8(text); element->textRenderSize = {};
    const float zoom = state.isLongImageMode() ? state.longCanvas.zoom : 1.f;
    // Measure desired single-line width, then let RichEdit report the actual
    // wrapped height. This preserves normal input behavior at the selection edge.
    const auto dc = GetDC(numberEdit_); const auto previous = SelectObject(dc, numberFont_);
    RECT measured{};
    DrawTextW(dc, text.empty() ? L"输入说明" : text.c_str(), -1, &measured, DT_CALCRECT | DT_NOPREFIX | DT_EXPANDTABS);
    TEXTMETRICW metrics{}; GetTextMetricsW(dc, &metrics);
    SelectObject(dc, previous); ReleaseDC(numberEdit_, dc);
    const int maxWidth = std::max(1, static_cast<int>(std::floor(element->textMaxWidth * zoom)));
    const int width = std::clamp(std::max(static_cast<int>(std::ceil(72 * zoom)), static_cast<int>(measured.right) + 14), 1, maxWidth);
    const int lineHeight = metrics.tmHeight + metrics.tmExternalLeading;
    RECT current{}; GetClientRect(numberEdit_, &current);
    SetWindowPos(numberEdit_, nullptr, 0, 0, width, std::max(lineHeight + 4, static_cast<int>(current.bottom)), SWP_NOMOVE | SWP_NOZORDER | SWP_NOACTIVATE);
    numberEditor_->SetRect({6, 4, std::max(7, width - 6), std::max(5, static_cast<int>(current.bottom) - 4)});
    numberRequestedHeight_ = lineHeight;
    numberEditor_->RequestResize();
    // EN_REQUESTRESIZE reports the text height. Both 4-pixel formatting
    // insets must be added, otherwise RichEdit scrolls a whole line for 4 px.
    const int desiredHeight = std::max(lineHeight, numberRequestedHeight_) + 8;
    element->winoneNumberEditorSize = {static_cast<int>(std::ceil(width / zoom)), static_cast<int>(std::ceil(desiredHeight / zoom))};
    element->winoneNumberEditorLineHeight = static_cast<int>(std::ceil((lineHeight + 8) / zoom));
    const auto bounds = number_label::textBounds(*element);
    const int remainingHeight = std::max(1, static_cast<int>(std::abs(state.dragEnd.y - state.dragStart.y)) - bounds.y - number_label::layout(*element).padY - 2);
    numberOverflow_ = !text.empty() && bounds.height > remainingHeight;
    const int originX = std::min(state.dragStart.x, state.dragEnd.x), originY = std::min(state.dragStart.y, state.dragEnd.y);
    const auto position = state.docToScreen(static_cast<float>(originX + bounds.x), static_cast<float>(originY + bounds.y));
    SetWindowPos(numberEdit_, HWND_TOP, static_cast<int>(position.x), static_cast<int>(position.y),
        width, std::max(1, static_cast<int>(std::floor(std::min(bounds.height, remainingHeight) * zoom))), SWP_NOACTIVATE);
    GetClientRect(numberEdit_, &current);
    numberEditor_->SetRect({6, 4, std::max(7L, current.right - 6), std::max(5L, current.bottom - 4)});
    // RichEdit may scroll to the caret while EN_CHANGE is resizing the old
    // one-line window. Once it fits, restore the complete visible paragraph.
    if (!numberOverflow_) {
        POINT origin{};
        SendMessageW(numberEdit_, EM_SETSCROLLPOS, 0, reinterpret_cast<LPARAM>(&origin));
    }
    sizingNumber_ = false;
    overlay.winoneInvalidate();
}

void CaptureApp::closeNumberEditor(bool cancel) {
    if (!numberEdit_ || closingNumber_) return;
    if (!cancel && numberOverflow_) { notice(language_=="en-US"?"Text exceeds the capture area. Shorten it before saving.":"说明超出截图范围，请缩短内容后再确认"); return; }
    closingNumber_ = true;
    auto& overlay = up::CaptureOverlay::instance();
    auto* element = overlay.winoneState().markup.getElementById(editingNumber_);
    if (element) { element->isEditing = false; element->winoneNumberEditorSize = {}; element->winoneNumberEditorLineHeight = 0; }
    if (element && cancel) { element->text = previousNumberText_; element->textRenderSize = {}; }
    if (element && !cancel && element->text != previousNumberText_ && numberBefore_)
        overlay.winoneState().markup.commitUndoSnapshot(std::move(*numberBefore_));
    numberBefore_.reset();
    const auto window = numberEdit_; numberEdit_ = nullptr;
    RemoveWindowSubclass(overlay.winoneWindow(), numberProcedure, 3100);
    RemoveWindowSubclass(window, numberProcedure, 3101); DestroyWindow(window);
    numberEditor_->Detach(); numberEditor_.reset();
    if (numberFont_) { DeleteObject(numberFont_); numberFont_ = nullptr; }
    closingNumber_ = false; editingNumber_ = 0; previousNumberText_.clear();
    overlay.winoneInvalidate();
}

LRESULT CALLBACK CaptureApp::numberProcedure(HWND window, UINT message, WPARAM w, LPARAM l, UINT_PTR id, DWORD_PTR data) {
    auto* app = reinterpret_cast<CaptureApp*>(data);
    if (id == 3100) {
        if (message == WM_NOTIFY && reinterpret_cast<NMHDR*>(l)->hwndFrom == app->numberEdit_ &&
            reinterpret_cast<NMHDR*>(l)->code == EN_REQUESTRESIZE) {
            const auto* resize = reinterpret_cast<REQRESIZE*>(l);
            app->numberRequestedHeight_ = resize->rc.bottom - resize->rc.top;
            // Inline composition changes RichEdit's layout before EN_CHANGE.
            // Consume those native requests too, after its current edit ends;
            // otherwise the old one-line viewport scrolls preceding text away.
            // Requests raised by our own sizing pass are already handled there.
            if (!app->sizingNumber_ && !app->closingNumber_ && !app->numberLayoutQueued_) {
                if (!PostMessageW(app->numberEdit_, updateNumberLayout, 0, 0))
                    throw std::runtime_error("Cannot queue number note layout");
                app->numberLayoutQueued_ = true;
            }
            return 0;
        }
        if (message == WM_COMMAND && reinterpret_cast<HWND>(l) == app->numberEdit_ && HIWORD(w) == EN_CHANGE) app->updateNumberEditor();
        if (message == WM_NCDESTROY) RemoveWindowSubclass(window, numberProcedure, id);
        return DefSubclassProc(window, message, w, l);
    }
    if (message == updateNumberLayout) {
        app->numberLayoutQueued_ = false;
        app->updateNumberEditor();
        return 0;
    }
    if (message == WM_IME_STARTCOMPOSITION) app->numberComposing_ = true;
    if (message == WM_IME_ENDCOMPOSITION) app->numberComposing_ = false;
    if (message == WM_SETCURSOR && LOWORD(l) == HTCLIENT) {
        // DefWindowProc would first ask the screenshot parent, which owns a
        // crosshair/move cursor. Text selection belongs to this control.
        SetCursor(LoadCursorW(nullptr, IDC_IBEAM)); return TRUE;
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
    if (message == WM_KILLFOCUS) {
        const auto result = DefSubclassProc(window, message, w, l);
        app->closeNumberEditor(false); return result;
    }
    if (message == WM_NCDESTROY) {
        RemoveWindowSubclass(window, numberProcedure, id);
        if (app->numberEdit_ == window) {
            app->numberEdit_ = nullptr; app->numberBefore_.reset();
            app->numberEditor_->Detach();
            if (app->numberFont_) { DeleteObject(app->numberFont_); app->numberFont_ = nullptr; }
        }
    }
    return DefSubclassProc(window, message, w, l);
}
}
