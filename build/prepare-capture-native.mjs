// Verify the pinned source before making an isolated build copy. Adaptations
// add explicit host callbacks; no upstream drawing algorithm is reimplemented.
import fs from 'node:fs/promises';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import {execFileSync} from 'node:child_process';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const [sourceArg, outputArg] = process.argv.slice(2);
if (!sourceArg || !outputArg) throw Error('Usage: prepare-capture-native.mjs <pinned-source> <isolated-output>');
const source = path.resolve(sourceArg), output = path.resolve(outputArg);
const temporaryRoot = path.join(root, '_temp') + path.sep;
if (!output.toLowerCase().startsWith(temporaryRoot.toLowerCase()) || output === source)
    throw Error('Native capture adaptations require a repository _temp directory');
execFileSync('git', ['-C', root, 'check-ignore', '--quiet', '--no-index', path.join(path.relative(root, output), 'probe')]);
const lock = JSON.parse(await fs.readFile(path.join(root, 'third_party/tools3000/source.json'), 'utf8'));
const prepared = new Map();
for (const file of lock.files) {
    const relative = file.path;
    if (path.isAbsolute(relative) || relative.split('/').includes('..')) throw Error('Invalid upstream source path');
    const data = await fs.readFile(path.join(source, relative));
    if (createHash('sha256').update(data).digest('hex') !== file.sha256) throw Error('Upstream source differs: ' + relative);
    prepared.set(relative, data);
}

async function adapt(relative, edits) {
    let text = prepared.get(relative).toString('utf8').replace(/\r\n/g, '\n');
    for (const [before, after] of edits) {
        if (text.split(before).length !== 2) throw Error('Upstream adaptation anchor changed: ' + relative + ': ' + before.slice(0, 90));
        text = text.replace(before, after);
    }
    prepared.set(relative, Buffer.from(text, 'utf8'));
}

await adapt('src/ocr/OcrEngine.cpp', [
    ['#include "ocr/OcrEngine.h"', '#include "ocr/OcrEngine.h"\n#include "community_hooks.h"'],
    ['bool OcrEngine::isAvailable() const {', 'bool OcrEngine::isAvailable() const {\n    return winone::community::ocrAvailable();\n#if 0'],
    ['bool OcrEngine::initialize() {', '#endif\n}\n\nbool OcrEngine::initialize() {\n    return winone::community::ocrAvailable();\n#if 0'],
    ['    return isAvailable();\n}', '    return isAvailable();\n#endif\n}'],
    ['std::vector<OcrResult> OcrEngine::extractText(const cv::Mat& image, const std::string& scriptModeOverride) {',
     'std::vector<OcrResult> OcrEngine::extractText(const cv::Mat& image, const std::string& scriptModeOverride) {\n    (void)scriptModeOverride;\n    return winone::community::ocr(image);\n#if 0'],
    ['OcrPageExtraction OcrEngine::extractPage(const cv::Mat& image, const std::string& scriptModeOverride) {',
     '#endif\n}\n\nOcrPageExtraction OcrEngine::extractPage(const cv::Mat& image, const std::string& scriptModeOverride) {\n    (void)scriptModeOverride;\n    OcrPageExtraction nativePage;\n    nativePage.lines = winone::community::ocr(image);\n    nativePage.activeEngine = "rapidocrlite";\n    return nativePage;\n#if 0'],
    ['std::string OcrEngine::recognizeToText(', '#endif\n}\n\nstd::string OcrEngine::recognizeToText('],
]);

// Pin language menus read the active shared OCR's state, without constructing
// four unused OpenCV networks merely to ask which model generation is present.
for (const relative of ['src/capture/PinWindowInput.cpp', 'src/capture/PinWindowOcr.cpp']) {
    let text = prepared.get(relative).toString('utf8');
    text = text.replace('#include "ocr/DeepOcrEngine.h"', '#include "ocr_model_state.h"');
    text = text.replaceAll('tools3000::ocr::DeepOcrEngine::instance()', 'winone::community::ocrModelState()');
    prepared.set(relative, Buffer.from(text));
}
await adapt('src/ocr/OcrEngine.cpp', [
    ['#include "ocr/DeepOcrEngine.h"', '// Recognition and lifetime are owned by WinOne.'],
    ['    DeepOcrEngine::instance().shutdown();', '    // Shared OCR is shut down by the Host, not by a pin window.'],
]);

await adapt('src/core/translate/TranslationService.cpp', [
    ['#include "core/translate/TranslationService.h"', '#include "core/translate/TranslationService.h"\n#include "community_hooks.h"'],
    ['    return translateBatchWithSettings(request, loadSettings(), cancelFlag);', '    return winone::community::translate(request, cancelFlag);'],
]);
{
    const relative='src/core/translate/TranslationService.cpp';
    let text=prepared.get(relative).toString('utf8');
    const begin=text.indexOf('TranslationBatchResult TranslationService::translateBatchWithSettings(');
    const body=text.indexOf('{',begin),end=text.indexOf('\n}\n',body);
    if(begin<0||body<0||end<0)throw Error('Community configured translation entry changed');
    // Pins snapshot upstream settings and enter this overload directly. Both
    // public translation entrypoints must use WinOne's selected provider;
    // upstream default providers must never form a second runtime path.
    text=text.slice(0,body)+`{
    (void)settings;
    return winone::community::translate(request, cancelFlag);
}`+text.slice(end+2);
    prepared.set(relative,Buffer.from(text));
}

await adapt('src/capture/CaptureOverlay.h', [
    ['    bool isOverlayVisible() const {', `    // Host callbacks are called on the existing upstream UI thread.
    using WinOneDelivery = std::function<bool(const CaptureRegion&, const cv::Mat&, const CaptureCompletion&)>;
    void setWinOneDelivery(WinOneDelivery callback) { m_winoneDelivery = std::move(callback); }
    void setWinOneCanvasOwner(std::function<bool()> callback) { m_winoneCanvasOwner = std::move(callback); }
    HWND winoneWindow() const { return m_hwnd; }
    CaptureState& winoneState() { return m_state; }
    void winoneInvalidate() { m_renderer.markMarkupDirty(); m_renderer.invalidate(); }
    bool winoneResizeRenderSurface(UINT width, UINT height) {
        const auto target = m_renderer.getRenderTarget();
        return target && SUCCEEDED(target->Resize(D2D1::SizeU(width, height)));
    }
    bool isOverlayVisible() const {`],
    ['    CaptureState m_state;', '    WinOneDelivery m_winoneDelivery;\n    std::function<bool()> m_winoneCanvasOwner;\n    CaptureState m_state;'],
    ['    void setWinOneDelivery(WinOneDelivery callback) { m_winoneDelivery = std::move(callback); }',
     '    void setWinOneDelivery(WinOneDelivery callback) { m_winoneDelivery = std::move(callback); }\n    void setWinOneRecording(std::function<bool(const CaptureRegion&, const RecordSetupOptions&)> callback) { m_winoneRecording = std::move(callback); }'],
    ['    WinOneDelivery m_winoneDelivery;', '    WinOneDelivery m_winoneDelivery;\n    std::function<bool(const CaptureRegion&, const RecordSetupOptions&)> m_winoneRecording;'],
]);

await adapt('src/capture/CaptureState.h', [
    ['enum class ToolbarCommand {', 'enum class ToolbarCommand {\n    WinOneRecordCodec, WinOneGifQuality, WinOneGifSeconds, WinOneRecordCursor, WinOneRecordDestination, WinOneMove, WinOneRegion, WinOneCover, WinOneCustomize, WinOneRecordGif, WinOneSaveAs, WinOneRetake,'],
    ['    None = 0,\n    RecordFormat,', '    None = 0,\n    WinOneToolbar, WinOneCodec, WinOneQuality, WinOneSeconds,\n    RecordFormat,'],
    ['    cv::Mat frozenScreen;', `    // A translated view is separate from original pixels and editable marks.
    cv::Mat winoneTranslated;
    bool winoneShowTranslated = false;
    bool winoneRegionTool = false;
    bool winoneMoveTool = false;
    bool winoneLiveCapture = false;
    float winoneProgress = -1.f;
    std::wstring winoneProgressText;
    std::function<void(uint32_t)> winoneEditNumber;
    std::function<void(int)> winoneCustomize;
    std::function<bool(bool)> winonePrepareRecording;
    std::function<void()> winoneChooseDestination;
    bool winoneNumberStyleInitialized = false;
    bool winoneNumberCreated = false;
    bool winoneRecordCursor = true;
    bool winoneScrollReady = false;
    int winoneGifQuality = 85, winoneGifSeconds = 120;
    std::function<void()> winoneRetake;
    std::function<void(const CaptureRegion&)> winoneScroll;
    std::function<void()> winoneSelected;
    std::function<bool()> winoneBeforeOutput;
    std::function<std::wstring()> winoneSaveAs;
    cv::Mat frozenScreen;`],
]);

await adapt('src/capture/CaptureOverlay.cpp', [
    ['void CaptureOverlay::confirmSelection(CaptureCompletion completion) {', `void CaptureOverlay::confirmSelection(CaptureCompletion completion) {
    if (m_state.winoneBeforeOutput && !m_state.winoneBeforeOutput()) return;`],
    ['void CaptureOverlay::shutdown() {', 'void CaptureOverlay::shutdown() {\n    m_input.shutdownDetection();'],
    ['bool CaptureOverlay::freezeScreen() {\n    releaseFrozenSurface();', `bool CaptureOverlay::freezeScreen() {
    releaseFrozenSurface();
    // Only the separately built private-desktop probe exports this provider.
    using Fixture = bool(*)(void*);
    if (auto fixture = reinterpret_cast<Fixture>(GetProcAddress(GetModuleHandleW(nullptr), "WinOneNativeCaptureFixture")))
        return fixture(&m_state.frozenScreen);`],
    ['    CaptureRegion region{x1 + offsetX, y1 + offsetY, w, h, m_state.cornerRadius};', `    CaptureRegion region{x1 + offsetX, y1 + offsetY, w, h, m_state.cornerRadius};
    // Keep editor completion callbacks intact when Host rejects a delivery.
    // Pin edits are owned by PinWindow and retain its normal completion path.
    if (m_winoneRecording && m_state.mode == OverlayMode::RecordRegion) {
        if (!m_winoneRecording(region, m_state.recordSetup)) return;
        cleanupAfterDelivery(false);
        ShortcutHintOverlay::instance().hide();
        return;
    }
    if (m_winoneDelivery && m_state.mode != OverlayMode::RecordRegion && m_winoneCanvasOwner && m_winoneCanvasOwner()) {
        const cv::Mat& shown = m_state.winoneShowTranslated && !m_state.winoneTranslated.empty()
            ? m_state.winoneTranslated : cropped;
        if (!m_winoneDelivery(region, shown, completion)) return;
        cleanupAfterDelivery(true);
        ShortcutHintOverlay::instance().hide();
        return;
    }`],
    ['void CaptureOverlay::resetTransientOverlayState() {', 'void CaptureOverlay::resetTransientOverlayState() {\n    m_state.winoneScrollReady = false;\n    m_state.winoneLiveCapture = false;\n    m_state.winoneMoveTool = false;\n    m_state.winoneRegionTool = false;\n    m_state.winoneProgress = -1;\n    m_state.winoneProgressText.clear();\n    m_state.winoneTranslated.release();\n    m_state.winoneShowTranslated = false;'],
    ['            return self->m_input.handleMessage(hwnd, msg, wParam, lParam);', `            const bool wasSelecting = self->m_state.state.load() == OverlayState::Selecting;
            const auto result = self->m_input.handleMessage(hwnd, msg, wParam, lParam);
            if (msg == WM_LBUTTONUP && wasSelecting && self->m_state.state.load() == OverlayState::Selected && self->m_state.winoneSelected)
                self->m_state.winoneSelected();
            return result;`],
]);
await adapt('src/capture/WindowElementDetector.h', [
    ['    ~WindowElementDetector();', '    ~WindowElementDetector();\n    void shutdown();'],
]);
await adapt('src/core/utils/OutOfProcessFileDialog.cpp', [
    ['            1, &hProcess, FALSE, 250, QS_SENDMESSAGE | QS_PAINT);', '            1, &hProcess, FALSE, 250, QS_SENDMESSAGE | QS_PAINT | QS_POSTMESSAGE | QS_TIMER);'],
    ['        while (PeekMessageW(&msg, nullptr, 0, 0, PM_REMOVE | PM_QS_SENDMESSAGE | PM_QS_PAINT)) {', `        while (PeekMessageW(&msg, nullptr, 0, 0, PM_REMOVE | PM_QS_SENDMESSAGE | PM_QS_PAINT | PM_QS_POSTMESSAGE)) {
            if (msg.message == WM_QUIT) {
                TerminateProcess(hProcess, ERROR_CANCELLED);
                WaitForSingleObject(hProcess, 5000);
                PostQuitMessage(static_cast<int>(msg.wParam));
                return;
            }
            // Host dispatch and lifecycle timers must stay responsive while
            // a file dialog is open. Disabled-owner input remains blocked.
            if (msg.hwnd == modalOwner && ((msg.message >= WM_KEYFIRST && msg.message <= WM_KEYLAST) ||
                (msg.message >= WM_MOUSEFIRST && msg.message <= WM_MOUSELAST))) continue;`],
    ['    STARTUPINFOW si{};', `    struct OwnedDialogJob { HANDLE handle = CreateJobObjectW(nullptr, nullptr); ~OwnedDialogJob() { if (handle) CloseHandle(handle); } } childJob;
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits{};
    limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
    if (!childJob.handle || !SetInformationJobObject(childJob.handle, JobObjectExtendedLimitInformation, &limits, sizeof(limits))) {
        LOG_ERROR("Cannot protect native file-dialog child lifetime"); return {};
    }
    STARTUPINFOW si{};`],
    ['            FALSE,\n            0,\n            nullptr,', '            FALSE,\n            CREATE_SUSPENDED,\n            nullptr,'],
    ['    AllowSetForegroundWindow(pi.dwProcessId);', `    if (!AssignProcessToJobObject(childJob.handle, pi.hProcess) || ResumeThread(pi.hThread) == static_cast<DWORD>(-1)) {
        TerminateProcess(pi.hProcess, ERROR_CANCELLED); WaitForSingleObject(pi.hProcess, 5000);
        CloseHandle(pi.hThread); CloseHandle(pi.hProcess);
        LOG_ERROR("Cannot attach native file-dialog child to its owner"); return {};
    }
    AllowSetForegroundWindow(pi.dwProcessId);`],
    ['    if (exitCode != 0 || !std::filesystem::exists(outPath, ec)) {', `    if (exitCode == ERROR_CANCELLED) { FileDialogResult cancelled; cancelled.cancelled = true; return cancelled; }
    if (exitCode != 0 || !std::filesystem::exists(outPath, ec)) {`],
]);
await adapt('src/capture/WindowElementDetector.cpp', [
    ['WindowElementDetector::~WindowElementDetector() {', 'WindowElementDetector::~WindowElementDetector() { shutdown(); }\n\nvoid WindowElementDetector::shutdown() {'],
]);
await adapt('src/capture/CaptureInput.cpp', [
    ['void CaptureInput::setCurrentTool(MarkupTool tool) {', `void CaptureInput::setCurrentTool(MarkupTool tool) {
    if(tool==MarkupTool::Number && !m_state->winoneNumberStyleInitialized) {
        m_state->currentColor=MarkupColor::Red();m_state->winoneNumberStyleInitialized=true;
    }`],
]);
await adapt('src/capture/CaptureInput.h', [
    ['    void endDetectionSession();', '    void endDetectionSession();\n    void shutdownDetection() { m_elementDetector.shutdown(); }'],
]);
await adapt('src/core/utils/WinUtils.h', [
    ['    static bool isWindowVisibleOnCurrentDesktop(HWND hwnd);',
     '    static bool isWindowVisibleOnCurrentDesktop(HWND hwnd);\n    static void releaseThreadDesktopManager();'],
]);
{
    const relative = 'src/core/utils/WinUtils.cpp';
    let text = prepared.get(relative).toString('utf8').replace(/\r\n/g, '\n');
    const first = text.indexOf('    struct ThreadLocalVdm {');
    const last = text.indexOf('    thread_local ThreadLocalVdm tlsVdm;', first);
    if (first < 0 || last < 0) throw Error('Upstream virtual-desktop lifetime anchor changed');
    const declaration = text.slice(first, last + '    thread_local ThreadLocalVdm tlsVdm;'.length);
    text = text.slice(0, first) + text.slice(last + '    thread_local ThreadLocalVdm tlsVdm;'.length);
    const anchor = 'bool WinUtils::isWindowVisibleOnCurrentDesktop(HWND hwnd) {';
    if (text.split(anchor).length !== 2) throw Error('Upstream visibility function changed');
    text = text.replace(anchor, `namespace {\n${declaration}\n}\n
// Release this apartment-bound COM object before CRT/TLS teardown enters the
// loader lock. The same cache and visibility algorithm remain in use.
void WinUtils::releaseThreadDesktopManager() {
    tlsVdm.mgr.Reset();
    if (tlsVdm.ownCom) { CoUninitialize(); tlsVdm.ownCom = false; }
    tlsVdm.attempted = false;
}

${anchor}`);
    prepared.set(relative, Buffer.from(text));
}
await adapt('src/capture/WindowElementDetector.cpp', [
    ['        CoUninitialize();', '        tools3000::core::WinUtils::releaseThreadDesktopManager();\n        CoUninitialize();'],
]);

// Do not report a successful capture of fabricated pixels after a failed OS
// capture. The private test target provides its explicit fixture above instead.
{
    const relative = 'src/capture/CaptureOverlay.cpp';
    let text = prepared.get(relative).toString('utf8');
    const start = text.indexOf('    } else if (');
    const marker = text.indexOf('启用安全底图兜底');
    if (marker < 0) throw Error('Upstream failed-capture branch changed');
    const branch = text.lastIndexOf('    } else if (', marker);
    const end = text.indexOf('    } else {', marker);
    if (start < 0 || branch < 0 || end < 0) throw Error('Cannot isolate upstream failed-capture branch');
    text = text.slice(0, branch) + text.slice(end);
    prepared.set(relative, Buffer.from(text));
}

await adapt('src/capture/CaptureRendererMarkup.cpp', [
    ['    if (!state.markupBaseReady || state.markup.elementCount() == 0 || !m_renderTarget) return;',
     '    if (!m_renderTarget || (!(state.winoneShowTranslated && !state.winoneTranslated.empty()) && (!state.markupBaseReady || state.markup.elementCount() == 0))) return;'],
    ['        cv::Mat composite = state.markup.getCompositeImage(true);',
     '        cv::Mat composite = state.winoneShowTranslated && !state.winoneTranslated.empty() ? state.winoneTranslated : state.markup.getCompositeImage(true);'],
]);
await adapt('src/capture/CaptureRenderer.cpp', [
    ['    auto hwndProps = D2D1::HwndRenderTargetProperties(', `    if (GetProcAddress(GetModuleHandleW(nullptr), "WinOneNativeRenderedFrame"))
        rtProps.usage = D2D1_RENDER_TARGET_USAGE_GDI_COMPATIBLE;
    auto hwndProps = D2D1::HwndRenderTargetProperties(`],
]);
{
    const relative = 'src/capture/CaptureRendererHud.cpp';
    let text = prepared.get(relative).toString('utf8');
    const anchor = 'recoverFromDeviceLoss(m_renderTarget->EndDraw());';
    if (text.split(anchor).length !== 4) throw Error('Community frame presentation changed');
    text = text.replaceAll(anchor, `if (auto readback = reinterpret_cast<void(*)(void*)>(GetProcAddress(GetModuleHandleW(nullptr), "WinOneNativeRenderedFrame")))
            readback(m_renderTarget.Get());
        recoverFromDeviceLoss(m_renderTarget->EndDraw());`);
    prepared.set(relative, Buffer.from(text));
}

await adapt('src/capture/CaptureInputCommands.cpp', [
    ['void CaptureInput::executeToolbarCommand(const ToolbarButton& button) {', `void CaptureInput::executeToolbarCommand(const ToolbarButton& button) {
    // Number style changes establish the next badge's preset. Existing step
    // labels keep their chosen size/color until explicitly edited or moved.
    if (button.isSecondary && m_state->activeElement && m_state->activeElement->tool == MarkupTool::Number) {
        m_state->activeElement->isActive = false; m_state->activeElement = nullptr;
    }`],
    ['    switch (button.command) {', `    switch (button.command) {
        case ToolbarCommand::WinOneMove:
        case ToolbarCommand::WinOneRegion:
            m_state->winoneMoveTool = button.command == ToolbarCommand::WinOneMove;
            m_state->winoneRegionTool = button.command == ToolbarCommand::WinOneRegion;
            m_state->isMarkupToolActive = false;
            if (m_state->activeElement) m_state->activeElement->isActive = false;
            m_state->activeElement = nullptr;
            m_state->toolbarLayoutValid = false;
            m_renderer->invalidate(); return;
        case ToolbarCommand::WinOneCover:
            setCurrentTool(MarkupTool::Rectangle);
            m_state->currentFillMode = true;
            m_state->currentColor = MarkupColor::Black();
            m_renderer->invalidate(); return;
        case ToolbarCommand::WinOneCustomize:
            openDropdownMenu(DropdownType::WinOneToolbar, button.rect); return;
        case ToolbarCommand::WinOneRecordCodec:
            if (m_state->recordSetup.format != RecordFormat::GIF) openDropdownMenu(DropdownType::WinOneCodec, button.rect); return;
        case ToolbarCommand::WinOneGifQuality:
            openDropdownMenu(DropdownType::WinOneQuality, button.rect); return;
        case ToolbarCommand::WinOneGifSeconds:
            openDropdownMenu(DropdownType::WinOneSeconds, button.rect); return;
        case ToolbarCommand::WinOneRecordCursor:
            m_state->winoneRecordCursor = !m_state->winoneRecordCursor;
            m_state->toolbarLayoutValid = false; m_renderer->invalidate(); return;
        case ToolbarCommand::WinOneRecordDestination:
            if (m_state->winoneChooseDestination) m_state->winoneChooseDestination(); return;
        case ToolbarCommand::WinOneRetake:
            if (m_state->winoneRetake) m_state->winoneRetake(); return;
        case ToolbarCommand::WinOneSaveAs:
            triggerSaveSelectionAs(m_hwnd); return;
        case ToolbarCommand::WinOneRecordGif:
            if (m_state->winonePrepareRecording && !m_state->winonePrepareRecording(true)) return;
            transitionToRecordMode(*m_state);
            m_state->recordSetup.format = RecordFormat::GIF;
            m_state->recordSetup.fps = 15;
            m_renderer->markMarkupDirty(); m_renderer->invalidate(); return;`],
    ['                if(m_cancelCb) m_cancelCb(); \n                ocrCb(region, cropped);', '                ocrCb(region, cropped);'],
    ['                if (m_cancelCb) m_cancelCb();\n                trCb(region, cropped);', '                trCb(region, cropped);'],
]);
await adapt('src/capture/CaptureShortcutPolicy.h', [
    ['? CaptureShortcutAction::QuickSaveSelection\n                        : CaptureShortcutAction::SaveSelectionAs;',
     '? CaptureShortcutAction::SaveSelectionAs\n                        : CaptureShortcutAction::QuickSaveSelection;'],
    ['        if (ctx.editingText) {', `        if (ctx.editingText && ctrl && vk == 'S') {
            out.action = shift ? CaptureShortcutAction::SaveSelectionAs : CaptureShortcutAction::QuickSaveSelection;
            return out;
        }
        if (ctx.editingText) {`],
]);
await adapt('src/capture/CaptureInputCommands.cpp', [
    ['void CaptureInput::triggerSaveSelectionAs(HWND hwnd) {', `void CaptureInput::triggerSaveSelectionAs(HWND hwnd) {
    if (m_state && m_state->winoneSaveAs) {
        closeAllCapturePopups(*m_state, false); commitPendingMarkupForDelivery();
        const auto path = m_state->winoneSaveAs();
        if (!path.empty() && m_confirmCb) m_confirmCb({CaptureCompletionAction::SaveAs,
            tools3000::core::WinUtils::wstringToUtf8(path), formatForSavePath(std::filesystem::path(path), 1)});
        return;
    }`],
]);
await adapt('src/capture/CaptureRendererToolbar.cpp', [
    ['            bool isDisabled = false;', `            if (!button.isSecondary) {
                if (button.command == ToolbarCommand::SelectTool)
                    isActiveTool = state.isMarkupToolActive && button.tool == state.currentTool &&
                        !(button.tool == MarkupTool::Rectangle && state.currentFillMode);
                if (button.command == ToolbarCommand::WinOneCover) isActiveTool = state.isMarkupToolActive && state.currentTool == MarkupTool::Rectangle && state.currentFillMode;
                if (button.command == ToolbarCommand::WinOneMove) isActiveTool = state.winoneMoveTool;
                if (button.command == ToolbarCommand::WinOneRegion) isActiveTool = state.winoneRegionTool;
            }
            bool isDisabled = false;`],
]);
{
    const relative = 'src/capture/CaptureInputCommands.cpp';
    let text = prepared.get(relative).toString('utf8');
    const begin = text.indexOf('        case ToolbarCommand::PinWindow: {');
    const end = text.indexOf('        case ToolbarCommand::ScrollCapture:', begin);
    if (begin < 0 || end < 0) throw Error('Community pin delivery changed');
    text = text.slice(0, begin) + `        case ToolbarCommand::PinWindow:
            if (m_confirmCb) m_confirmCb({CaptureCompletionAction::Pin});
            return;

` + text.slice(end);
    const scroll = '            RECT capRect = {x1 + offsetX, y1 + offsetY, x1 + offsetX + w, y1 + offsetY + h};';
    if (text.split(scroll).length !== 2) throw Error('Community scroll selection changed');
    text = text.replace(scroll, scroll + `
            if (m_state->winoneScroll) {
                m_state->winoneScroll(CaptureRegion{capRect.left, capRect.top, w, h});
                return;
            }`);
    prepared.set(relative, Buffer.from(text));
}
await adapt('src/capture/CaptureInputKeyboard.cpp', [
    ['                const auto route = resolveLongImageWheelRoute(\n                    *m_state, wheelPt, wheelBtn != nullptr, ctrlDown, shiftDown);',
     '                const auto route = wheelBtn == nullptr ? LongImageWheelRoute::CanvasZoom : resolveLongImageWheelRoute(\n                    *m_state, wheelPt, true, ctrlDown, shiftDown);'],
    ['                case CaptureShortcutAction::HierarchicalEscape:', `                case CaptureShortcutAction::HierarchicalEscape:
                    if (!hasAnyCapturePopupOpen(*m_state) && !keyCtx.editingText) {
                        if (m_cancelCb) m_cancelCb();
                        return 0;
                    }`],
]);
await adapt('src/capture/CaptureInputMouse.cpp', [
    ['                    case DoubleClickIntent::ConfirmSelectionAndCopy:', '                    case DoubleClickIntent::ConfirmSelectionAndCopy:\n                        if (m_state->isLongImageMode()) return 0;'],
    ['                cv::Point local = inSelection ? toMarkupPoint(point) : cv::Point{0, 0};', `                cv::Point local = inSelection ? toMarkupPoint(point) : cv::Point{0, 0};
                // WinOne exposes distinct selection and annotation-move tools.
                // Delegate the actual movement to the community input engine.
                if (m_state->winoneRegionTool && inSelection) {
                    const auto hit = hitTestSelectionBox(point);
                    m_state->isAdjustingSelection = true;
                    m_state->selAdjustHandle = hit == HitArea::None ? HitArea::Body : hit;
                    m_state->selAdjustLast = point;
                    m_state->longCanvas.lastSelAdjustDocPt = m_state->screenToDoc(px, py);
                    return 0;
                }
                if (m_state->winoneMoveTool && (!inSelection || !m_state->markup.getElementAtEx(local).element)) return 0;
                if (!m_state->winoneMoveTool && !m_state->winoneRegionTool && inSelection) {
                    auto hit = m_state->markup.getElementAtEx(local);
                    if (hit.element && hit.element->tool == MarkupTool::Number && m_state->winoneEditNumber) {
                        if (m_state->activeElement) m_state->activeElement->isActive = false;
                        m_state->activeElement = hit.element;
                        hit.element->isActive = true;
                        m_state->winoneEditNumber(hit.element->id);
                        m_renderer->markMarkupDirty(); m_renderer->invalidate();
                        return 0;
                    }
                }`],
    ['        case WM_RBUTTONUP: {', `        case WM_RBUTTONUP: {
            if (!m_state->isLongImageMode() && m_state->dropdownMenu.type == DropdownType::None &&
                m_state->sliderPopup.type == SliderPopupType::None && m_state->openSubmenu == SubmenuType::None) {
                if (m_cancelCb) m_cancelCb();
                return 0;
            }`],
]);
await adapt('src/capture/CaptureInput.cpp', [
    ['void CaptureInput::setCurrentTool(MarkupTool tool) {', `void CaptureInput::setCurrentTool(MarkupTool tool) {
    m_state->winoneMoveTool = false; m_state->winoneRegionTool = false;
    if (tool == MarkupTool::Number && m_state->currentTool != MarkupTool::Number) {
        m_state->currentStrokeWidth = 16; m_state->currentColor = MarkupColor::Red();
    }`],
]);
{
    const relative = 'src/capture/CaptureInputMouse.cpp';
    let text = prepared.get(relative).toString('utf8');
    const begin = text.indexOf('        case WM_LBUTTONDBLCLK: {');
    const end = text.indexOf('        case WM_RBUTTONUP:', begin);
    let part = text.slice(begin, end);
    const anchor = '                                    m_state->currentNumberFormat = hit.element->numberFormat;';
    if (begin < 0 || end < 0 || part.split(anchor).length !== 2) throw Error('Community number edit anchor changed');
    part = part.replace(anchor, anchor + '\n                                    if (m_state->winoneEditNumber) m_state->winoneEditNumber(hit.element->id);');
    text = text.slice(0, begin) + part + text.slice(end);
    prepared.set(relative, Buffer.from(text));
}
{
    const relative = 'src/capture/CaptureToolbarLayout.cpp';
    let text = prepared.get(relative).toString('utf8');
    const begin = text.indexOf('    std::vector<ButtonSpec> specs;\n    // 形状工具组');
    const end = text.indexOf('\n    return specs;\n}', begin);
    if (begin < 0 || end < 0) throw Error('Community screenshot toolbar changed');
    text = text.slice(0, begin) + `    std::vector<ButtonSpec> specs;
    const auto preferences = tools3000::core::ConfigManager::instance().get<nlohmann::json>("/capture/winoneToolbar", nullptr);
    const std::vector<std::string> defaults{"crop","select","rectangle","ellipse","arrow","pen","cover","text","step","mosaic","blur","ocr","translate"};
    const auto selected = preferences.is_array() ? preferences.get<std::vector<std::string>>() : defaults;
    auto add = [&](ToolbarCommand command, MarkupTool tool, const wchar_t* hint, float width = 32.0f) {
        ButtonSpec value; value.command = command; value.tool = tool; value.baseWidth = width; value.tooltipHint = hint;
        specs.push_back(std::move(value));
    };
    auto captureActions = [&] {
        if (state.isLongImageMode()) {
            add(ToolbarCommand::WinOneRetake, MarkupTool::Rectangle, zh ? L"重新滚动截图" : L"Retake scrolling capture");
        } else if (state.winoneLiveCapture) {
            add(ToolbarCommand::ScrollCapture, MarkupTool::Rectangle, zh ? L"滚动截图" : L"Scrolling capture");
            add(ToolbarCommand::StartRecord, MarkupTool::Rectangle, zh ? L"录制视频" : L"Record video");
            add(ToolbarCommand::WinOneRecordGif, MarkupTool::Rectangle, L"GIF");
        }
    };
    if (std::find(selected.begin(), selected.end(), "crop") == selected.end()) captureActions();
    for (const auto& name : selected) {
        if (name == "crop") add(ToolbarCommand::WinOneRegion, MarkupTool::Rectangle, zh ? L"调整选区" : L"Adjust selection");
        else if (name == "select") add(ToolbarCommand::WinOneMove, MarkupTool::Rectangle, zh ? L"移动标注" : L"Move annotation");
        else if (name == "rectangle") add(ToolbarCommand::SelectTool, MarkupTool::Rectangle, zh ? L"矩形" : L"Rectangle");
        else if (name == "ellipse") add(ToolbarCommand::SelectTool, MarkupTool::Ellipse, zh ? L"椭圆" : L"Ellipse");
        else if (name == "arrow") add(ToolbarCommand::SelectTool, MarkupTool::Arrow, zh ? L"箭头" : L"Arrow");
        else if (name == "pen") add(ToolbarCommand::SelectTool, MarkupTool::Pen, zh ? L"画笔" : L"Pen");
        else if (name == "cover") add(ToolbarCommand::WinOneCover, MarkupTool::Rectangle, zh ? L"遮盖" : L"Cover");
        else if (name == "text") add(ToolbarCommand::SelectTool, MarkupTool::Text, zh ? L"文字" : L"Text");
        else if (name == "step") add(ToolbarCommand::SelectTool, MarkupTool::Number, zh ? L"序号说明" : L"Numbered step");
        else if (name == "mosaic") add(ToolbarCommand::SelectTool, MarkupTool::Mosaic, zh ? L"马赛克" : L"Mosaic");
        else if (name == "blur") add(ToolbarCommand::SelectTool, MarkupTool::Blur, zh ? L"模糊" : L"Blur");
        else if (name == "ocr") add(ToolbarCommand::ExtractText, MarkupTool::Rectangle, zh ? L"识别文字" : L"Recognize text");
        else if (name == "translate") add(ToolbarCommand::Translate, MarkupTool::Rectangle, zh ? L"翻译" : L"Translate");
        if (name == "crop") captureActions();
    }
    add(ToolbarCommand::Undo, MarkupTool::Rectangle, zh ? L"撤销" : L"Undo");
    add(ToolbarCommand::Redo, MarkupTool::Rectangle, zh ? L"重做" : L"Redo");
    add(ToolbarCommand::PinWindow, MarkupTool::Rectangle, zh ? L"贴图" : L"Pin");
    add(ToolbarCommand::Copy, MarkupTool::Rectangle, zh ? L"复制" : L"Copy");
    add(ToolbarCommand::QuickSave, MarkupTool::Rectangle, zh ? L"保存" : L"Save");
    add(ToolbarCommand::WinOneSaveAs, MarkupTool::Rectangle, zh ? L"另存为" : L"Save as");
    add(ToolbarCommand::WinOneCustomize, MarkupTool::Rectangle, zh ? L"自定义工具栏" : L"Customize toolbar");
    add(ToolbarCommand::Cancel, MarkupTool::Rectangle, zh ? L"取消" : L"Cancel");
    add(ToolbarCommand::Confirm, MarkupTool::Rectangle, zh ? L"确认并复制" : L"Confirm and copy");
` + text.slice(end);
    prepared.set(relative, Buffer.from(text));
}
{
    const relative = 'src/capture/CaptureRendererMarkup.cpp';
    let text = prepared.get(relative).toString('utf8');
    const anchor = '    if (!m_renderTarget || !brush) return;';
    if (text.split(anchor).length !== 2) throw Error('Community toolbar icon renderer changed');
    text = '#include "toolbar_icons.h"\n' + text;
    text = text.replace(anchor, anchor + `
    if (!button.label.empty()) {
        TextFormatScope format(m_infoTextFormat.Get(), DWRITE_TEXT_ALIGNMENT_CENTER, DWRITE_PARAGRAPH_ALIGNMENT_CENTER, DWRITE_WORD_WRAPPING_NO_WRAP);
        m_renderTarget->DrawTextW(button.label.c_str(),static_cast<UINT32>(button.label.size()),m_infoTextFormat.Get(),rect,brush);
        return;
    }
    const char* winoneIcon = nullptr;
    switch (button.command) {
        case ToolbarCommand::WinOneMove: winoneIcon = "select"; break;
        case ToolbarCommand::WinOneRegion: winoneIcon = "region"; break;
        case ToolbarCommand::WinOneCover: winoneIcon = "cover"; break;
        case ToolbarCommand::WinOneCustomize: winoneIcon = "overflow"; break;
        case ToolbarCommand::WinOneRecordGif: winoneIcon = "gif"; break;
        case ToolbarCommand::WinOneSaveAs: winoneIcon = "saveAs"; break;
        case ToolbarCommand::WinOneRetake: winoneIcon = "scroll"; break;
        case ToolbarCommand::Undo: winoneIcon = "undo"; break;
        case ToolbarCommand::Redo: winoneIcon = "redo"; break;
        case ToolbarCommand::PinWindow: winoneIcon = "pin"; break;
        case ToolbarCommand::Copy: winoneIcon = "copy"; break;
        case ToolbarCommand::QuickSave: winoneIcon = "save"; break;
        case ToolbarCommand::Confirm: winoneIcon = "confirm"; break;
        case ToolbarCommand::Cancel: winoneIcon = "cancel"; break;
        case ToolbarCommand::ExtractText: winoneIcon = "ocr"; break;
        case ToolbarCommand::Translate: winoneIcon = "translate"; break;
        case ToolbarCommand::ScrollCapture: winoneIcon = "scroll"; break;
        case ToolbarCommand::StartRecord: winoneIcon = "video"; break;
        case ToolbarCommand::SelectTool:
            if (button.tool == MarkupTool::Rectangle) winoneIcon = "rectangle";
            if (button.tool == MarkupTool::Ellipse) winoneIcon = "ellipse";
            if (button.tool == MarkupTool::Arrow) winoneIcon = "arrow";
            if (button.tool == MarkupTool::Pen) winoneIcon = "pen";
            if (button.tool == MarkupTool::Text) winoneIcon = "text";
            if (button.tool == MarkupTool::Number) winoneIcon = "step";
            if (button.tool == MarkupTool::Mosaic) winoneIcon = "mosaic";
            if (button.tool == MarkupTool::Blur) winoneIcon = "blur";
            break;
        default: break;
    }
    const float iconSide = std::min(button.rect.bottom - button.rect.top, button.rect.right - button.rect.left) * (18.0f / 32.0f);
    const float iconX = (rect.left + rect.right) / 2, iconY = (rect.top + rect.bottom) / 2;
    const auto winoneRect = D2D1::RectF(iconX - iconSide / 2, iconY - iconSide / 2, iconX + iconSide / 2, iconY + iconSide / 2);
    if (winoneIcon && !button.isSecondary && winone::icons::render(m_renderTarget.Get(), winoneIcon,
        FloatingGlassBar::isEffectiveDarkMode(), winoneRect, brush->GetOpacity())) return;
    CaptureIconId nativeIcon = CaptureIconId::None;
    switch (button.command) {
        case ToolbarCommand::WinOneMove: nativeIcon = CaptureIconId::ActionResetScale; break;
        case ToolbarCommand::WinOneRegion: nativeIcon = CaptureIconId::ActionResetSelection; break;
        case ToolbarCommand::WinOneCover: nativeIcon = CaptureIconId::PropFillSolid; break;
        case ToolbarCommand::WinOneCustomize: nativeIcon = CaptureIconId::PropStrokeWidth; break;
        case ToolbarCommand::WinOneRecordGif: nativeIcon = CaptureIconId::ActionRecordVideo; break;
        case ToolbarCommand::WinOneSaveAs: nativeIcon = CaptureIconId::ActionSave; break;
        default: break;
    }
    if (nativeIcon != CaptureIconId::None) {
        CaptureVectorIcons::renderIcon(m_renderTarget.Get(), m_d2dFactory.Get(), nativeIcon, rect, brush, scale);
        return;
    }`);
    prepared.set(relative, Buffer.from(text));
}
{
    const relative = 'src/capture/MarkupEngineEffects.cpp';
    let text = prepared.get(relative).toString('utf8').replace(/\r\n/g, '\n');
    text = '#include "number_label.h"\n' + text;
    // Keep circular badge flat to match WinOne's flat annotation style.
    text = text.replace(
        '                    Gdiplus::SolidBrush shadowBrush(Gdiplus::Color(95, 0, 0, 0));\n                    g.FillEllipse(&shadowBrush, cx - radius + 1.5f * scale, cy - radius + 2.5f * scale, radius * 2.0f, radius * 2.0f);\n',
        '');
    const begin = text.indexOf('class NumberHandler : public DefaultBoxHandler {');
    const end = text.indexOf('class MagnifierHandler : public DefaultBoxHandler {', begin);
    if (begin < 0 || end < 0) throw Error('Community number handler changed');
    let handler = text.slice(begin, end);
    handler = handler.replace('        return baseDpi * sizeFactor;', '        (void)baseDpi; (void)sizeFactor;\n        return std::max(10.f, element.fontSize) / 22.0f;');
    const bounds = '        return badgeBox;';
    if (handler.split(bounds).length !== 2) throw Error('Community number bounds changed');
    handler = handler.replace(bounds, '        const auto label = winone::number_label::bubbleBounds(element);\n        return label.empty() ? badgeBox : (badgeBox | label);');
    handler = handler.replace('    HitArea hitTest(const MarkupElement& element, cv::Point pt, int padding) const override {',
        '    HitArea hitTest(const MarkupElement& element, cv::Point pt, int padding) const override {\n        if (winone::number_label::bubbleBounds(element).contains(pt)) return HitArea::Body;');
    handler = handler.replace('        renderSnipasteStyleNumberBadge(canvas, element.numberValue, element.startPt, color,',
        '        winone::number_label::renderBackground(canvas, element);\n        renderSnipasteStyleNumberBadge(canvas, element.numberValue, element.startPt, color,');
    handler = handler.replace('                                       element.fill, effScale, element.numberShape, element.numberFormat);',
        '                                       true, effScale, tools3000::capture::NumberBadgeShape::Circle, element.numberFormat);');
    handler = handler.replace('                                       true, effScale, tools3000::capture::NumberBadgeShape::Circle, element.numberFormat);',
        '                                       true, effScale, tools3000::capture::NumberBadgeShape::Circle, element.numberFormat);\n        winone::number_label::render(canvas, element);');
    text = text.slice(0, begin) + handler + text.slice(end);
    prepared.set(relative, Buffer.from(text));
}
{
    const relative = 'src/capture/CaptureInput.cpp';
    let text = prepared.get(relative).toString('utf8').replace(/\r\n/g, '\n');
    const begin = text.indexOf('        case MarkupTool::Number: {');
    const end = text.indexOf('        case MarkupTool::Magnifier:', begin);
    if (begin < 0 || end < 0) throw Error('Community number creation changed');
    let block = text.slice(begin, end);
    block = block.replace('                elem->isActive = true;',
        '                elem->isActive = true;\n                elem->fill = true;\n                if(!m_state->winoneNumberCreated){m_state->currentColor=MarkupColor::Red();m_state->winoneNumberCreated=true;}\n                elem->color=m_state->currentColor;\n                elem->numberShape = NumberBadgeShape::Circle;\n                elem->isEditing = true;\n                elem->fontSize = std::clamp(static_cast<float>(m_state->currentStrokeWidth), 6.f, 256.f) * currentDpiScale;\n                elem->hasLeaderArrow = false;\n                if (m_state->winoneEditNumber) m_state->winoneEditNumber(elem->id);');
    text = text.slice(0, begin) + block + text.slice(end);
    prepared.set(relative, Buffer.from(text));
}
await adapt('src/capture/MarkupEngine.cpp', [
    ['            maxByFormat[slot] = (std::max)(maxByFormat[slot], elem->numberValue);',
     '            elem->numberValue = ++maxByFormat[slot];'],
    ['    m_nextNumbers = snapshot.nextNumbers;', '    m_nextNumbers = snapshot.nextNumbers;\n    recalculateNextNumbers();'],
]);
// Legacy drafts may contain an explicitly chosen shadow. Reuse the same
// community handler at its saved offset; new annotations remain flat.
await adapt('src/capture/MarkupEngine.h', [
    ['    Wavy = 4,', '    DashDotDot = 5,\n    Wavy = 4,'],
    ['struct MarkupElement {', `struct MarkupElement {
    bool winoneShadow = false;
    std::string winoneLegacyText;
    std::vector<std::pair<std::string, cv::Point>> winoneLegacyLines;
    MarkupColor winoneShadowColor{0, 0, 0, 128};
    cv::Point winoneShadowOffset{0, 0};`],
]);
await adapt('src/capture/MarkupEngine.cpp', [
    ['        a.tool != b.tool ||', '        a.winoneLegacyText != b.winoneLegacyText || a.winoneLegacyLines != b.winoneLegacyLines ||\n        a.winoneShadow != b.winoneShadow || a.winoneShadowColor != b.winoneShadowColor || a.winoneShadowOffset != b.winoneShadowOffset ||\n        a.tool != b.tool ||'],
    ['        if (!handler || targetCanvas.empty()) return;', `        if (!handler || targetCanvas.empty()) return;
        if (element.winoneShadow) {
            auto shadow = element; shadow.winoneShadow = false;
            shadow.moveBy(element.winoneShadowOffset.x, element.winoneShadowOffset.y);
            shadow.color = element.winoneShadowColor; shadow.textOutlineColor = shadow.color;
            renderElement(targetCanvas, shadow, false);
        }`],
    ['void MarkupEngine::renderElement(cv::Mat& canvas, const MarkupElement& element, bool includeActiveHandles) const {', `void MarkupEngine::renderElement(cv::Mat& canvas, const MarkupElement& element, bool includeActiveHandles) const {
    if (element.winoneShadow) {
        auto shadow = element; shadow.winoneShadow = false;
        shadow.moveBy(element.winoneShadowOffset.x, element.winoneShadowOffset.y);
        shadow.color = element.winoneShadowColor; shadow.textOutlineColor = shadow.color;
        renderElement(canvas, shadow, false);
    }`],
]);
await adapt('src/capture/MarkupEngineShapes.cpp', [
    ['    } else if (style == LineStyle::DashDot) {', `    } else if (style == LineStyle::DashDotDot) {
        const float gap = (std::max)(5.0f, 1.6f * t) + t;
        const float dot = (std::max)(1.0f, 0.20f * t);
        pattern = {(std::max)(10.0f, 2.2f * t), gap, dot, gap, dot, gap};
    } else if (style == LineStyle::DashDot) {`],
]);
{
    const relative = 'src/capture/MarkupEngineEffects.cpp';
    let text = prepared.get(relative).toString('utf8');
    const begin = text.indexOf('class TextHandler : public DefaultBoxHandler {');
    const end = text.indexOf('class NumberHandler : public DefaultBoxHandler {', begin);
    const anchor = '    void render(cv::Mat& canvas, const MarkupElement& element) const override {';
    let handler = text.slice(begin,end);
    if(begin < 0 || end < 0 || handler.split(anchor).length !== 2)throw Error('Community text rendering seam changed');
    handler = handler.replace(anchor, anchor + `
        if (!element.isEditing && !element.winoneLegacyLines.empty() && element.text == element.winoneLegacyText) {
            // Preserve saved visual line breaks. Every line is still rendered
            // by this same community text handler, not a replacement engine.
            for (const auto& [content, offset] : element.winoneLegacyLines) {
                auto line = element; line.winoneLegacyLines.clear(); line.text = content;
                line.startPt += offset; line.endPt = line.startPt;
                line.textMaxWidth = 0; line.hasCustomTextWidth = false; line.customTextBoxHeight = 0;
                line.textRenderSize = {}; line.isActive = false; render(canvas, line);
            }
            return;
        }`);
    prepared.set(relative,Buffer.from(text.slice(0,begin)+handler+text.slice(end)));
}
{
    const relative = 'src/capture/RecordingIndicator.cpp';
    let text = prepared.get(relative).toString('utf8').replace(/\r\n/g, '\n');
    text = '#include "recording_bridge.h"\n' + text;
    const statsCalls = text.split('ScreenRecorder::instance().stats()').length - 1;
    const stateCalls = text.split('ScreenRecorder::instance().state()').length - 1;
    if (statsCalls !== 4 || stateCalls !== 3) throw Error('Upstream recording status seam changed');
    text = text.replaceAll('ScreenRecorder::instance().stats()', 'winone::community::recordingStats()');
    text = text.replaceAll('ScreenRecorder::instance().state()', 'winone::community::recordingState()');
    // WinOne records the selected physical pixels directly. Its existing engine
    // has no Studio/countdown stage, so those UI hints are not an active path.
    const begin = text.indexOf('static std::wstring resolveCountdownEffectTip(bool isZh) {');
    const end = text.indexOf('\nLRESULT CALLBACK RecordingIndicator::countdownWndProc', begin);
    if (begin < 0 || end < 0) throw Error('Upstream recording countdown seam changed');
    text = text.slice(0, begin) + 'static std::wstring resolveCountdownEffectTip(bool) { return {}; }\n' + text.slice(end);
    text = text.replace('        ScreenRecorder::instance().skipCountdown();', '        // WinOne starts recording directly; no countdown worker is running.');
    if (text.includes('ScreenRecorder::instance()')) throw Error('Unmapped community recording call');
    prepared.set(relative, Buffer.from(text));
}
{
    const relative = 'src/capture/CaptureToolbarLayout.cpp';
    let text = prepared.get(relative).toString('utf8').replace(/\r\n/g, '\n');
    const begin = text.indexOf('    if (state.mode == OverlayMode::RecordRegion) {');
    const end = text.indexOf('        return specs;\n    }', begin);
    if (begin < 0 || end < 0) throw Error('Upstream recording preparation toolbar changed');
    text = text.slice(0, begin) + `    if (state.mode == OverlayMode::RecordRegion) {
        std::vector<ButtonSpec> specs;
        const bool gif = state.recordSetup.format == RecordFormat::GIF;
        auto option = [&](ToolbarCommand command, std::wstring label, float width) {
            specs.push_back({command, MarkupTool::Rectangle, {}, label, width});
        };
        option(ToolbarCommand::WinOneRecordCodec, gif ? L"GIF" : (state.recordSetup.format == RecordFormat::MP4_H265 ? L"H.265" : L"H.264"), 68);
        option(ToolbarCommand::RecordCycleFps, std::to_wstring(state.recordSetup.fps)+L" fps", 78);
        if (gif) {
            option(ToolbarCommand::WinOneGifQuality, (zh?L"质量 ":L"Quality ")+std::to_wstring(state.winoneGifQuality), zh?85:110);
            option(ToolbarCommand::WinOneGifSeconds, std::to_wstring(state.winoneGifSeconds)+(zh?L" 秒":L" sec"), 78);
        }
        option(ToolbarCommand::WinOneRecordCursor, state.winoneRecordCursor ? (zh?L"鼠标 开":L"Cursor on") : (zh?L"鼠标 关":L"Cursor off"), zh?85:108);
        option(ToolbarCommand::WinOneRecordDestination, zh?L"保存地址":L"Save to…", zh?90:100);
        specs.push_back({ToolbarCommand::Cancel, MarkupTool::Rectangle, {}, L"", 32.0f});
        specs.push_back({ToolbarCommand::RecordStartConfirm, MarkupTool::Rectangle, {},
            zh ? L"开始录制" : L"Start", zh ? 100.0f : 86.0f});
        return specs;
    }` + text.slice(end + '        return specs;\n    }'.length);
    prepared.set(relative, Buffer.from(text));
}
{
    const relative='src/capture/CaptureToolbarLayout.cpp';
    let text=prepared.get(relative).toString('utf8');
    const anchor='    if (state.mode == OverlayMode::RecordRegion) {';
    text=text.replace(anchor,`    if (state.winoneScrollReady) {
        return {{ToolbarCommand::ScrollCapture, MarkupTool::Rectangle, {}, zh?L"开始滚动截图":L"Start scrolling capture", zh?152.f:214.f},
                {ToolbarCommand::Cancel, MarkupTool::Rectangle, {}, zh?L"取消":L"Cancel", 72.f}};
    }
${anchor}`);
    prepared.set(relative,Buffer.from(text));
}
await adapt('src/capture/CaptureToolbarLayout.cpp', [
    ['    const CaptureOptions* options) {', '    const CaptureOptions* options) {\n    if (!button.tooltipHint.empty() && !button.isSecondary) return button.tooltipHint;'],
    ['    float priShrink = (priContentW > maxAvailPriW && priContentW > 0.0f) ? (maxAvailPriW / priContentW) : 1.0f;\n    if (priShrink < 1.0f) {\n        priContentW *= priShrink;\n        priPanelW = priContentW + 2.0f * paddingX;\n    }', `    // Keep native hit targets and glyphs legible at high DPI. Wrap the
    // existing community button specifications instead of squeezing a row.
    const float priShrink = 1.0f;
    int primaryRows = 1;
    float rowWidth = gripW + gripGap, widestRow = rowWidth;
    for (const auto& sp : priSpecs) {
        const float next = (sp.isSeparatorBefore ? sepWidth : 0.f) + sp.baseWidth * scale + gapX;
        if (rowWidth + next - gapX > maxAvailPriW && rowWidth > 0) {
            widestRow = std::max(widestRow, rowWidth - gapX); rowWidth = 0; ++primaryRows;
        }
        rowWidth += next;
    }
    widestRow = std::max(widestRow, rowWidth - gapX);
    priPanelW = widestRow + 2.f * paddingX;
    priPanelH = primaryRows * buttonHeight + (primaryRows - 1) * gapX + 2.f * paddingY;`],
    ['        float w = sp.baseWidth * scale * priShrink;', `        float w = sp.baseWidth * scale * priShrink;
        if (curX + w > state.primaryToolbarRect.right - paddingX + .5f) {
            curX = priPanelX + paddingX;
            curY += buttonHeight + gapX;
        }`],
]);
await adapt('src/capture/CaptureInputKeyboard.cpp', [
    ['        case WM_KEYDOWN: {', `        case WM_KEYDOWN: {
            if (wParam == VK_TAB && !m_state->winoneTranslated.empty()) {
                m_state->winoneShowTranslated = !m_state->winoneShowTranslated;
                m_renderer->markMarkupDirty(); m_renderer->invalidate(); return 0;
            }`],
]);
await adapt('src/capture/PinWindowTranslate.cpp', [
    ['#include "capture/PinWindowInternal.h"', '#include "capture/PinWindowInternal.h"\n#include "core/config/ConfigManager.h"'],
    ['void PinWindow::toggleTranslationOverlay() {', `void PinWindow::toggleTranslationOverlay() {
    if (!tools3000::core::ConfigManager::instance().get<bool>("/capture/pinTranslation", false)) {
        clearTranslationState();
        showTransientHudBadge(tools3000::core::trWCstr(L"请先开启截图与贴图原位翻译", L"Enable in-place screenshot translation first"), 3000);
        return;
    }`],
    ['void PinWindow::translateInPlace(bool selectedOnly, bool forceRetranslate) {', `void PinWindow::translateInPlace(bool selectedOnly, bool forceRetranslate) {
    if (!tools3000::core::ConfigManager::instance().get<bool>("/capture/pinTranslation", false)) {
        clearTranslationState();
        return;
    }`],
]);
{
    const relative = 'src/capture/CaptureRendererHud.cpp';
    let text = prepared.get(relative).toString('utf8');
    const begin = text.indexOf('void CaptureRenderer::render(CaptureState& state) {');
    if (begin < 0) throw Error('Community capture rendering entry changed');
    let body = text.slice(begin);
    const anchor = '    applyThemeColors();';
    if (body.split(anchor).length !== 2) throw Error('Community capture render theme anchor changed');
    body = body.replace(anchor, `    const auto drawWinOneProgress = [&] {
        if (state.winoneProgress < 0 || state.winoneProgressText.empty() || !m_infoTextFormat) return;
        const auto size = m_renderTarget->GetSize();
        const float width = (std::min)(400.f, size.width - 32.f);
        const float x = (std::max)(16.f, ((std::min)(state.dragStart.x, state.dragEnd.x) + (std::max)(state.dragStart.x, state.dragEnd.x) - width) * .5f);
        const float y = (std::min)(size.height - 68.f, static_cast<float>((std::max)(state.dragStart.y, state.dragEnd.y)) + 12.f);
        m_renderTarget->FillRoundedRectangle(D2D1::RoundedRect(D2D1::RectF(x, y, x + width, y + 48), 6, 6), m_infoBgBrush.Get());
        m_renderTarget->DrawTextW(state.winoneProgressText.c_str(), static_cast<UINT32>(state.winoneProgressText.size()),
            m_infoTextFormat.Get(), D2D1::RectF(x + 12, y + 4, x + width - 12, y + 31), m_infoTextBrush.Get());
        m_renderTarget->FillRectangle(D2D1::RectF(x + 12, y + 36, x + width - 12, y + 40), m_dimBrush.Get());
        m_renderTarget->FillRectangle(D2D1::RectF(x + 12, y + 36, x + 12 + (width - 24) * std::clamp(state.winoneProgress, 0.f, 1.f), y + 40), m_borderBrush.Get());
    };
${anchor}`);
    body = body.replaceAll('recoverFromDeviceLoss(m_renderTarget->EndDraw());', 'drawWinOneProgress();\n        recoverFromDeviceLoss(m_renderTarget->EndDraw());');
    prepared.set(relative, Buffer.from(text.slice(0, begin) + body));
}
await adapt('src/capture/PinWindow.h', [
    ['    static void refreshAllVisualsAndGroupBar();', `    static void refreshAllVisualsAndGroupBar();
    static void winoneResetTranslations() {
        for (const auto& pin : s_instances) if (pin) { pin->clearTranslationState(); pin->createRenderResources(pin->activeDisplayImage()); pin->render(); }
    }`],
]);
{
    const relative='src/capture/PinWindow.cpp';
    let text=prepared.get(relative).toString('utf8');
    const begin=text.indexOf('bool savePinnedImage(HWND owner, const cv::Mat& image) {');
    const end=text.indexOf('\n}\n',begin);
    if(begin<0||end<0)throw Error('Community pin save boundary changed');
    text='#include "community_hooks.h"\n'+text.slice(0,begin)+`bool savePinnedImage(HWND owner, const cv::Mat& image) {
    return winone::community::endpoint<bool(*)(HWND,const void*)>("WinOneNativeSavePin")(owner,&image);
}`+text.slice(end+2);
    prepared.set(relative,Buffer.from(text));
}
await adapt('src/capture/RecordingIndicator.h', [
    ['    HWND m_hwnd = nullptr;', '    bool m_winonePinned = true;\n    HWND m_hwnd = nullptr;'],
    ['    void onSnapshot(IndicatorAction action) { m_onSnapshot = std::move(action); }', `    void onSnapshot(IndicatorAction action) { m_onSnapshot = std::move(action); }
    HWND winoneWindow() const { return m_hwnd; }
    bool winonePinned() const { return m_winonePinned; }
    POINT winoneControlCenter(int index) const {
        const RECT r = index == 0 ? m_snapshotBtn : index == 1 ? m_pauseBtn : m_stopBtn;
        return {static_cast<LONG>((r.left+r.right)*.5f*m_dpiScale), static_cast<LONG>((r.top+r.bottom)*.5f*m_dpiScale)};
    }
    void toggleWinOnePinned() {
        m_winonePinned = !m_winonePinned;
        const auto style = GetWindowLongPtrW(m_hwnd, GWL_EXSTYLE);
        SetWindowLongPtrW(m_hwnd, GWL_EXSTYLE, m_winonePinned ? style | WS_EX_NOACTIVATE : style & ~WS_EX_NOACTIVATE);
        SetWindowPos(m_hwnd, m_winonePinned ? HWND_TOPMOST : HWND_NOTOPMOST, 0, 0, 0, 0, SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE);
        if (!m_winonePinned) SetForegroundWindow(m_hwnd);
        InvalidateRect(m_hwnd, nullptr, FALSE);
    }`],
]);
await adapt('src/capture/RecordingIndicator.cpp', [
    ['#include "recording_bridge.h"', '#include "recording_bridge.h"\n#include "toolbar_icons.h"'],
    ['L"抓拍高清截图", L"Take Snapshot"', 'L"固定录制控制栏", L"Pin recording controls"'],
    ['m_hoverSnapshot, false, false, false, 1.0f, isDark);', 'm_hoverSnapshot, m_winonePinned, false, false, 1.0f, isDark);'],
    ['            CaptureVectorIcons::renderIcon(m_renderTarget.Get(), m_d2dFactory.Get(), CaptureIconId::ActionSnapshot, snapIconRc, m_textBrush.Get(), 1.0f);', `            D2D1_MATRIX_3X2_F transform{}; m_renderTarget->GetTransform(&transform);
            if (m_winonePinned) m_renderTarget->SetTransform(D2D1::Matrix3x2F::Rotation(45,
                D2D1::Point2F((snapIconRc.left + snapIconRc.right) / 2, (snapIconRc.top + snapIconRc.bottom) / 2)) * transform);
            winone::icons::render(m_renderTarget.Get(), "pin", isDark, snapIconRc);
            m_renderTarget->SetTransform(transform);`],
]);
await adapt('src/capture/RecordingIndicator.cpp', [
    ['    auto hwndProps = D2D1::HwndRenderTargetProperties(', `    if(GetProcAddress(GetModuleHandleW(nullptr),"WinOneNativeRenderedFrame"))rtProps.usage=D2D1_RENDER_TARGET_USAGE_GDI_COMPATIBLE;
    auto hwndProps = D2D1::HwndRenderTargetProperties(`],
    ['    const HRESULT hrEnd = m_renderTarget->EndDraw();', `    if(auto readback=reinterpret_cast<void(*)(void*)>(GetProcAddress(GetModuleHandleW(nullptr),"WinOneNativeRenderedFrame")))readback(m_renderTarget.Get());
    const HRESULT hrEnd = m_renderTarget->EndDraw();`],
    ['out.pauseBtn = {out.snapshotBtn.right + 5, btnY, out.snapshotBtn.right + 5 + BTN_SIZE, btnY + BTN_SIZE};',
     'out.pauseBtn = {out.snapshotBtn.right + 5, btnY, out.snapshotBtn.right + 5 + 76, btnY + BTN_SIZE};'],
    ['out.stopBtn = {out.pauseBtn.right + 5, btnY, out.pauseBtn.right + 5 + BTN_SIZE, btnY + BTN_SIZE};',
     'out.stopBtn = {out.pauseBtn.right + 5, btnY, out.pauseBtn.right + 5 + 76, btnY + BTN_SIZE};'],
    ['            CaptureVectorIcons::renderIcon(m_renderTarget.Get(), m_d2dFactory.Get(),\n                m_paused ? CaptureIconId::ActionRecordStart : CaptureIconId::ActionRecordPause,\n                pIconRc, m_textBrush.Get(), 1.0f);',
     '            const auto label=m_paused?(isZh?L"继续":L"Resume"):(isZh?L"暂停":L"Pause");\n            m_renderTarget->DrawTextW(label,static_cast<UINT32>(wcslen(label)),m_textFormat.Get(),pIconRc,m_textBrush.Get());'],
    ['        CaptureVectorIcons::renderIcon(m_renderTarget.Get(), m_d2dFactory.Get(),\n            CaptureIconId::ActionRecordStop, sIconRc,\n            stopIconBrush ? stopIconBrush.Get() : m_textBrush.Get(), 1.0f);',
     '        const auto stopLabel=isZh?L"结束":L"Finish";\n        m_renderTarget->DrawTextW(stopLabel,static_cast<UINT32>(wcslen(stopLabel)),m_textFormat.Get(),sIconRc,stopIconBrush ? stopIconBrush.Get() : m_textBrush.Get());'],
]);
{
    const relative = 'src/capture/RecordingIndicator.cpp';
    let text = prepared.get(relative).toString('utf8');
    const anchor = 'LRESULT CALLBACK RecordingIndicator::indicatorWndProc(HWND hwnd, UINT msg, WPARAM wParam, LPARAM lParam) {\n    auto* self = reinterpret_cast<RecordingIndicator*>(GetWindowLongPtrW(hwnd, GWLP_USERDATA));';
    if (text.split(anchor).length !== 2) throw Error('Community recording window procedure changed');
    text = text.replace(anchor, anchor + `
    if (self && msg == WM_ACTIVATE && LOWORD(wParam) == WA_INACTIVE && !self->m_winonePinned && !self->m_saving && !self->m_completed) {
        winone::community::endpoint<void(*)(int)>("WinOneNativeRecordingAction")(2);
        return 0;
    }
    if (self && msg == WM_CLOSE) {
        if (self->m_completed) self->hide();
        else winone::community::endpoint<void(*)(int)>("WinOneNativeRecordingAction")(2);
        return 0;
    }
    if (self && msg == WM_RBUTTONUP && !self->m_saving) {
        const bool zh = isChineseLanguagePreferred();
        HMENU menu = CreatePopupMenu();
        AppendMenuW(menu, MF_STRING | (self->m_winonePinned ? MF_CHECKED : 0), 1, zh ? L"置顶控制栏" : L"Keep controls on top");
        if (!self->m_completed) {
            AppendMenuW(menu, MF_STRING, 2, zh ? L"停止并保存" : L"Stop and save");
            AppendMenuW(menu, MF_STRING, 3, zh ? L"取消录制" : L"Discard recording");
        }
        POINT cursor{}; GetCursorPos(&cursor);
        const auto selected = TrackPopupMenu(menu, TPM_RETURNCMD | TPM_RIGHTBUTTON, cursor.x, cursor.y, 0, hwnd, nullptr);
        DestroyMenu(menu);
        if (selected == 1) {
            self->toggleWinOnePinned();
        } else if (selected == 2 || selected == 3) winone::community::endpoint<void(*)(int)>("WinOneNativeRecordingAction")(selected == 3 ? 1 : 2);
        return 0;
    }`);
    prepared.set(relative, Buffer.from(text));
}
await adapt('src/capture/FloatingGlassBar.cpp', [
    // HUD foreground and background must stay one pair. The shared palette is
    // also used by MyGo and the native common-control tooltip.
    ['    return resolveThemePalette(true);', `    auto palette = resolveThemePalette(true);
    palette.panelBg = winone::hint::background();
    palette.primaryText = palette.textPrimary = winone::hint::foreground();
    palette.panelBorder = winone::hint::border();
    return palette;`],
    ['#include "capture/FloatingGlassBar.h"', '#include "capture/FloatingGlassBar.h"\n#include "hint_style.h"'],
    ['    p.accentSubtle = p.accentSoftBg;', `    // Use the existing WinOne neutral palette; the community layout and
    // rendering primitives stay intact.
    p.panelBg = D2D1::ColorF(isDark ? 0x242423 : 0xffffff, 1);
    p.panelHoverBg = D2D1::ColorF(isDark ? 0x333331 : 0xe8e8e5, 1);
    p.panelBorder = D2D1::ColorF(isDark ? 0x454541 : 0xcececa, 1);
    p.panelHoverBorder = p.panelBorder;
    p.primaryText = D2D1::ColorF(isDark ? 0xe7e7e2 : 0x292927, 1);
    p.secondaryText = p.mutedText = D2D1::ColorF(isDark ? 0xaaa9a3 : 0x60605c, 1);
    p.hoverBg = p.panelHoverBg;
    p.pressBg = D2D1::ColorF(isDark ? 0x3b3b38 : 0xdededa, 1);
    p.accent = p.accentText = D2D1::ColorF(isDark ? 0xe4e4e0 : 0x30302e, 1);
    p.accentSoftBg = p.accentHoverBg = D2D1::ColorF(isDark ? 0x343432 : 0xe4e4e0, 1);
    p.accentBorder = p.panelBorder;
    p.chipBg = D2D1::ColorF(isDark ? 0x292928 : 0xf0f0ee, 1);
    p.chipHoverBg = p.hoverBg; p.chipText = p.primaryText; p.chipBorder = p.panelBorder;
    p.accentSubtle = p.accentSoftBg;`],
    ['        renderShadow(ambientSteps, sizeof(ambientSteps) / sizeof(ambientSteps[0]));\n        renderShadow(keySteps, sizeof(keySteps) / sizeof(keySteps[0]));',
     '        renderShadow(keySteps, 1);'],
]);
// A minimap viewport is an overlay on the image, not a selected button face.
await adapt('src/capture/ScrollCaptureOverlay.h', [
    ['        int compactLevel = 0;', '        for(auto& spec:specs)spec.keycap.clear(); // Local shortcuts are listed in Settings.\n        int compactLevel = 0;'],
]);
await adapt('src/capture/ScrollCaptureOverlay.cpp', [
    ['L"已停滚 · 回车裁切/完成" : L"Stopped · Enter to trim"', 'L"已停滚" : L"Stopped"'],
    ['L"已暂停 · 回车裁切/完成" : L"Paused · Enter to trim"', 'L"已暂停" : L"Paused"'],
]);
{
    const actions=JSON.parse(await fs.readFile(path.join(root,'internal/captureprefs/shortcuts.json'),'utf8'));
    const rows=actions.map(a=>`{"${a.id}","${a.key}",ToolbarCommand::${a.command},MarkupTool::${a.tool||'Rectangle'}}`).join(',\n');
    prepared.set('winone/canvas_shortcuts.h',Buffer.from(`#pragma once
#include "capture/CaptureState.h"
#include "core/config/ConfigManager.h"
#include <array>
namespace winone::capture {
using namespace tools3000::capture;
struct CanvasBinding {const char* id;const char* key;ToolbarCommand command;MarkupTool tool;};
inline constexpr std::array<CanvasBinding,${actions.length}> bindings{{${rows}}};
inline std::string chord(WPARAM key,bool ctrl,bool shift,bool alt) {
    std::string name;
    if((key>='A'&&key<='Z')||(key>='0'&&key<='9'))name=static_cast<char>(key);
    else if(key>=VK_F1&&key<=VK_F24)name="F"+std::to_string(key-VK_F1+1);
    else if(key>=VK_NUMPAD0&&key<=VK_NUMPAD9)name="Num"+std::to_string(key-VK_NUMPAD0);
    else return {};
    return std::string(ctrl?"Ctrl+":"")+(alt?"Alt+":"")+(shift?"Shift+":"")+name;
}
inline std::pair<bool,ToolbarButton> canvasKey(const CaptureState& state,WPARAM key,bool ctrl,bool shift,bool alt) {
    ToolbarButton result;
    result.command=ToolbarCommand::WinOneNoop;
    if(state.mode!=OverlayMode::Screenshot || (state.state.load()!=OverlayState::Selected && state.state.load()!=OverlayState::Marking) ||
       (state.activeElement&&state.activeElement->isEditing) || state.dropdownMenu.type!=DropdownType::None) return {false,result};
    const auto pressed=chord(key,ctrl,shift,alt);
    const auto values=tools3000::core::ConfigManager::instance().get<nlohmann::json>("/capture/winoneShortcuts",nlohmann::json::object());
    bool managed=false;
    for(const auto& binding:bindings) {
        if(pressed==binding.key)managed=true;
        const std::string configured=values.is_object()?values.value(binding.id,std::string(binding.key)):binding.key;
        if(!configured.empty() && pressed==configured){result.command=binding.command;result.tool=binding.tool;return {true,result};}
    }
    return {managed,result};
}
}
`));
}
await adapt('src/capture/CaptureState.h', [['    WinOneRecordCodec,','    WinOneNoop, WinOneRecordCodec,']]);
await adapt('src/capture/CaptureInputKeyboard.cpp', [
    ['#include "capture/CaptureInputInternal.h"', '#include "capture/CaptureInputInternal.h"\n#include "canvas_shortcuts.h"'],
    ['            const auto decision = CaptureShortcutPolicy::resolveKeyDown(keyCtx, wParam, ctrl, shift, alt);', `            const auto custom = winone::capture::canvasKey(*m_state,wParam,ctrl,shift,alt);
            if (!keyCtx.editingText && custom.first) {
                if(!isAutoRepeat && custom.second.command!=ToolbarCommand::WinOneNoop)executeToolbarCommand(custom.second);
                return 0;
            }
            const auto decision = CaptureShortcutPolicy::resolveKeyDown(keyCtx, wParam, ctrl, shift, alt);`],
]);
await adapt('src/capture/CaptureInput.cpp', [
    ['void CaptureInput::setCurrentTool(MarkupTool tool) {', `void CaptureInput::setCurrentTool(MarkupTool tool) {
    if(tool==MarkupTool::Number && !m_state->winoneNumberStyleInitialized) {
        m_state->currentColor=MarkupColor::Red();m_state->winoneNumberStyleInitialized=true;
    }`],
]);
await adapt('src/capture/CaptureInput.h', [
    ['    void endDetectionSession();', `    void endDetectionSession();
    POINT winoneSnapPoint(POINT point) {
        if(!m_state || m_state->isLongImageMode() || (GetKeyState(VK_MENU)&0x8000) ||
            m_state->aspectRatio != AspectRatioPreset::Free) return point;
        const POINT screen{point.x+GetSystemMetrics(SM_XVIRTUALSCREEN),point.y+GetSystemMetrics(SM_YVIRTUALSCREEN)};
        auto candidates=m_elementDetector.querySync(screen);
        std::vector<RECT> edges; for(const auto& candidate:candidates)edges.push_back(candidate.rect);
        for(const auto& rect:m_state->detectedWindowHierarchy)edges.push_back(rect);
        return winone::capture::snapPoint(point,edges,static_cast<int>(8*std::max(1.f,m_state->dpiScale)));
    }`],
    ['#include "capture/CaptureState.h"', '#include "capture/CaptureState.h"\n#include "selection_snap.h"'],
]);
await adapt('src/capture/CaptureInputMouse.cpp', [
    ['                    m_state->dragEnd = m_state->currentCursor;', '                    m_state->dragEnd = winoneSnapPoint(m_state->currentCursor);'],
    ['            m_state->dragEnd = {GET_X_LPARAM(lParam), GET_Y_LPARAM(lParam)};', '            m_state->dragEnd = winoneSnapPoint({GET_X_LPARAM(lParam), GET_Y_LPARAM(lParam)});'],
    ['                    int dx = m_state->currentCursor.x - m_state->selAdjustLast.x;\n                    int dy = m_state->currentCursor.y - m_state->selAdjustLast.y;\n                    adjustSelection(m_state->selAdjustHandle, dx, dy);', `                    const auto snapped = winoneSnapPoint(m_state->currentCursor);
                    int dx = snapped.x - m_state->selAdjustLast.x;
                    int dy = snapped.y - m_state->selAdjustLast.y;
                    adjustSelection(m_state->selAdjustHandle, dx, dy);
                    m_state->selAdjustLast = snapped;
                    m_renderer->invalidate(); return 0;`],
]);
await adapt('src/capture/CaptureRendererHud.cpp', [
    ['    m_renderTarget->CreateSolidColorBrush(pal.accentSoftBg, viewFillBr.GetAddressOf());',
     '    auto viewportTint = pal.accent; viewportTint.a = 0.18f;\n    m_renderTarget->CreateSolidColorBrush(viewportTint, viewFillBr.GetAddressOf());'],
    ['L"头部 Home"', 'L"头部"'],
    ['L"尾部 End"', 'L"尾部"'],
]);
{
    const relative='src/capture/CaptureRendererHud.cpp';
    let text=prepared.get(relative).toString('utf8');
    const begin=text.indexOf('void CaptureRenderer::drawLongCanvasMinimap(');
    const end=text.indexOf('void CaptureRenderer::renderLongImageStudio(',begin);
    let body=text.slice(begin,end);
    body=body.replace('m_renderTarget->CreateSolidColorBrush(pal.panelBg, panelBg.GetAddressOf());',
        'auto translucentPanel=pal.panelBg;translucentPanel.a=0.16f;\n    m_renderTarget->CreateSolidColorBrush(translucentPanel, panelBg.GetAddressOf());');
    text=text.slice(0,begin)+body+text.slice(end);prepared.set(relative,Buffer.from(text));
}
await adapt('src/capture/CaptureInputCommands.cpp', [
    ['                transitionToRecordMode(*m_state);', '                if (m_state->winonePrepareRecording && !m_state->winonePrepareRecording(false)) return;\n                transitionToRecordMode(*m_state);'],
]);
{
    const relative='src/capture/CaptureDropdownMenu.cpp';
    let text=prepared.get(relative).toString('utf8');
    const begin=text.indexOf('void openDropdownMenu(CaptureState& state,');
    const commit=text.indexOf('void commitDropdownSelection(CaptureState& state,');
    const anchor='        case DropdownType::BeautyShellBg: {';
    let open=text.slice(begin,commit);
    open=open.replace(anchor,`        case DropdownType::WinOneToolbar: {
            const std::array<const char*,13> ids{"crop","select","rectangle","ellipse","arrow","pen","cover","text","step","mosaic","blur","ocr","translate"};
            const std::array<const wchar_t*,13> zh{L"调整选区",L"移动标注",L"矩形",L"椭圆",L"箭头",L"画笔",L"遮盖",L"文字",L"序号说明",L"马赛克",L"模糊",L"识别文字",L"翻译"};
            const std::array<const wchar_t*,13> en{L"Adjust selection",L"Move annotation",L"Rectangle",L"Ellipse",L"Arrow",L"Pen",L"Cover",L"Text",L"Numbered step",L"Mosaic",L"Blur",L"Recognize text",L"Translate"};
            const auto selected=tools3000::core::ConfigManager::instance().get<nlohmann::json>("/capture/winoneToolbar",nullptr);
            for(size_t i=0;i<ids.size();++i) state.dropdownMenu.items.push_back({static_cast<int>(i),isZh?zh[i]:en[i],L"",
                selected.is_null() || std::find(selected.begin(),selected.end(),ids[i])!=selected.end()});
            state.dropdownMenu.items.push_back({99,isZh?L"恢复默认":L"Reset to defaults"});
            break;
        }
        case DropdownType::WinOneCodec:
            state.dropdownMenu.items={{0,L"H.264",L"",state.recordSetup.format==RecordFormat::MP4_H264},{1,L"H.265",L"",state.recordSetup.format==RecordFormat::MP4_H265}};break;
        case DropdownType::WinOneQuality:
            for(int quality:{50,65,75,85,95,100})state.dropdownMenu.items.push_back({quality,std::to_wstring(quality),L"",state.winoneGifQuality==quality});break;
        case DropdownType::WinOneSeconds:
            for(int seconds:{1,3,15,30,60,120,300,600})state.dropdownMenu.items.push_back({seconds,std::to_wstring(seconds)+(isZh?L" 秒":L" sec"),L"",state.winoneGifSeconds==seconds});break;
${anchor}`);
    let rest=text.slice(commit);
    rest=rest.replace(anchor,`        case DropdownType::WinOneToolbar:
            if(state.winoneCustomize)state.winoneCustomize(id);break;
        case DropdownType::WinOneCodec:
            state.recordSetup.format=id==1?RecordFormat::MP4_H265:RecordFormat::MP4_H264;break;
        case DropdownType::WinOneQuality: state.winoneGifQuality=id;break;
        case DropdownType::WinOneSeconds: state.winoneGifSeconds=id;break;
${anchor}`);
    text=text.slice(0,begin)+open+rest;
    // GIF's frame-rate ceiling is visible in the options, never silently clamped.
    text=text.replace('        case DropdownType::RecordQuality: {',`        case DropdownType::RecordQuality: {`);
    const fpsBegin=text.indexOf('        case DropdownType::RecordFps: {');
    const fpsEnd=text.indexOf('        case DropdownType::RecordQuality: {',fpsBegin);
    let fps=text.slice(fpsBegin,fpsEnd);
    fps=fps.replace('            break;', '            if(state.recordSetup.format==RecordFormat::GIF)state.dropdownMenu.items.pop_back();\n            break;');
    text=text.slice(0,fpsBegin)+fps+text.slice(fpsEnd);
    prepared.set(relative,Buffer.from(text));
}
await adapt('src/capture/CaptureRendererToolbar.cpp', [
    ['            float labelW = 0.0f;', '            kbdPart.clear(); // Canvas shortcuts live in WinOne settings, not hover hints.\n            float labelW = 0.0f;'],
    ['            FloatingGlassBar::drawGlassPanel(m_renderTarget.Get(), tipRect, 7.5f * scale, true);',
     '            winone::hint::draw(m_renderTarget.Get(), tipRect, scale);'],
    ['#include "capture/CaptureRendererInternal.h"', '#include "capture/CaptureRendererInternal.h"\n#include "hint_style.h"'],
]);
{
    const relative='src/capture/PinWindowRender.cpp';
    let text=prepared.get(relative).toString('utf8');
    text='#include "hint_style.h"\n'+text;
    // Keep the community placement/animation; use the same popup surface.
    text=text.replace(/FloatingGlassBar::drawGlassPanel\(m_renderTarget.Get\(\), tipRect, ([^;]+?), true\);/g,
        'winone::hint::draw(m_renderTarget.Get(), tipRect, dpiScale);');
    prepared.set(relative,Buffer.from(text));
}
{
    const style=JSON.parse(await fs.readFile(path.join(root,'modules/nativeui/hint/style.json'),'utf8'));
    const hex=value=>'0x'+value.slice(1);
    prepared.set('winone/hint_style.h',Buffer.from(`#pragma once
#include <d2d1.h>
#include <wrl/client.h>
namespace winone::hint {
inline D2D1_COLOR_F background(){return D2D1::ColorF(${hex(style.colors.background)},1);}
inline D2D1_COLOR_F foreground(){return D2D1::ColorF(${hex(style.colors.foreground)},1);}
inline D2D1_COLOR_F border(){return D2D1::ColorF(${hex(style.colors.border)},1);}
inline void draw(ID2D1RenderTarget* target,const D2D1_RECT_F& rect,float scale) {
    Microsoft::WRL::ComPtr<ID2D1SolidColorBrush> fill,line;
    if(FAILED(target->CreateSolidColorBrush(background(),fill.GetAddressOf())) ||
       FAILED(target->CreateSolidColorBrush(border(),line.GetAddressOf()))) return;
    const auto shape=D2D1::RoundedRect(rect,${style.radius}.f*scale,${style.radius}.f*scale);
    target->FillRoundedRectangle(shape,fill.Get());target->DrawRoundedRectangle(shape,line.Get(),scale);
}
}
`));
}
for (const [relative, data] of prepared) {
    const target = path.join(output, relative);
    let previous;
    try { previous = await fs.readFile(target); } catch (error) { if (error.code !== 'ENOENT') throw error; }
    if (previous?.equals(data)) continue;
    await fs.mkdir(path.dirname(target), {recursive: true});
    await fs.writeFile(target, data);
}
// Reuse the exact Solar bitmap assets generated for the existing capture UI.
const iconNames = ['select','region','cover','overflow','gif','saveAs','undo','redo','pin','save','confirm','cancel','ocr','translate','scroll','video','rectangle','ellipse','arrow','pen','text','step','mosaic','blur'];
const iconRoot = path.join(root,'modules/capture/NativeUI/Assets/Icons');
const generated = path.join(output,'winone');
await fs.mkdir(generated,{recursive:true});
const resourceLines = ['#include <windows.h>'];
const headerLines = ['#pragma once','#include <string_view>','namespace winone::icons { struct Resource { std::string_view name; int id; }; inline constexpr Resource resources[] = {'];
for (const [index,name] of iconNames.entries()) {
    const id = 200 + index*2;
    headerLines.push(`{"${name}",${id}},`);
    for (const [offset,suffix] of ['', '-dark'].entries()) {
        const file = path.join(iconRoot,`${name}${suffix}-48.png`);
        await fs.access(file);
        resourceLines.push(`${id+offset} RCDATA "${file.replaceAll('\\','/')}"`);
    }
}
// The existing toolbar uses the confirmation glyph for copying.
headerLines.push(`{"copy",${200+iconNames.indexOf('confirm')*2}},`);
headerLines.push('}; }');
resourceLines.push(`1 ICON "${path.join(root,'internal/branding/icon.ico').replaceAll('\\','/')}"`);
for (const [name,content] of [['winone_icons.rc',resourceLines.join('\n')+'\n'],['winone_icon_resources.h',headerLines.join('\n')+'\n']]) {
    const destination=path.join(generated,name);
    let old='';try{old=await fs.readFile(destination,'utf8');}catch(error){if(error.code!=='ENOENT')throw error;}
    if(old!==content)await fs.writeFile(destination,content,'utf8');
}
await fs.writeFile(path.join(output, 'winone-source.json'), JSON.stringify({version: lock.version, commit: lock.commit,
    purpose: 'WinOne native capture source with verified host integration seams'}, null, 2) + '\n');
console.log(`Prepared ${lock.files.length} verified community files with WinOne host seams`);
