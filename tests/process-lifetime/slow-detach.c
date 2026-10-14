#include <windows.h>

// A bounded stand-in for an injected input method's blocked DLL detach.
// It owns no files, system settings, hooks or shared named objects.
BOOL WINAPI DllMain(HINSTANCE module, DWORD reason, LPVOID reserved) {
    (void)module;
    (void)reserved;
    if (reason == DLL_PROCESS_DETACH) {
        const char message[] = "native-detach\n";
        DWORD written;
        WriteFile(GetStdHandle(STD_OUTPUT_HANDLE), message, sizeof(message)-1, &written, NULL);
        Sleep(2000);
    }
    return TRUE;
}
