#include "ocr_client.h"
#include <opencv2/imgcodecs.hpp>
#include <future>

namespace winone::capture {
namespace {
std::wstring quote(std::wstring_view value) {
    std::wstring result = L"\"";
    size_t slashes = 0;
    for (auto character : value) {
        if (character == L'\\') { ++slashes; continue; }
        result.append(character == L'\"' ? slashes * 2 + 1 : slashes, L'\\');
        slashes = 0;
        result.push_back(character);
    }
    result.append(slashes * 2, L'\\');
    return result + L'\"';
}

struct RedirectedPipe {
    Handle parent;
    Handle child;
    explicit RedirectedPipe(bool input) {
        SECURITY_ATTRIBUTES attributes{sizeof(SECURITY_ATTRIBUTES), nullptr, TRUE};
        HANDLE read = nullptr, write = nullptr;
        if (!CreatePipe(&read, &write, &attributes, 65536)) throw systemError("Cannot create OCR pipe");
        parent.reset(input ? write : read);
        child.reset(input ? read : write);
        if (!SetHandleInformation(parent.get(), HANDLE_FLAG_INHERIT, 0)) throw systemError("Cannot isolate OCR pipe");
    }
};
}

json OcrClient::recognize(const cv::Mat& image, bool pinLayout, const std::atomic<bool>* cancelled) const {
    // Production shares Host's warm engine. Standalone private probes retain
    // their explicit isolated worker; a Host error is never silently retried cold.
    if (host_ && pinLayout) return host_->recognizeLayout(image, cancelled);
    std::vector<uchar> png;
    if (image.empty() || static_cast<uint64_t>(image.cols) * image.rows > 40'000'000 ||
        !cv::imencode(".png", image, png) || png.size() > 32 * 1024 * 1024)
        throw std::runtime_error("Invalid OCR image");
    const auto executable = directory_ / L"WinOne.Ocr.exe";
    const auto runtime = directory_ / L"Ocr";
    auto command = quote(executable.wstring()) + L" --ocr-worker " + quote(runtime.wstring()) + (pinLayout ? L" --pin-layout" : L"");
    RedirectedPipe input(true), output(false), errors(false);
    STARTUPINFOEXW startup{};
    startup.StartupInfo.cb = sizeof(startup);
    startup.StartupInfo.dwFlags = STARTF_USESTDHANDLES;
    startup.StartupInfo.hStdInput = input.child.get();
    startup.StartupInfo.hStdOutput = output.child.get();
    startup.StartupInfo.hStdError = errors.child.get();
    SIZE_T bytes = 0;
    InitializeProcThreadAttributeList(nullptr, 1, 0, &bytes);
    std::vector<uint8_t> storage(bytes);
    startup.lpAttributeList = reinterpret_cast<LPPROC_THREAD_ATTRIBUTE_LIST>(storage.data());
    if (!InitializeProcThreadAttributeList(startup.lpAttributeList, 1, 0, &bytes)) throw systemError("Cannot initialize OCR process attributes");
    HANDLE inherited[]{input.child.get(), output.child.get(), errors.child.get()};
    if (!UpdateProcThreadAttribute(startup.lpAttributeList, 0, PROC_THREAD_ATTRIBUTE_HANDLE_LIST,
                                  inherited, sizeof(inherited), nullptr, nullptr)) {
        DeleteProcThreadAttributeList(startup.lpAttributeList);
        throw systemError("Cannot restrict OCR inherited handles");
    }
    Handle job(CreateJobObjectW(nullptr, nullptr));
    if (!job) { DeleteProcThreadAttributeList(startup.lpAttributeList); throw systemError("Cannot own OCR process"); }
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits{};
    limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
    if (!SetInformationJobObject(job.get(), JobObjectExtendedLimitInformation, &limits, sizeof(limits))) {
        DeleteProcThreadAttributeList(startup.lpAttributeList);
        throw systemError("Cannot configure OCR process ownership");
    }
    PROCESS_INFORMATION information{};
    const BOOL created = CreateProcessW(executable.c_str(), command.data(), nullptr, nullptr, TRUE,
        CREATE_NO_WINDOW | CREATE_SUSPENDED | EXTENDED_STARTUPINFO_PRESENT, nullptr, runtime.c_str(), &startup.StartupInfo, &information);
    const auto creationError = GetLastError();
    DeleteProcThreadAttributeList(startup.lpAttributeList);
    if (!created) throw systemError("Cannot start native OCR", creationError);
    Handle process(information.hProcess), thread(information.hThread);
    if (!AssignProcessToJobObject(job.get(), process.get())) {
        const auto error = GetLastError(); TerminateProcess(process.get(), 1); WaitForSingleObject(process.get(), INFINITE);
        throw systemError("Cannot assign native OCR ownership", error);
    }
    input.child.reset(); output.child.reset(); errors.child.reset();
    if (ResumeThread(thread.get()) == static_cast<DWORD>(-1)) {
        job.reset(); WaitForSingleObject(process.get(), INFINITE); throw systemError("Cannot resume native OCR");
    }
    thread.reset();
    auto writer = std::async(std::launch::async, [&] {
        size_t offset = 0;
        while (offset < png.size()) {
            DWORD sent = 0;
            if (!WriteFile(input.parent.get(), png.data() + offset, static_cast<DWORD>(png.size() - offset), &sent, nullptr) || sent == 0)
                throw systemError("Cannot send OCR image");
            offset += sent;
        }
        input.parent.reset();
    });
    std::string reply;
    auto drain = [&](HANDLE pipe, bool keep) {
        for (;;) {
            DWORD available = 0;
            if (!PeekNamedPipe(pipe, nullptr, 0, nullptr, &available, nullptr)) {
                if (GetLastError() == ERROR_BROKEN_PIPE) return;
                throw systemError("Cannot inspect OCR reply");
            }
            if (available == 0) return;
            char buffer[16384];
            DWORD count = 0;
            if (!ReadFile(pipe, buffer, std::min<DWORD>(available, sizeof(buffer)), &count, nullptr)) throw systemError("Cannot read OCR reply");
            if (keep) {
                if (reply.size() + count > 64 * 1024 * 1024) throw std::runtime_error("OCR layout response is too large");
                reply.append(buffer, count);
            }
        }
    };
    try {
        const auto deadline = GetTickCount64() + 60000;
        for (;;) {
            drain(output.parent.get(), true); drain(errors.parent.get(), false);
            const auto state = WaitForSingleObject(process.get(), 10);
            if (state == WAIT_OBJECT_0) break;
            if (state == WAIT_FAILED) throw systemError("Cannot wait for native OCR");
            if (cancelled && cancelled->load()) throw std::runtime_error("OCR cancelled");
            if (GetTickCount64() >= deadline) throw std::runtime_error("OCR timed out");
        }
        writer.get();
        drain(output.parent.get(), true); drain(errors.parent.get(), false);
        DWORD code = 0;
        if (!GetExitCodeProcess(process.get(), &code) || code != 0) throw std::runtime_error("Native OCR failed");
        const auto result = json::parse(reply);
        if (result.contains("error")) throw std::runtime_error("Native OCR rejected the image");
        return result;
    } catch (...) {
        job.reset(); WaitForSingleObject(process.get(), INFINITE);
        if (writer.valid()) { try { writer.get(); } catch (...) {} }
        throw;
    }
}

std::vector<tools3000::ocr::OcrResult> OcrClient::layout(const cv::Mat& image, const std::atomic<bool>* cancelled) const {
    const auto response = recognize(image, true, cancelled);
    std::vector<tools3000::ocr::OcrResult> lines;
    for (const auto& line : response.at("layout").at("lines")) {
        tools3000::ocr::OcrResult item;
        item.text = line.at("text").get<std::string>();
        item.confidence = 1; // The pinned worker already filters at confidence >= .5.
        const auto& box = line.at("box");
        item.box = {box[0].get<int>(), box[1].get<int>(), box[2].get<int>(), box[3].get<int>()};
        for (const auto& glyph : line.at("glyphs")) {
            const auto& bounds = glyph.at("box");
            item.charBoxes.emplace_back(bounds[0].get<int>(), bounds[1].get<int>(), bounds[2].get<int>(), bounds[3].get<int>());
        }
        lines.push_back(std::move(item));
    }
    return lines;
}
}
