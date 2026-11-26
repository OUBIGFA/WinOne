#pragma once
#include "capture/CaptureState.h"
#include "capture/FloatingGlassBar.h"
#include "core/i18n/UiI18n.h"
#include "toolbar_icons.h"
#include <array>
#include <algorithm>
#include <cmath>

namespace winone::capture {

// Keep the community menu items and hit testing. Only the customization menu
// uses this roomier single-column layout; the footer never scrolls offscreen.
inline void drawToolbarCustomization(tools3000::capture::CaptureState& state,
    ID2D1RenderTarget* target, IDWriteTextFormat* format, float labelWidth) {
    using namespace tools3000::capture;
    const float scale = std::clamp(state.dpiScale, 1.f, 5.f);
    const auto surface = target->GetSize();
    const float edge = 8 * scale, pad = 12 * scale, pitch = 40 * scale, rowHeight = 38 * scale;
    const float header = 42 * scale, footer = 54 * scale;
    const int count = static_cast<int>(state.dropdownMenu.items.size()) - 1;
    const float width = std::min(std::max(256 * scale, labelWidth + 112 * scale), surface.width - edge * 2);
    const auto anchor = state.dropdownMenu.anchorButtonRect;
    float above = anchor.top, below = anchor.bottom;
    for (const auto bar : {state.primaryToolbarRect, state.secondaryToolbarRect}) {
        if (bar.right <= bar.left || bar.bottom <= bar.top) continue;
        above = std::min(above, bar.top); below = std::max(below, bar.bottom);
    }
    const float upperSpace = above - edge * 2, lowerSpace = surface.height - below - edge * 2;
    const bool openAbove = upperSpace >= lowerSpace;
    const float available = std::max(upperSpace, lowerSpace);
    const int rows = std::clamp(static_cast<int>((available - header - footer) / pitch), 1, count);
    state.winoneToolbarVisibleRows = rows;
    state.winoneToolbarFirstRow = std::clamp(state.winoneToolbarFirstRow, 0, count - rows);
    const float height = header + rows * pitch + footer;
    const float x = std::clamp((anchor.left + anchor.right - width) / 2, edge, std::max(edge, surface.width - width - edge));
    float y = openAbove ? above - height - edge : below + edge;
    y = std::clamp(y, edge, std::max(edge, surface.height - height - edge));
    const auto panel = D2D1::RectF(x, y, x + width, y + height);
    state.dropdownMenu.menuRect = panel;
    state.dropdownMenu.childMenuRect = {};

    const auto palette = FloatingGlassBar::resolveThemePalette();
    Microsoft::WRL::ComPtr<ID2D1SolidColorBrush> background, border, ink, muted, hover;
    target->CreateSolidColorBrush(palette.isDark ? D2D1::ColorF(0x242424) : D2D1::ColorF(0xffffff), background.GetAddressOf());
    target->CreateSolidColorBrush(palette.isDark ? D2D1::ColorF(0x4b4b4b) : D2D1::ColorF(0xd4d4d4), border.GetAddressOf());
    target->CreateSolidColorBrush(palette.primaryText, ink.GetAddressOf());
    target->CreateSolidColorBrush(palette.secondaryText, muted.GetAddressOf());
    target->CreateSolidColorBrush(palette.isDark ? D2D1::ColorF(0x353535) : D2D1::ColorF(0xededed), hover.GetAddressOf());
    if (!background || !border || !ink || !muted || !hover || !format) return;
    const auto rounded = D2D1::RoundedRect(panel, 9 * scale, 9 * scale);
    target->FillRoundedRectangle(rounded, background.Get());
    target->DrawRoundedRectangle(rounded, border.Get(), scale);
    const auto alignment = format->GetTextAlignment();
    format->SetTextAlignment(DWRITE_TEXT_ALIGNMENT_LEADING);
    const auto title = tools3000::core::isUiLanguageChinese() ? L"自定义工具栏" : L"Customize toolbar";
    target->DrawTextW(title, static_cast<UINT32>(wcslen(title)), format,
        D2D1::RectF(x + pad + 8 * scale, y + 4 * scale, x + width - pad, y + header), muted.Get());

    static constexpr std::array<const char*, 13> icons{
        "region", "select", "rectangle", "ellipse", "arrow", "pen", "cover", "text", "step", "mosaic", "blur", "ocr", "translate"};
    for (int index = 0; index <= count; ++index) {
        auto& item = state.dropdownMenu.items[index];
        item.isHovered = false;
        if (index < count && (index < state.winoneToolbarFirstRow || index >= state.winoneToolbarFirstRow + rows)) {
            // Offscreen entries must not retain their previous hit rectangles.
            item.rect = D2D1::RectF(-1, -1, -1, -1);
            continue;
        }
        const bool reset = index == count;
        const float top = reset ? y + header + rows * pitch + 10 * scale
            : y + header + (index - state.winoneToolbarFirstRow) * pitch;
        item.rect = D2D1::RectF(x + pad, top, x + width - pad, top + rowHeight);
        item.isHovered = index == state.dropdownMenu.hoveredIndex;
        if (item.isHovered) target->FillRoundedRectangle(D2D1::RoundedRect(item.rect, 5 * scale, 5 * scale), hover.Get());
        if (reset) {
            const float lineY = top - 6 * scale;
            target->DrawLine(D2D1::Point2F(x + pad, lineY), D2D1::Point2F(x + width - pad, lineY), border.Get(), scale);
        }
        const float center = top + rowHeight / 2;
        const auto iconRect = D2D1::RectF(item.rect.left + 8 * scale, center - 9 * scale, item.rect.left + 26 * scale, center + 9 * scale);
        const char* icon = reset ? "undo" : icons.at(static_cast<size_t>(item.id));
        winone::icons::render(target, icon, palette.isDark, iconRect, .85f);
        target->DrawTextW(item.label.c_str(), static_cast<UINT32>(item.label.size()), format,
            D2D1::RectF(item.rect.left + 40 * scale, top, item.rect.right - (reset ? 8 : 40) * scale, top + rowHeight), ink.Get());
        if (!reset) {
            const auto box = D2D1::RoundedRect(D2D1::RectF(item.rect.right - 28 * scale, center - 8 * scale,
                item.rect.right - 12 * scale, center + 8 * scale), 3 * scale, 3 * scale);
            if (item.isSelected) target->FillRoundedRectangle(box, ink.Get());
            else target->DrawRoundedRectangle(box, muted.Get(), scale);
            if (item.isSelected) {
                const float cx = item.rect.right - 20 * scale;
                target->DrawLine(D2D1::Point2F(cx - 4 * scale, center), D2D1::Point2F(cx - scale, center + 3 * scale), background.Get(), 1.6f * scale);
                target->DrawLine(D2D1::Point2F(cx - scale, center + 3 * scale), D2D1::Point2F(cx + 4 * scale, center - 3 * scale), background.Get(), 1.6f * scale);
            }
        }
    }
    if (rows < count) {
        const float track = rows * pitch, thumb = track * rows / count;
        const float top = y + header + (track - thumb) * state.winoneToolbarFirstRow / (count - rows);
        target->FillRoundedRectangle(D2D1::RoundedRect(D2D1::RectF(x + width - 7 * scale, top,
            x + width - 4 * scale, top + thumb), 1.5f * scale, 1.5f * scale), muted.Get());
    }
    format->SetTextAlignment(alignment);
}
}
