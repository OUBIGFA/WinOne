set(VCPKG_TARGET_ARCHITECTURE x64)
set(VCPKG_CRT_LINKAGE dynamic)
set(VCPKG_LIBRARY_LINKAGE dynamic)
set(VCPKG_BUILD_TYPE release)
# Upstream x264 runs AS as a shell command. Preserve the task's space-free
# download alias rather than the expanded checkout path selected by vcpkg.
file(GLOB WINONE_NASM_TOOLS "$ENV{VCPKG_DOWNLOADS}/tools/nasm/*/nasm.exe")
if(WINONE_NASM_TOOLS)
    list(GET WINONE_NASM_TOOLS 0 WINONE_NASM)
    set(ENV{AS} "${WINONE_NASM}")
endif()
