// WinOne adapter for the MIT-licensed PowerToys AlwaysOnTop core.
#include "pch.h"
#include "AlwaysOnTop.h"
#include <common/utils/game_mode.h>

bool isExcluded(HWND window);

HRESULT AlwaysOnTop::SetOwnedPin(HWND window, bool enabled)
{
    if (!IsWindow(window)) return HRESULT_FROM_WIN32(ERROR_INVALID_WINDOW_HANDLE);
    if (enabled)
    {
        if (isExcluded(window)) return HRESULT_FROM_WIN32(ERROR_ACCESS_DISABLED_BY_POLICY);
        if (AlwaysOnTopSettings::settings()->blockInGameMode && detect_game_mode()) return HRESULT_FROM_WIN32(ERROR_BUSY);
        if (IsTopmost(window) && !IsPinned(window)) return HRESULT_FROM_WIN32(ERROR_ALREADY_EXISTS);
        if (IsPinned(window) && IsTopmost(window)) return S_FALSE;
        if (!PinTopmostWindow(window)) return HRESULT_FROM_WIN32(GetLastError() ? GetLastError() : ERROR_GEN_FAILURE);
        if (!AssignBorder(window))
        {
            if (UnpinTopmostWindow(window)) m_topmostWindows.erase(window);
            return E_FAIL;
        }
        if (AlwaysOnTopSettings::settings()->enableSound) m_sound.Play(Sound::Type::On);
    }
    else
    {
        if (!IsPinned(window)) return S_FALSE;
        if (!UnpinTopmostWindow(window)) return HRESULT_FROM_WIN32(GetLastError() ? GetLastError() : ERROR_GEN_FAILURE);
        m_topmostWindows.erase(window);
        if (AlwaysOnTopSettings::settings()->enableSound) m_sound.Play(Sound::Type::Off);
    }
    return S_OK;
}

HRESULT AlwaysOnTop::ClearOwnedPins()
{
    std::vector<HWND> windows;
    for (const auto& entry : m_topmostWindows) windows.push_back(entry.first);
    HRESULT result = S_OK;
    for (const auto window : windows)
    {
        if (!IsWindow(window) || !IsPinned(window)) { m_topmostWindows.erase(window); continue; }
        const auto current = SetOwnedPin(window, false);
        if (FAILED(current)) result = current;
    }
    return result;
}

void AlwaysOnTopSettings::ApplyWinOneSettings(const Settings& settings)
{
    m_settings.store(std::make_shared<const Settings>(settings), std::memory_order_release);
    for (const auto id : { SettingId::FrameEnabled, SettingId::FrameThickness, SettingId::FrameColor,
        SettingId::FrameOpacity, SettingId::FrameAccentColor, SettingId::RoundCornersEnabled,
        SettingId::BlockInGameMode, SettingId::ExcludeApps }) NotifyObservers(id);
}
