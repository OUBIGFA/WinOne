#include <windows.h>
#include <objbase.h>
#include <oleauto.h>
#include <UIAutomation.h>
#include <wrl/client.h>
#include <cerrno>
#include <cstdio>
#include <cstdlib>
#include "CaptureQueryLifetime.h"

using Microsoft::WRL::ComPtr;

static bool succeeded(HRESULT result, const char* stage)
{
    if (SUCCEEDED(result)) return true;
    std::fprintf(stderr, "UIA %s failed: 0x%08lx\n", stage, static_cast<unsigned long>(result));
    return false;
}

// One request per process isolates unresponsive accessibility providers. Only
// geometry is exported; names, values and document text are never requested.
int wmain(int argc, wchar_t** argv)
{
    if (argc != 2 && argc != 3) return 2;
    wchar_t* end = nullptr;
    errno = 0;
    const auto value = wcstoull(argv[1], &end, 10);
    if (errno || end == argv[1] || *end || !value) return 2;
    const auto target = reinterpret_cast<HWND>(static_cast<UINT_PTR>(value));
    if (!IsWindow(target) || !IsWindowVisible(target)) return 3;
    DWORD parent = 0;
    if (argc == 3) {
        errno = 0;
        const auto parentValue = wcstoull(argv[2], &end, 10);
        if (errno || end == argv[2] || *end || parentValue == 0 || parentValue > MAXDWORD) return 2;
        parent = static_cast<DWORD>(parentValue);
    }
    CaptureQueryLifetime lifetime;
    if (!lifetime.Start(parent)) return 4;
    SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
    if (FAILED(CoInitializeEx(nullptr, COINIT_MULTITHREADED))) return 4;
    int status = [&]() -> int {
        ComPtr<IUIAutomation2> automation;
        if (FAILED(CoCreateInstance(CLSID_CUIAutomation8, nullptr, CLSCTX_INPROC_SERVER, IID_PPV_ARGS(&automation)))) return 4;
        if (FAILED(automation->put_ConnectionTimeout(200)) || FAILED(automation->put_TransactionTimeout(200))) return 4;
        ComPtr<IUIAutomationElement> root;
        if (!succeeded(automation->ElementFromHandle(target, &root), "window") || !root) return 5;
        ComPtr<IUIAutomationCacheRequest> cache;
        if (FAILED(automation->CreateCacheRequest(&cache))) return 4;
        if (FAILED(cache->AddProperty(UIA_BoundingRectanglePropertyId)) ||
            FAILED(cache->AddProperty(UIA_ControlTypePropertyId)) ||
            FAILED(cache->AddProperty(UIA_IsOffscreenPropertyId)) ||
            FAILED(cache->put_AutomationElementMode(AutomationElementMode_None))) return 4;
        ComPtr<IUIAutomationCondition> condition;
        if (FAILED(automation->get_ControlViewCondition(&condition))) return 4;
        ComPtr<IUIAutomationElementArray> elements;
        if (!succeeded(root->FindAllBuildCache(TreeScope_Descendants, condition.Get(), cache.Get(), &elements), "regions") || !elements) return 5;
        int count = 0;
        if (FAILED(elements->get_Length(&count)) || count > 8192) return 6;
        for (int index = 0; index < count; ++index) {
            ComPtr<IUIAutomationElement> element;
            RECT bounds{}; CONTROLTYPEID type = 0; BOOL offscreen = TRUE;
            if (FAILED(elements->GetElement(index, &element)) || !element ||
                FAILED(element->get_CachedBoundingRectangle(&bounds)) ||
                FAILED(element->get_CachedControlType(&type)) ||
                FAILED(element->get_CachedIsOffscreen(&offscreen))) return 5;
            // Text runs are not independently useful screenshot regions. Keep
            // their containing button, panel, document or other control instead.
            if (offscreen || type == UIA_TextControlTypeId ||
                static_cast<long long>(bounds.right) - bounds.left < 12 ||
                static_cast<long long>(bounds.bottom) - bounds.top < 12) continue;
            std::printf("%ld %ld %ld %ld\n", bounds.left, bounds.top, bounds.right, bounds.bottom);
        }
        return std::ferror(stdout) ? 7 : 0;
    }();
    CoUninitialize();
    return status;
}
