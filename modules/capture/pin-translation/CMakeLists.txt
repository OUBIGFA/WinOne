cmake_minimum_required(VERSION 3.24)
project(WinOnePinTranslation LANGUAGES C CXX)
# The static OpenCV build is restricted to the algorithms the pinned renderer uses.
set(BUILD_LIST core,imgproc,photo,objdetect CACHE STRING "")
set(WITH_QUIRC ON CACHE BOOL "Use the pinned QR decoder" FORCE)
set(BUILD_WITH_STATIC_CRT ON CACHE BOOL "")
set(CMAKE_MSVC_RUNTIME_LIBRARY MultiThreaded)
foreach(option BUILD_SHARED_LIBS BUILD_TESTS BUILD_PERF_TESTS BUILD_EXAMPLES BUILD_opencv_apps BUILD_JAVA
        BUILD_DOCS WITH_IPP WITH_ITT WITH_OPENCL WITH_LAPACK WITH_EIGEN WITH_ADE WITH_PROTOBUF
        WITH_OPENEXR WITH_FFMPEG WITH_MSMF WITH_JPEG WITH_PNG WITH_TIFF WITH_WEBP WITH_JASPER WITH_OPENJPEG
        WITH_FLATBUFFERS)
    set(${option} OFF CACHE BOOL "")
endforeach()
set(CPU_BASELINE SSE2 CACHE STRING "")
set(CPU_DISPATCH "" CACHE STRING "")
add_subdirectory("${OPENCV_SOURCE}" opencv EXCLUDE_FROM_ALL)
add_library(WinOne.PinTranslation SHARED
    PinTranslation.cpp ocr/OcrEngine.cpp capture/InPlaceTranslateEngine.cpp capture/TextReplaceEngine.cpp)
target_compile_features(WinOne.PinTranslation PRIVATE cxx_std_20)
target_compile_definitions(WinOne.PinTranslation PRIVATE UNICODE _UNICODE NOMINMAX WIN32_LEAN_AND_MEAN)
target_compile_options(WinOne.PinTranslation PRIVATE /utf-8 /EHsc /Gy /Gw)
target_include_directories(WinOne.PinTranslation PRIVATE . "${CMAKE_BINARY_DIR}" "${CMAKE_BINARY_DIR}/opencv")
foreach(module core imgproc objdetect photo)
    target_include_directories(WinOne.PinTranslation PRIVATE "${OPENCV_SOURCE}/modules/${module}/include")
endforeach()
target_link_options(WinOne.PinTranslation PRIVATE /OPT:REF /OPT:ICF)
target_link_libraries(WinOne.PinTranslation PRIVATE opencv_core opencv_imgproc opencv_photo opencv_objdetect d2d1 dwrite windowscodecs ole32 uuid)
