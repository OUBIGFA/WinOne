#include <windows.h>
#include <cstdio>
#include <cstdlib>
#include "../../modules/capture/native/CaptureQueryLifetime.h"

// Built only as a test executable, never included in the application bundle.
int wmain(int argc, wchar_t** argv) {
    if (argc < 3 || argc > 4) return 2;
    CaptureQueryLifetime lifetime;
    if (!lifetime.Start(wcstoul(argv[1], nullptr, 10), wcstoul(argv[2], nullptr, 10))) return 4;
    std::puts("ready"); std::fflush(stdout);
    if (argc == 4) return 0;
    Sleep(10000);
    return 9;
}
