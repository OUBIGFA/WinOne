#include "capture/FloatingGlassBar.h"
#include "capture_app.h"
#include "core/events/MainThreadDispatcher.h"
#include "core/hotkey/KeyboardHook.h"
#include "core/utils/WinUtils.h"
#include <dwrite.h>
#include <opencv2/imgproc.hpp>
#include <windowsx.h>
#include <wrl/client.h>

namespace winone::capture {
namespace upstream = tools3000::capture;
using tools3000::core::MainThreadDispatcher;

void CaptureApp::beginCollection() {
    if (collecting_ || active_.empty()) return;
    closeNumberEditor(false);
    if (numberEdit_) return;
    collecting_ = true;
    collectionSuspended_ = false;
    collectionScreen_ = 1;
    collectionWindow_ = std::make_unique<CollectionWindow>([this](auto action, int index) {
        MainThreadDispatcher::instance().postDeferred([this, action, index] {
            try {
                collectionAction(action, index);
            } catch (const std::exception &e) {
                refreshCollection(wide(e.what()));
            }
        });
    });
    auto &overlay = upstream::CaptureOverlay::instance();
    overlay.winoneState().winoneCollecting = true;
    collectSelection();
    refreshCollection();
}

void CaptureApp::collectSelection() {
    if (!collecting_ || textBusy_) return;
    auto &overlay = upstream::CaptureOverlay::instance();
    auto &state = overlay.winoneState();
    if (state.state.load() != upstream::OverlayState::Selected && state.state.load() != upstream::OverlayState::Marking)
        return;
    const int x = std::min(state.dragStart.x, state.dragEnd.x), y = std::min(state.dragStart.y, state.dragEnd.y);
    auto rect =
        cv::Rect(x, y, std::abs(state.dragStart.x - state.dragEnd.x), std::abs(state.dragStart.y - state.dragEnd.y)) &
        cv::Rect(0, 0, state.frozenScreen.cols, state.frozenScreen.rows);
    if (rect.empty()) return;
    cv::Mat image = state.frozenScreen(rect);
    if (state.markupBaseReady && state.markup.elementCount()) image = state.markup.getCompositeImage(true);
    if (state.winoneShowTranslated && !state.winoneTranslated.empty()) image = state.winoneTranslated;
    collection_.add(image, rect, collectionScreen_);
    state.activeElement = nullptr;
    state.markup.resetSession();
    state.markupBaseReady = false;
    state.isMarkupToolActive = false;
    state.winoneTranslated.release();
    state.winoneShowTranslated = false;
    state.winoneMoveTool = false;
    state.winoneRegionTool = false;
    state.dragStart = {};
    state.dragEnd = {};
    state.dragging = false;
    state.state = upstream::OverlayState::Selecting;
    state.toolbarButtons.clear();
    state.toolbarLayoutValid = false;
    overlay.winoneInvalidate();
    refreshCollection();
}

void CaptureApp::refreshCollection(const std::wstring &status) {
    if (!collecting_ || !collectionWindow_) return;
    auto &overlay = upstream::CaptureOverlay::instance();
    const auto theme = captureSettings().value("theme", std::string("system"));
    collectionWindow_->show(window_, language_ == "en-US",
                            theme == "dark" ||
                                (theme == "system" && upstream::FloatingGlassBar::resolveHudPalette().isDark));
    collectionWindow_->refresh(collection_, collectionSuspended_, textBusy_, status);
    overlay.winoneInvalidate();
}

void CaptureApp::collectionAction(CollectionWindow::Action action, int index) {
    if (!collecting_) return;
    if (textBusy_ && action != CollectionWindow::Cancel) return;
    auto &overlay = upstream::CaptureOverlay::instance();
    switch (action) {
    case CollectionWindow::Continue: {
        collectionWindow_->hide();
        // A fresh screen contains none of the staged previews or the bar.
        upstream::CaptureOptions options;
        options.copyToClipboard = false;
        options.saveToFile = false;
        options.recordHistory = false;
        options.showShortcutHints = false;
        options.autoDetectWindow = true;
        options.autoBypassFullscreen = false;
        preserving_ = true;
        try {
            overlay.startSelection(options);
        } catch (...) {
            preserving_ = false;
            throw;
        }
        preserving_ = false;
        if (!overlay.isOverlayVisible()) {
            refreshCollection(language_ == "en-US" ? L"Cannot capture screen; fragments retained"
                                                   : L"无法截取屏幕，片段已保留");
            return;
        }
        ++collectionScreen_;
        collectionSuspended_ = false;
        auto &state = overlay.winoneState();
        state.winoneCollecting = true;
        state.winoneLiveCapture = true;
        refreshCollection();
        break;
    }
    case CollectionWindow::Switch:
        collectSelection();
        collectionSuspended_ = true;
        tools3000::core::KeyboardHook::instance().setCaptureOverlayVisible(false);
        ShowWindow(overlay.winoneWindow(), SW_HIDE);
        refreshCollection();
        break;
    case CollectionWindow::Undo:
        collection_.undo();
        refreshCollection();
        break;
    case CollectionWindow::Up:
        if (index > 0) collection_.move(index, index - 1);
        refreshCollection();
        if (index > 0) collectionWindow_->select(index - 1);
        break;
    case CollectionWindow::Down:
        if (index >= 0 && index + 1 < static_cast<int>(collection_.items().size())) collection_.move(index, index + 1);
        refreshCollection();
        if (index >= 0) collectionWindow_->select(index + 1);
        break;
    case CollectionWindow::Delete:
        if (index >= 0) collection_.erase(index);
        refreshCollection();
        break;
    case CollectionWindow::Recognize:
        finishCollection(false);
        break;
    case CollectionWindow::Copy:
        finishCollection(true);
        break;
    case CollectionWindow::Cancel:
        operationCancelled_->store(true);
        clearCollection();
        overlay.cancel();
        break;
    default:
        break;
    }
}

void CaptureApp::finishCollection(bool copy) {
    collectSelection();
    if (collection_.items().empty()) return;
    if (!host_) throw std::runtime_error("WinOne 后台不可用，片段已保留");
    textBusy_ = true;
    refreshCollection(language_ == "en-US" ? L"Recognizing fragments…" : L"正在识别片段…");
    const auto fragments = collection_.items();
    const auto cancelled = operationCancelled_;
    const auto id = active_;
    background([this, fragments, cancelled, id, copy] {
        try {
            std::string text;
            size_t index = 0;
            for (const auto &fragment : fragments) {
                auto value = host_->recognize(fragment.image, cancelled.get(), true);
                const auto start = value.find_first_not_of(" \r\n\t"), end = value.find_last_not_of(" \r\n\t");
                if (start != std::string::npos) {
                    if (!text.empty()) text += '\n';
                    text += value.substr(start, end - start + 1);
                }
                ++index;
                MainThreadDispatcher::instance().post([this, cancelled, id, index, total = fragments.size()] {
                    if (!cancelled->load() && id == active_)
                        refreshCollection((language_ == "en-US" ? L"Recognizing " : L"正在识别 ") +
                                          std::to_wstring(index) + L" / " + std::to_wstring(total));
                });
            }
            if (text.empty()) throw std::runtime_error("未识别到文字，片段已保留");
            if (!copy) host_->openTextResult(text, cancelled.get());
            MainThreadDispatcher::instance().post([this, cancelled, id, copy, text = std::move(text)] {
                if (cancelled->load() || id != active_) return;
                try {
                    if (copy && !tools3000::core::WinUtils::copyToClipboard(wide(text)))
                        throw std::runtime_error("无法复制识别文字，片段已保留");
                    textBusy_ = false;
                    clearCollection();
                    retireDraft(true);
                    active_.clear();
                    complete(id, {{"completed", true}, {"copied", copy}});
                    upstream::CaptureOverlay::instance().cancel();
                } catch (const std::exception &e) {
                    textBusy_ = false;
                    refreshCollection(wide(e.what()));
                }
            });
        } catch (const std::exception &e) {
            const auto message = wide(e.what());
            MainThreadDispatcher::instance().post([this, cancelled, id, message] {
                if (!cancelled->load() && id == active_) {
                    textBusy_ = false;
                    refreshCollection(message);
                }
            });
        }
    });
}

void CaptureApp::clearCollection() {
    collecting_ = false;
    collectionSuspended_ = false;
    collectionDragging_ = -1;
    upstream::CaptureOverlay::instance().winoneState().winoneCollecting = false;
    collection_.clear();
    collectionWindow_.reset();
}

bool CaptureApp::collectionInput(HWND hwnd, UINT message, WPARAM wp, LPARAM lp) {
    if (!collecting_ || collectionSuspended_) return false;
    auto queue = [this](auto action) {
        MainThreadDispatcher::instance().postDeferred([this, action] {
            try {
                collectionAction(action, -1);
            } catch (const std::exception &e) {
                refreshCollection(wide(e.what()));
            }
        });
    };
    if (message == WM_KEYDOWN) {
        if (wp == VK_ESCAPE) {
            queue(CollectionWindow::Cancel);
            return true;
        }
        if (wp == VK_RETURN) {
            queue(CollectionWindow::Recognize);
            return true;
        }
        if (wp == VK_TAB) {
            queue(CollectionWindow::Switch);
            return true;
        }
        if (wp == 'Z' && (GetKeyState(VK_CONTROL) & 0x8000)) {
            queue(CollectionWindow::Undo);
            return true;
        }
    }
    if (textBusy_)
        return message == WM_LBUTTONDOWN || message == WM_LBUTTONUP || message == WM_RBUTTONUP ||
               message == WM_LBUTTONDBLCLK;
    const cv::Point point(GET_X_LPARAM(lp), GET_Y_LPARAM(lp));
    auto &overlay = upstream::CaptureOverlay::instance();
    auto &state = overlay.winoneState();
    if (message == WM_LBUTTONDOWN) {
        for (int i = static_cast<int>(collection_.items().size()) - 1; i >= 0; --i) {
            const auto &item = collection_.items()[i];
            if (item.screen == collectionScreen_ && item.rect.contains(point)) {
                SetCapture(hwnd);
                collectionDragging_ = i;
                collectionOriginalRect_ = collectionDragRect_ = item.rect;
                collectionDragStart_ = {point.x, point.y};
                return true;
            }
        }
    }
    if (collectionDragging_ >= 0) {
        if (message == WM_MOUSEMOVE) {
            collectionDragRect_.x =
                std::clamp(collectionOriginalRect_.x + point.x - static_cast<int>(collectionDragStart_.x), 0,
                           std::max(0, state.frozenScreen.cols - collectionOriginalRect_.width));
            collectionDragRect_.y =
                std::clamp(collectionOriginalRect_.y + point.y - static_cast<int>(collectionDragStart_.y), 0,
                           std::max(0, state.frozenScreen.rows - collectionOriginalRect_.height));
            overlay.winoneInvalidate();
            return true;
        }
        if (message == WM_LBUTTONUP) {
            if (collectionDragRect_ != collectionOriginalRect_)
                collection_.replace(collectionDragging_, state.frozenScreen(collectionDragRect_), collectionDragRect_);
            collectionDragging_ = -1;
            ReleaseCapture();
            refreshCollection();
            return true;
        }
        if (message == WM_CAPTURECHANGED && reinterpret_cast<HWND>(lp) != hwnd) {
            collectionDragging_ = -1;
            overlay.winoneInvalidate();
        }
    }
    return false;
}

void CaptureApp::drawCollection(ID2D1RenderTarget *target, IDWriteTextFormat *format, ID2D1Brush *text,
                                ID2D1Brush *background) {
    if (!collecting_ || collectionSuspended_ || !target || !format || !text || !background) return;
    // Reuse the canvas's fonts and theme brushes instead of allocating DWrite
    // factories and GDI resources on each animation/selection frame.
    const auto alignment = format->GetTextAlignment();
    const auto paragraph = format->GetParagraphAlignment();
    const auto scale = upstream::CaptureOverlay::instance().winoneState().dpiScale;
    format->SetTextAlignment(DWRITE_TEXT_ALIGNMENT_CENTER);
    format->SetParagraphAlignment(DWRITE_PARAGRAPH_ALIGNMENT_CENTER);
    for (size_t i = 0; i < collection_.items().size(); ++i) {
        const auto &fragment = collection_.items()[i];
        if (fragment.screen != collectionScreen_) continue;
        auto box = collectionDragging_ == static_cast<int>(i) ? collectionDragRect_ : fragment.rect;
        const auto rect = D2D1::RectF(static_cast<float>(box.x), static_cast<float>(box.y),
                                      static_cast<float>(box.br().x), static_cast<float>(box.br().y));
        target->DrawRectangle(rect, text, 2);
        const float y = std::max(0.f, rect.top - 26 * scale);
        auto badge =
            D2D1::RoundedRect(D2D1::RectF(rect.left, y, rect.left + 34 * scale, y + 24 * scale), 5 * scale, 5 * scale);
        target->FillRoundedRectangle(badge, background);
        auto label = std::to_wstring(i + 1);
        target->DrawTextW(label.c_str(), static_cast<UINT32>(label.size()), format, badge.rect, text);
    }
    format->SetTextAlignment(alignment);
    format->SetParagraphAlignment(paragraph);
}
} // namespace winone::capture
