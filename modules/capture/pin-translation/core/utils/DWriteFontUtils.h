#pragma once
#include <windows.h>
#include <dwrite.h>
#include <wrl/client.h>
#include <iterator>
namespace tools3000::core {
inline const wchar_t* resolveInstalledUiFontFamily(IDWriteFactory* dwriteFactory) noexcept {
    const auto lang = PRIMARYLANGID(GetUserDefaultUILanguage());
    auto findFirstInstalled = [&](const wchar_t* const* candidates, std::size_t count, const wchar_t* fallback) noexcept {
        if (dwriteFactory) {
            Microsoft::WRL::ComPtr<IDWriteFontCollection> fontCollection;
            if (SUCCEEDED(dwriteFactory->GetSystemFontCollection(fontCollection.GetAddressOf(), FALSE)) && fontCollection) {
                for (std::size_t i = 0; i < count; ++i) {
                    UINT32 index = 0;
                    BOOL exists = FALSE;
                    if (SUCCEEDED(fontCollection->FindFamilyName(candidates[i], &index, &exists)) && exists) {
                        return candidates[i];
                    }
                }
            }
        }
        return fallback;
    };

    switch (lang) {
        case LANG_CHINESE: {
            if (SUBLANGID(GetUserDefaultUILanguage()) != SUBLANG_CHINESE_TRADITIONAL) break;
            static constexpr const wchar_t* kTw[] = {
                L"Microsoft JhengHei UI", L"Noto Sans TC", L"Segoe UI Variable Text", L"Segoe UI", L"Microsoft YaHei UI"
            };
            return findFirstInstalled(kTw, std::size(kTw), L"Microsoft JhengHei UI");
        }
        case LANG_JAPANESE: {
            static constexpr const wchar_t* kJa[] = {
                L"Yu Gothic UI", L"Meiryo UI", L"Noto Sans JP", L"Segoe UI Variable Text", L"Segoe UI"
            };
            return findFirstInstalled(kJa, std::size(kJa), L"Yu Gothic UI");
        }
        case LANG_KOREAN: {
            static constexpr const wchar_t* kKo[] = {
                L"Malgun Gothic", L"Noto Sans KR", L"Segoe UI Variable Text", L"Segoe UI"
            };
            return findFirstInstalled(kKo, std::size(kKo), L"Malgun Gothic");
        }
        case LANG_ENGLISH:
        case LANG_RUSSIAN:
        case LANG_PORTUGUESE:
        case LANG_SPANISH: {
            static constexpr const wchar_t* kLatinCyr[] = {
                L"Segoe UI Variable Text", L"Segoe UI", L"Noto Sans SC", L"Microsoft YaHei UI"
            };
            return findFirstInstalled(kLatinCyr, std::size(kLatinCyr), L"Segoe UI");
        }

        default: break;
    }
    static constexpr const wchar_t* kZh[] = {
        L"Noto Sans SC", L"Source Han Sans SC", L"Microsoft YaHei UI", L"Segoe UI Variable Text", L"Segoe UI"
    };
    return findFirstInstalled(kZh, std::size(kZh), L"Microsoft YaHei UI");
}

}
