// Exercise the production App, including its navigation and save guards. Every
// native boundary below is in-memory: no user configuration, hooks, files,
// sign-in, Explorer integration or startup registration is changed.
import type {Settings} from "../src/bridge";
import {interconnectFixture} from './interconnect-fixture';
import type {AIConfiguration} from "../src/features/ai-configuration";
import type {DirectoryEntry} from "../src/features/directory-links-browser";
import type {DirectoryLinkRecord} from "../src/features/directory-links";
import strokeDefaults from "../../../../modules/stroke/defaults.json";
import {usageFixture} from "./ai-usage-fixture";

const query = new URLSearchParams(location.search);
const language = query.get("language") === "en-US" ? "en-US" : "zh-CN";
const theme = query.get("theme") === "dark" ? "dark" : "light";
const calls: {method: string; params?: unknown}[] = [];
const globalSettings: Settings = {
  schemaVersion: 1, revision: 1, theme, language,
  defaultPage: query.get("page") || "settings",
  openShortcut: "Alt+A", captureShortcut: "Shift+A", ocrShortcut: "Shift+C",
  translateShortcut: "Alt+Shift+A", selectionShortcut: "Alt+Shift+S",
  quickAssistantShortcut: "Ctrl+Shift+Q", pinShortcut: "Alt+Q",
  startup: {runAtLogon: false, startMinimized: true, keepUIRunning: true, preferAdministrator: false},
};
const moduleNames = [
  ["capture", "截图助手", "Capture assistant"],
  ["selection", "划词助手", "Selection assistant"],
  ["quick-assistant", "快捷助手", "Quick assistant"],
  ["wintray", "程序管理", "Program management"],
  ["always-on-top", "置顶功能", "Always on Top"],
  ["stroke", "鼠标手势", "Mouse gestures"],
];
const modules = moduleNames.map(([id, zh, en]) => ({
  manifest: {id, name: {"zh-CN": zh, "en-US": en}, distribution: "bundled"},
  enabled: true, pendingStop: false, runtime: {id, name: language === "zh-CN" ? zh : en, state: "ready", detail: ""},
}));
let capture = {schemaVersion: 1, revision: 1, outputDirectory: "C:\\WinOne-test\\Pictures", copyToClipboard: true, askWhereToSave: false, openEditor: true, pinAfterCapture: false, theme: "system", scrollDelay: 300, scrollFromTop: false, fileNameTemplate: ""};
let aot = {schemaVersion: 1, revision: 1, frame: true, frameThickness: 3, frameColor: "#0078d4", frameOpacity: 90, useAccent: true, roundCorners: true, blockInGameMode: false, sound: false, excludedApps: [] as string[]};
const defaultActions = [
  {id: "copy", name: "复制", englishName: "Copy", icon: "copy", kind: "copy", enabled: true},
  {id: "translate", name: "翻译", englishName: "Translate", icon: "translate", kind: "translate", prompt: "翻译选中的文字。", enabled: true},
  {id: "explain", name: "解释", englishName: "Explain", icon: "explain", kind: "ai", prompt: "解释选中的文字。", enabled: true},
  {id: "summarize", name: "总结", englishName: "Summarize", icon: "summarize", kind: "ai", prompt: "总结选中的文字。", enabled: true},
  {id: "polish", name: "润色", englishName: "Polish", icon: "polish", kind: "ai", prompt: "润色选中的文字。", enabled: true},
  {id: "code", name: "代码", englishName: "Code", icon: "code", kind: "ai", prompt: "解释选中的代码。", enabled: true},
];
let selection = {schemaVersion: 1, revision: 1, automatic: true, keyboard: true, excludedApps: ["notepad.exe"], actions: structuredClone(defaultActions)};
let quick = {...selection, alwaysOnTop: false, showReasoning: true, actions: structuredClone(defaultActions.filter(action => action.kind !== "copy"))};
let stroke = structuredClone(strokeDefaults);
let programs = {schemaVersion: 3, language, runAtLogon: true, startMinimizedToTray: true, exitAfterManagedAppsCompleted: false, closeWindowRetrySeconds: 10, startupIntervalSeconds: 3, managedApps: ["浏览器", "音乐播放器", "文件同步"].map((name, index) => ({
  id: String(index), name: language === "en-US" ? ["Browser", "Music player", "File sync"][index] : name,
  exePath: `C:\\WinOne-test\\Programs\\example${index}.exe`, args: "", runOnStartup: index !== 1,
  collectTrayIcon: index === 2, launchViaLogonTask: false, launchHiddenInBackground: false,
  trayBehavior: {autoMinimizeAndHideOnLaunch: true, closeDelaySeconds: 3},
  schedule: {enabled: index === 0, startDelayMinutes: 5, autoExitMinutes: 30, frequencyEnabled: false, frequencyDays: 1, frequencyRuns: 1},
}))};
if (query.has("empty-programs")) programs.managedApps = [];
let ai: AIConfiguration = {revision: 1, engine: "google", ocrEngine: "local", target: "auto", prompt: "", providers: [
  {id: "one", name: language === "en-US" ? "Example service" : "示例服务", authMode: "api-key", icon: "deepseek-color", protocol: "chat", baseUrl: "https://example.invalid/v1", hasKey: true, models: ["shared-model", "vision-model"], availableModels: ["shared-model", "vision-model", "unchecked-model"]},
  {id: "account", name: "ChatGPT / Codex", authMode: "account", icon: "openai", agent: "codex", protocol: "chat", hasKey: false, models: ["gpt-5.5"]},
], tasks: {default: {provider: "one", model: "shared-model"}}, magpieSettings: {noStats: true}};
if (query.has("long")) {
  capture.outputDirectory = "C:\\WinOne-test\\" + "包含较长名称的截图保存目录\\".repeat(8);
  programs.managedApps[0].name = "LongApplicationName".repeat(8);
  programs.managedApps[0].exePath = "C:\\WinOne-test\\" + "LongDirectoryName\\".repeat(12) + "program.exe";
  ai.providers[0].name = "Example provider with a long descriptive name";
  ai.providers[0].availableModels!.push(...Array.from({length: 50}, (_, index) => `long-model-name-${index}`));
}
let gateway = {revision: 1, enabled: true, running: true, port: 3426, hasKey: true, baseUrl: "http://127.0.0.1:3426/v1", models: null};
const source = "C:\\WinOne-test\\source", destination = "C:\\WinOne-test\\links";
const entries: DirectoryEntry[] = [{name: "项目", path: source + "\\项目", objectType: "directory"}, ...Array.from({length: 24}, (_, index) => ({name: `document-${index + 1}.txt`, path: source + `\\document-${index + 1}.txt`, objectType: "file" as const}))];
let records: DirectoryLinkRecord[] = ["hardlink", "junction", "symlink", "shortcut"].map((kind, index) => ({id: `known-${index}`, name: `example-${kind}`, linkPath: destination + `\\example-${kind}`, targetPath: source + `\\example-${kind}`, kind: kind as DirectoryLinkRecord["kind"], objectType: index % 2 ? "directory" : "file", targetText: source, relative: false, createdAt: "2026-10-15T00:00:00Z", checkedAt: "2026-10-15T00:00:00Z", status: "active", reason: "target-accessible", accessible: true, entryIdentity: {file: `fixture-${index}`}}));
if(query.has("history-states")){records[1].status="broken";records[1].reason="target-missing";records[2].status="unavailable";records[2].reason="target-offline";records[3].targetPath=records[0].targetPath;}
let menu = {enabled: false, partial: false};
let administrator = false;
const snapshot = () => ({settings: structuredClone(globalSettings), modules: modules.map(module => ({...module.runtime})), locale: language, administrator});
const fail = (method: string) => {
  if (query.get("failure") === method) throw Error(language === "zh-CN" ? "设置读取失败，请重试。（隔离测试）" : "Settings could not be read. Retry. (Isolated test)");
};
const record = (method: string, params?: unknown) => {calls.push({method, params}); fail(method);};
Object.assign(window, {settingsDesignFixture: {calls, recover: () => query.delete("failure"), state: () => structuredClone({globalSettings, capture, aot, selection, quick, stroke, programs, ai, gateway})}});

type NativeApp = NonNullable<NonNullable<Window["go"]>["main"]>["App"];
window.go = {main: {App: {
  Interconnect: interconnectFixture(),
  Snapshot: async () => snapshot(),
  SaveSettings: async (value: Settings) => {record("settings.save", value); Object.assign(globalSettings, value, {revision: globalSettings.revision + 1}); return structuredClone(globalSettings);},
  SettingsNavigation: async () => 0,
  InitializeWindow: async () => ({maximized: false}), WindowFrameState: async () => ({maximized: false}), WindowCommand: async (action: string) => {record("window.command", action);},
  StartupProbeNonce: async () => "", SetSettingsActivity: async () => {},
  BeginShortcutRecording: async () => "isolated-shortcut", EndShortcutRecording: async () => {},
  Shortcuts: async () => JSON.stringify({bindings: [["open-winone", "openShortcut"], ["capture", "captureShortcut"], ["ocr", "ocrShortcut"], ["translate", "translateShortcut"], ["selection", "selectionShortcut"], ["quick-assistant", "quickAssistantShortcut"], ["pin", "pinShortcut"]].map(([id, key]) => ({id, shortcut: globalSettings[key as keyof Settings], registered: !!globalSettings[key as keyof Settings], error: ""})), actionErrors: {}}),
  SetAdministratorPreference: async (value: boolean) => {record("permission.save", value); globalSettings.startup!.preferAdministrator = value; return structuredClone(globalSettings);},
  RestartAsAdministrator: async () => {record("permission.restart"); administrator = true; return false;},
  UpdateInfo: async () => ({version: "0.0.1", configured: false}),
  ReadModuleLogs: async () => {record("logs.read"); return {folderAvailable: false, entries: []};},
  OpenModuleLogFolder: async () => {record("logs.open");},
  ExportProfile: async () => {record("profile.export"); return "C:\\WinOne-test\\profile.winone";},
  PickDirectory: async () => source,
  PickProgram: async () => "C:\\WinOne-test\\replacement.exe",
  PickPrograms: async () => ["C:\\WinOne-test\\new-program.exe"],
  ProgramInfo: async (path: string) => ({name: path.split(/[\\/]/).at(-1)?.replace(/\.[^.]+$/, "") || "Example"}),
  PickChatFiles: async () => [entries[1].path], PickDirectories: async () => [entries[0].path],
  Residency: async () => JSON.stringify({requested: false, reasons: []}),
  Modules: async (method: string, payload: string) => {
    const params = JSON.parse(payload); record("modules." + method, params);
    if (method === "snapshot") return JSON.stringify({revision: 1, modules});
    if (method === "setEnabled") {const module = modules.find(module => module.manifest.id === params.id)!; module.enabled = params.enabled; module.runtime.state = params.enabled ? "ready" : "disabled"; return JSON.stringify({state: "completed"});}
    if (method === "resetWinTray.preview") return JSON.stringify({configurationRevision: 1, moduleRevision: 1, programCount: programs.managedApps.length});
    throw Error("Unsupported isolated module operation: " + method);
  },
  AlwaysOnTop: async (method: string, payload: string) => {
    const params = JSON.parse(payload); record("aot." + method, params);
    if (method === "settings.save") aot = {...params, revision: aot.revision + 1};
    return JSON.stringify(method === "snapshot" ? {settings: aot, administrator} : aot);
  },
  Selection: async (method: string, payload: string) => {record("selection." + method, JSON.parse(payload)); if (method === "settings.save") selection = {...JSON.parse(payload), revision: selection.revision + 1}; return JSON.stringify(selection);},
  QuickAssistant: async (method: string, payload: string) => {record("quick." + method, JSON.parse(payload)); if (method === "settings.save") quick = {...JSON.parse(payload), revision: quick.revision + 1}; return JSON.stringify(method === "show" ? {shown: true} : quick);},
  Stroke: async (method: string, payload: string) => {record("stroke." + method, JSON.parse(payload)); if (method === "settings.save") stroke = JSON.parse(payload); return JSON.stringify(stroke);},
  WinTray: async (method: string, payload: string) => {const params = JSON.parse(payload); record("wintray." + method, params); if (method === "settings.save") {programs = params.settings; return JSON.stringify({state: "completed"});} return JSON.stringify({revision: "1", settings: programs});},
  Chat: async (method: string, payload: string) => {const value = usageFixture("chat." + method, payload); if (value !== undefined) return JSON.stringify(value); throw Error("Chat is excluded from this fixture");},
  Capture: async (method: string, payload: string) => {
    const params = JSON.parse(payload); record("capture." + method, params);
    const usage = usageFixture(method, payload); if (usage !== undefined) return JSON.stringify(usage);
    if (method === "snapshot") return JSON.stringify({settings: capture});
    if (method === "settings.get") return JSON.stringify(capture);
    if (method === "settings.save") {capture = {...params, revision: capture.revision + 1}; return JSON.stringify(capture);}
    if (method === "filename.preview") return JSON.stringify({name: (params.template || "WinOne-{timestamp}").replaceAll("{timestamp}", "20261015093000").replaceAll("{date}", "20261015").replaceAll("{time}", "093000").replaceAll("{sequence}", "01") + ".png"});
    if (method === "ai.configuration.get") return JSON.stringify(ai);
    if (method === "ai.configuration.save") {ai = {...params.configuration, revision: ai.revision + 1}; return JSON.stringify(ai);}
    if (method === "ai.accounts.catalog") return JSON.stringify({presets: [{id: "openai", name: "OpenAI", icon: "openai", chat: "https://example.invalid/v1", models: ["gpt-5.5"]}], agents: [{id: "codex", name: "ChatGPT / Codex", icon: "openai"}]});
    if (method === "ai.accounts.list") return JSON.stringify({accounts: [{agent: "codex", user: "preview@example.invalid", plan: "Preview"}]});
    if (method === "ai.models.fetch") return JSON.stringify({models: [{id: "shared-model"}, {id: "vision-model"}, {id: "new-model"}]});
    if (method === "ai.models.test") return JSON.stringify({results: params.models.map((model: string) => ({model, ok: true, ms: 120})), clients: {codex: "", "claude-code": ""}});
    if (method === "ai.magpie.catalog") return JSON.stringify({visionModels: [{id: "one/vision-model", name: "Vision", provider: "one", providerName: ai.providers[0].name}], imageGenModels: [], titleModels: [], searchVendors: [{id: "tavily", name: "Tavily"}, {id: "searxng", name: "SearXNG", needURL: true}], searchChoices: [], quotas: [{id: "account", name: "ChatGPT / Codex"}], codexUsers: ["preview@example.invalid"], pluginCheckins: [{id: "example", name: "Example check-in"}]});
    // Viewing privacy settings owns a read-only lease; the layout fixture
    // never downloads or starts a model, just like a fresh native profile.
    if (["ai.magpie.privacy.open", "ai.magpie.privacy.state"].includes(method)) return JSON.stringify({on: false, supported: true, installed: false, size: 1637572192});
    if (method === "ai.magpie.privacy.close") return JSON.stringify({});
    if (method === "ai.gateway.get") return JSON.stringify(gateway);
    if (method === "ai.gateway.save") {gateway = {...gateway, ...params.settings, revision: gateway.revision + 1, running: params.settings.enabled}; return JSON.stringify(gateway);}
    if (method === "ai.gateway.key") return JSON.stringify({key: "fixture-not-a-real-key"});
    if (["ai.routing.close", "ai.routing.plugins.close", "ai.routing.extensions.close"].includes(method)) return JSON.stringify({});
    if (["ai.routing.open", "ai.routing.plugins.open", "ai.routing.extensions.open"].includes(method)) throw Error("Magpie 原版路由与插件页在桌面集成测试中验证；此夹具只检查宿主错误边界。");
    throw Error("Unsupported isolated capture operation: " + method);
  },
  InspectDroppedPaths: async (paths:string[]) => {record("paths.inspect",paths);return paths.map(path=>({path,kind:/\.[a-z0-9]+$/i.test(path)?"file":"directory"}));},
  SetFileDropTarget: async () => {},
  DirectoryLinks: async (method: string, payload: string) => {
    const params = JSON.parse(payload); record("links." + method, params);
    if (method === "pending") return JSON.stringify({requested: false, paths: []});
    if (method === "menu.get") return JSON.stringify(menu);
    if (method === "menu.set") {menu = {enabled: params.enabled, partial: false}; return JSON.stringify(menu);}
    if (method === "forgetInvalid") {const previous=records.length;records=records.filter(row=>!["broken","removed","replaced","unlinked","target-changed","source-changed"].includes(row.status));return JSON.stringify({records,forgotten:previous-records.length});}
    if (method === "list" || method === "inspectBatch") return JSON.stringify(records);
    if (method === "browse") return JSON.stringify({directory: params.directory, parent: "C:\\WinOne-test", entries: params.directory === source ? entries : [], total: params.directory === source ? entries.length : 0, offset: 0, hasMore: false});
    if (method === "scan") return JSON.stringify({records, issues: [], scanned: 24, truncated: false, cancelled: false});
    if (method === "forget") {records = records.filter(record => record.id !== params.id); return JSON.stringify({});}
    throw Error("Unsupported isolated file-link operation: " + method);
  },
} as unknown as NativeApp}};

// Import the real entry only after replacing every native boundary it reads.
await import("../src/main");
