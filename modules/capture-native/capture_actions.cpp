#include "capture_app.h"
#include "capture/PinWindow.h"
#include "capture/ScrollCaptureOverlay.h"
#include "capture/InPlaceTranslateEngine.h"
#include "core/events/MainThreadDispatcher.h"
#include "core/utils/OutOfProcessFileDialog.h"
#include "core/utils/WinUtils.h"
#include <opencv2/imgcodecs.hpp>
#include <fstream>
#include <iomanip>
#include <sstream>

namespace winone::capture {
namespace upstream = tools3000::capture;
using tools3000::core::MainThreadDispatcher;
namespace {
void replaceAll(std::string& text, std::string_view needle, const std::string& value) {
    size_t offset = 0;
    while ((offset = text.find(needle, offset)) != std::string::npos) { text.replace(offset, needle.size(), value); offset += value.size(); }
}
std::string timePart(const SYSTEMTIME& at, bool date, bool time) {
    std::ostringstream result;
    if (date) result << std::setfill('0') << std::setw(4) << at.wYear << std::setw(2) << at.wMonth << std::setw(2) << at.wDay;
    if (time) result << std::setfill('0') << std::setw(2) << at.wHour << std::setw(2) << at.wMinute << std::setw(2) << at.wSecond;
    return result.str();
}
}

std::filesystem::path CaptureApp::suggestedPath(std::string_view format) const {
    const auto directory = std::filesystem::path(wide(captureSettings().at("outputDirectory").get<std::string>()));
    const std::string configured = captureSettings().contains("fileNameTemplate") && captureSettings()["fileNameTemplate"].is_string()
        ? captureSettings()["fileNameTemplate"].get<std::string>() : "";
    const bool useDefault = configured.empty();
    const auto pattern = useDefault ? std::string("WinOne-{timestamp}-OK") : configured;
    if (pattern.size() > 160) throw std::runtime_error("文件名模板过长");
    SYSTEMTIME at{}; GetLocalTime(&at);
    for (unsigned sequence = 1; sequence <= 10000; ++sequence) {
        auto name = pattern;
        replaceAll(name, "{timestamp}", timePart(at, true, true)); replaceAll(name, "{date}", timePart(at, true, false));
        replaceAll(name, "{time}", timePart(at, false, true));
        std::ostringstream number; number << std::setfill('0') << std::setw(2) << sequence;
        replaceAll(name, "{sequence}", number.str());
        if (name.size() >= 4 && _stricmp(name.substr(name.size() - 4).c_str(), ".png") == 0) name.resize(name.size() - 4);
        if (name.empty() || name.back() == '.' || name.back() == ' ' ||
            name.find_first_of("{}<>:\"/\\|?*") != std::string::npos || std::any_of(name.begin(), name.end(), [](unsigned char c) { return c < 32; }))
            throw std::runtime_error("文件名模板无效");
        auto base = wide(name.substr(0, name.find('.')));
        while (!base.empty() && base.back() == ' ') base.pop_back();
        for (auto& c : base) c = towupper(c);
        if (base == L"CON" || base == L"PRN" || base == L"AUX" || base == L"NUL" ||
            (base.size() == 4 && (base.starts_with(L"COM") || base.starts_with(L"LPT")) &&
             ((base[3] >= L'1' && base[3] <= L'9') || base[3] == L'¹' || base[3] == L'²' || base[3] == L'³')))
            throw std::runtime_error("文件名使用了系统保留名称");
        if (sequence > 1 && pattern.find("{sequence}") == std::string::npos) {
            std::ostringstream suffix; suffix << '-' << std::setfill('0') << std::setw(2) << (useDefault ? sequence - 1 : sequence);
            name += suffix.str();
        }
        if (format == "mp4" || format == "gif") {
            const std::string marker = format == "mp4" ? "Video" : "GIF";
            if (name.size() >= 3 && _stricmp(name.substr(name.size() - 3).c_str(), "-OK") == 0) name.insert(name.size() - 3, "-" + marker);
            else name += "-" + marker;
        }
        if (name.size() > 240) throw std::runtime_error("生成的文件名过长");
        auto result = directory / wide(name + "." + std::string(format));
        if (!std::filesystem::exists(result)) return result;
    }
    throw std::runtime_error("无法生成未被使用的文件名");
}

std::filesystem::path CaptureApp::chooseSavePath(const std::filesystem::path& suggested, HWND owner) {
    tools3000::core::FileDialogRequest request;
    request.mode = tools3000::core::FileDialogMode::SaveFile;
    request.ownerHwnd = owner ? owner : upstream::CaptureOverlay::instance().winoneWindow();
    request.title = language_ == "en-US" ? L"Save capture" : L"保存截图";
    request.defaultExtension = suggested.extension().wstring().substr(1);
    request.defaultFileName = suggested.filename().wstring();
    request.initialDirectory = suggested.parent_path();
    request.filters = {{L"PNG", L"*.png"}, {L"JPEG", L"*.jpg;*.jpeg"}, {L"BMP", L"*.bmp"}, {L"WebP", L"*.webp"}};
    const auto result = tools3000::core::OutOfProcessFileDialog::show(request);
    if (result.cancelled) return {};
    if (!result.accepted || result.selectedPath.empty()) throw std::runtime_error("无法打开保存对话框");
    return result.selectedPath;
}

bool CaptureApp::savePin(HWND owner, const cv::Mat& image) {
    try {
        const auto path=chooseSavePath(suggestedPath(),owner);
        if(path.empty()) return false;
        saveImage(image,path,true); return true;
    } catch(const std::exception& error) {
        MessageBoxW(owner,wide(error.what()).c_str(),L"WinOne",MB_OK|MB_ICONWARNING);
        return false;
    }
}

std::filesystem::path CaptureApp::saveImage(const cv::Mat& image, std::filesystem::path destination, bool replace) {
    if (!destination.is_absolute() || destination.filename().empty()) throw std::runtime_error("保存路径无效");
    auto extension = utf8(destination.extension().wstring());
    std::transform(extension.begin(), extension.end(), extension.begin(), [](unsigned char c) { return static_cast<char>(tolower(c)); });
    if (extension != ".png" && extension != ".jpg" && extension != ".jpeg" && extension != ".bmp" && extension != ".webp")
        throw std::runtime_error("不支持的图片格式");
    std::vector<uchar> encoded;
    if (!cv::imencode(extension, image, encoded)) throw std::runtime_error("图片编码失败");
    std::filesystem::create_directories(destination.parent_path());
    auto temporary = destination; temporary += L"." + wide(identity()) + L".tmp";
    Handle file(CreateFileW(temporary.c_str(), GENERIC_WRITE, 0, nullptr, CREATE_NEW,
                            FILE_ATTRIBUTE_TEMPORARY | FILE_FLAG_WRITE_THROUGH, nullptr));
    if (!file) throw systemError("无法创建图片文件");
    try {
        size_t offset = 0;
        while (offset < encoded.size()) {
            DWORD written = 0;
            if (!WriteFile(file.get(), encoded.data() + offset, static_cast<DWORD>(std::min<size_t>(encoded.size() - offset, 1 << 20)), &written, nullptr) || written == 0)
                throw systemError("图片写入失败");
            offset += written;
        }
        if (!FlushFileBuffers(file.get())) throw systemError("图片写入未完成");
        file.reset();
        if (!MoveFileExW(temporary.c_str(), destination.c_str(), MOVEFILE_WRITE_THROUGH | (replace ? MOVEFILE_REPLACE_EXISTING : 0)))
            throw systemError("无法发布图片文件，原截图已保留");
    } catch (...) { file.reset(); DeleteFileW(temporary.c_str()); throw; }
    return destination;
}

json CaptureApp::publish(const cv::Mat& image, const upstream::CaptureRegion& region, const upstream::CaptureCompletion& completion) {
    using Action = upstream::CaptureCompletionAction;
    bool copy = captureSettings().value("copyToClipboard", true);
    bool pin = captureSettings().value("pinAfterCapture", false);
    bool save = !captureSettings().value("openEditor", true);
    bool ask = captureSettings().value("askWhereToSave", false);
    if (completion.action == Action::Copy) { copy = true; pin = false; save = false; ask = false; }
    if (completion.action == Action::Pin) { copy = false; pin = true; save = false; ask = false; }
    if (completion.action == Action::QuickSave) { save = true; ask = ask && captureSettings().value("openEditor", true); }
    if (completion.action == Action::SaveAs) { copy = false; pin = false; save = true; ask = true; }
    if (completion.action == Action::Default && captureSettings().value("openEditor", true)) {
        // WinOne's editor confirm action copies; file saving is a separate
        // explicit action. Quick-save mode retains its configured combination.
        copy = true; pin = false; save = false; ask = false;
    }
    std::filesystem::path path;
    if (save) {
        path = completion.filePath.empty() ? suggestedPath() : std::filesystem::path(wide(completion.filePath));
        if (ask && completion.filePath.empty()) path = chooseSavePath(path);
        if (path.empty()) return nullptr;
        saveImage(image, path, ask || !completion.filePath.empty());
    }
    if (copy && !upstream::ScreenCapture::instance().copyToClipboard(image, path.wstring(), &region))
        throw std::runtime_error("无法复制图片，截图已保留");
    if (pin) {
        auto& state = upstream::CaptureOverlay::instance().winoneState();
        cv::Mat clean;
        std::vector<upstream::MarkupElement> marks;
        const cv::Rect selected(std::min(state.dragStart.x,state.dragEnd.x),std::min(state.dragStart.y,state.dragEnd.y),image.cols,image.rows);
        if (!state.winoneShowTranslated && !state.frozenScreen.empty() &&
            (selected & cv::Rect(0,0,state.frozenScreen.cols,state.frozenScreen.rows)) == selected) {
            clean = state.frozenScreen(selected).clone(); marks = state.markup.exportElements();
        }
        if (!upstream::PinWindow::create(image, region.x, region.y, 0, {}, clean, std::move(marks), false))
            throw std::runtime_error("无法创建贴图，截图已保留");
    }
    lastImage_ = image.clone();
    last_ = {{"path", utf8(path.wstring())}, {"width", image.cols}, {"height", image.rows},
             {"copied", copy}, {"pinned", pin}, {"id", identity()}, {"previewReady", false}};
    return last_;
}

void CaptureApp::textAction(const cv::Mat& source, bool translate) {
    if (!host_) { notice("WinOne 后台不可用"); return; }
    if (textBusy_) {
        operationCancelled_->store(true); operationCancelled_ = std::make_shared<std::atomic<bool>>(false);
        textBusy_ = false;
        upstream::CaptureOverlay::instance().winoneState().winoneProgress = -1;
        notice(language_ == "en-US" ? "Cancelled" : "已取消"); return;
    }
    snapshotDraft(true);
    auto image = source.clone();
    auto cancelled = operationCancelled_;
    const auto id = active_;
    const bool inplace = translate && captureSettings().value("pinTranslation", false);
    const bool copyText = !translate && mode_ == "ocr";
    auto& overlay = upstream::CaptureOverlay::instance();
    if (inplace && !overlay.winoneState().winoneTranslated.empty()) {
        overlay.winoneState().winoneShowTranslated = !overlay.winoneState().winoneShowTranslated;
        overlay.winoneInvalidate(); return;
    }
    textBusy_ = true;
    if (inplace) { overlay.winoneState().winoneProgress = .05f; overlay.winoneState().winoneProgressText = language_ == "en-US" ? L"Recognizing text" : L"识别文字"; }
    notice(language_ == "en-US" ? "Recognizing text…" : "正在识别文字…");
    background([this, image = std::move(image), cancelled, id, inplace, translate, copyText] {
        try {
            if (inplace) {
                const auto lines = ocr_->layout(image, cancelled.get());
                std::vector<upstream::PinOcrBlock> blocks;
                for (const auto& line : lines) blocks.push_back({line.text, line.box, line.charBoxes, line.layoutRegionId, line.alignedRowId});
                auto paragraphs = upstream::InPlaceTranslateEngine::buildTranslationParagraphs(blocks, image.size(), {}, image);
                std::string sample;
                for (const auto& paragraph : paragraphs) sample += paragraph.originalText + '\n';
                const auto target = host_->target(sample);
                size_t done = 0;
                for (auto& paragraph : paragraphs) {
                    MainThreadDispatcher::instance().post([this, cancelled, id, done, total = paragraphs.size()] {
                        if (cancelled->load() || id != active_) return;
                        auto& view = upstream::CaptureOverlay::instance();
                        view.winoneState().winoneProgress = .2f + .7f * static_cast<float>(done) / static_cast<float>(std::max<size_t>(1, total));
                        view.winoneState().winoneProgressText = (language_ == "en-US" ? L"Translating " : L"逐段翻译 ") + std::to_wstring(done) + L"/" + std::to_wstring(total);
                        view.winoneInvalidate();
                    });
                    paragraph.translatedText = host_->translate(paragraph.originalText, target, cancelled.get()); ++done;
                }
                MainThreadDispatcher::instance().post([this, cancelled, id] {
                    if (cancelled->load() || id != active_) return;
                    auto& view = upstream::CaptureOverlay::instance(); view.winoneState().winoneProgress = .9f;
                    view.winoneState().winoneProgressText = language_ == "en-US" ? L"Rendering translation" : L"生成译文图像"; view.winoneInvalidate();
                });
                auto result = upstream::InPlaceTranslateEngine::renderTranslatedCanvas(image, std::move(paragraphs), target);
                if (!result.success) throw std::runtime_error("无法生成译文图片");
                MainThreadDispatcher::instance().post([this, result = std::move(result), cancelled, id]() mutable {
                    if (cancelled->load() || id != active_) return;
                    auto& current = upstream::CaptureOverlay::instance();
                    textBusy_ = false;
                    current.winoneState().winoneProgress = -1;
                    current.winoneState().winoneTranslated = std::move(result.translatedCanvas);
                    current.winoneState().winoneShowTranslated = true; current.winoneInvalidate();
                });
                return;
            }
            std::string text;
            if (copyText) text = host_->recognize(image, cancelled.get());
            else host_->openResult(image, translate, cancelled.get());
            MainThreadDispatcher::instance().post([this, cancelled, id, text = std::move(text), copyText] {
                if (cancelled->load() || id != active_) return;
                textBusy_ = false;
                if (copyText && !tools3000::core::WinUtils::copyToClipboard(wide(text))) { notice("无法复制识别文字"); return; }
                active_.clear();
                complete(id, {{"completed", true}, {"copied", copyText}});
                upstream::CaptureOverlay::instance().cancel();
            });
        } catch (const std::exception& error) {
            const std::string message = error.what();
            MainThreadDispatcher::instance().post([this, cancelled, id, message] { if (!cancelled->load() && id == active_) { textBusy_ = false; upstream::CaptureOverlay::instance().winoneState().winoneProgress = -1; notice(message); } });
        }
    });
}

void CaptureApp::startScrolling(const upstream::CaptureRegion& region) {
    snapshotDraft(true);
    const bool originalLive = !retaking_ && upstream::CaptureOverlay::instance().winoneState().winoneLiveCapture;
    const auto id = active_;
    const auto originalDraft = draft_ ? draft_->id : std::string{};
    preserving_ = true;
    active_.clear();
    upstream::CaptureOverlay::instance().cancel();
    active_ = id;
    auto& scrolling = upstream::ScrollCapture::instance();
    auto& overlay = upstream::ScrollCaptureOverlay::instance();
    try {
    if (!overlay.initialize()) throw std::runtime_error("无法初始化长截图控制栏");
    scrolling.setProgressCallback([](const cv::Mat& image, int frames) {
        auto preview = image.clone();
        MainThreadDispatcher::instance().post([preview = std::move(preview), frames] { upstream::ScrollCaptureOverlay::instance().updatePreview(preview, frames); });
    });
    scrolling.setCompletionCallback([this, id, originalDraft, originalLive](const upstream::ScrollCaptureResult& result) {
        auto image = result.stitchedImage.clone();
        MainThreadDispatcher::instance().post([this, id, originalDraft, originalLive, result, image = std::move(image)] {
            upstream::ScrollCaptureOverlay::instance().hide();
            if (id != active_) return;
            retaking_ = false;
            if (result.cancelled || !result.success) {
                if (!originalDraft.empty()) {
                    resumeDraft(id, originalDraft);
                    upstream::CaptureOverlay::instance().winoneState().winoneLiveCapture = originalLive;
                    upstream::CaptureOverlay::instance().winoneState().toolbarLayoutValid = false;
                    upstream::CaptureOverlay::instance().winoneInvalidate();
                    if (!result.cancelled) notice(result.errorMessage);
                } else { active_.clear(); complete(id, {{"cancelled", true}}); }
                return;
            }
            if (!upstream::CaptureOverlay::instance().startEditLongImage(image, result.frameCount,
                [](const cv::Mat&, upstream::ScrollFinishAction, int) {})) {
                resumeDraft(id, originalDraft); notice("无法打开长截图编辑器，原截图已保留"); return;
            }
            retireDraft(); draft_.reset(); draftFingerprint_.clear(); preserving_ = false;
            mode_ = "scroll-edit"; snapshotDraft(true);
            using Finish = upstream::ScrollFinishAction;
            using Action = upstream::CaptureCompletionAction;
            if (result.action != Finish::Confirm) {
                const auto action = result.action == Finish::Pin ? Action::Pin : result.action == Finish::Copy ? Action::Copy :
                    result.action == Finish::SaveAs ? Action::SaveAs : Action::QuickSave;
                upstream::CaptureOverlay::instance().confirmCurrentSelection({action});
            }
        });
    });
    upstream::ScrollCaptureOptions options;
    options.captureRect = {region.x, region.y, region.x + region.width, region.y + region.height};
    options.scrollDelayMs = captureSettings().value("scrollDelay", 300);
    options.interactiveHoldAtBottom = true;
    options.stagingDirectory = directory_ / L"scroll-staging";
    std::filesystem::create_directories(options.stagingDirectory);
    options.isUiHotZone = [](POINT p) { return upstream::ScrollCaptureOverlay::instance().isPointInControlZone(p); };
    if (captureSettings().value("scrollFromTop", false)) {
        POINT center{region.x + region.width / 2, region.y + region.height / 2};
        const auto target = WindowFromPoint(center);
        if (target) {
            DWORD_PTR ignored = 0;
            SendMessageTimeoutW(target, WM_VSCROLL, SB_TOP, 0, SMTO_ABORTIFHUNG, 500, &ignored);
        }
    }
    overlay.show(options.captureRect); scrolling.start(options);
    } catch (...) {
        overlay.hide(); retaking_ = false;
        if (!originalDraft.empty()) {
            resumeDraft(id, originalDraft);
            upstream::CaptureOverlay::instance().winoneState().winoneLiveCapture = originalLive;
            upstream::CaptureOverlay::instance().winoneState().toolbarLayoutValid = false;
            upstream::CaptureOverlay::instance().winoneInvalidate();
        }
        throw;
    }
}

void CaptureApp::retakeScrolling() {
    auto& view = upstream::CaptureOverlay::instance();
    if (!view.isLongImageEditing() || active_.empty()) throw std::runtime_error("当前没有可重拍的长截图");
    snapshotDraft(true);
    const auto id = active_;
    preserving_ = true; active_.clear(); view.cancel(); active_ = id;
    retaking_ = true; mode_ = "scroll";
    upstream::CaptureOptions options;
    options.copyToClipboard = false; options.saveToFile = false; options.recordHistory = false;
    options.autoBypassFullscreen = false;
    view.startSelection(options);
    preserving_ = false;
    if (!view.isOverlayVisible()) { retaking_ = false; resumeDraft(id, draft_->id); throw std::runtime_error("无法重新取得屏幕，原长图已保留"); }
    view.winoneState().winoneLiveCapture = true;
}
}
