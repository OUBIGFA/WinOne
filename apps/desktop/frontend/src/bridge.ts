import {scheduleChatCall} from './features/chat-read-queue';
import type {CaptureLaunchPort,CaptureLaunchRequest} from "./features/capture-launch";

export interface Settings {
  schemaVersion: number;
  revision: number;
  theme: "system" | "light" | "dark";
  language?: "system" | "zh-CN" | "en-US";
  defaultPage?: string;
  openShortcut?: string;
  captureShortcut?: string;
  ocrShortcut?: string;
  translateShortcut?: string;
  selectionShortcut?: string;
  quickAssistantShortcut?: string;
  pinShortcut?: string;
  startup?: {runAtLogon:boolean;startMinimized:boolean;preferAdministrator?:boolean;keepUIRunning?:boolean};
}
export interface Module {
  id: string;
  name: string;
  state: string;
  detail: string;
}
export interface Snapshot {
  settings: Settings;
  modules: Module[];
  locale?: "zh-CN" | "en-US";
  administrator?: boolean;
  runtimeWarning?: string;
}

interface NativeApp {
  UpdateInfo():Promise<UpdateBuildInfo>;
  CheckForUpdates(id:string):Promise<UpdateCheckResult>;
  CancelUpdateCheck(id:string):Promise<void>;
  OpenUpdateRelease():Promise<void>;
  AbortCaptureOpening():Promise<void>;
  OpenRepository():Promise<void>;
  ShowCaptureRecovery(id:string):Promise<void>;
  RevealCaptureEditor():Promise<void>;
  KeepSettingsWindow():Promise<void>;
  BeginCaptureSurface(id:string):Promise<boolean>;
  ShowCaptureSurface(id:string,width:number,height:number,scale:number):Promise<void>;
  EndCaptureSurface(id:string):Promise<void>;
  OpenCaptureWindow(id:string):Promise<void>;
  CaptureSurfaceMonitor(id:string):Promise<{left:number;top:number;right:number;bottom:number}>;
  BeginCaptureSurfaceProbe(nonce:string):Promise<{width:number;height:number}>;
  EndCaptureSurfaceProbe(nonce:string,width:number,height:number,scale:number):Promise<void>;
	SettingsNavigation():Promise<number>;
  InitializeWindow():Promise<{maximized:boolean}>;
  WindowFrameState():Promise<{maximized:boolean}>;
  WindowCommand(action:string):Promise<void>;
  BeginShortcutRecording():Promise<string>;
  EndShortcutRecording(token:string):Promise<void>;
  SetSettingsActivity?(dirty:boolean,busy:boolean):Promise<void>;
  SettingsClosePending?():Promise<boolean>;
  CancelSettingsClose?():Promise<void>;
  ApproveSettingsClose?():Promise<void>;
  StartupProbeNonce?():Promise<string>;
  CompleteStartupProbe?(nonce:string):Promise<void>;
  ReadModuleLogs(id:string):Promise<ModuleLogs>;
  OpenModuleLogFolder(id:string):Promise<void>;
  Snapshot(): Promise<Snapshot>;
  SaveSettings(value: Settings): Promise<Settings>;
  SetAdministratorPreference(enabled:boolean):Promise<Settings>;
  RestartAsAdministrator():Promise<boolean>;
  WinTray(method: string, payload: string): Promise<string>;
  PickProgram(): Promise<string>;
  ProgramInfo(path:string):Promise<{name:string;icon?:string}>;
  PickPrograms(): Promise<string[]|null>;
  PickWinTrayConfig(): Promise<string>;
  Capture(method: string, payload: string): Promise<string>;
  AlwaysOnTop(method: string, payload: string): Promise<string>;
  Selection(method: string, payload: string): Promise<string>;
  QuickAssistant(method: string, payload: string): Promise<string>;
  Chat(method: string, payload: string): Promise<string>;
  ChatBrowser(method: string, payload: string): Promise<string>;
  ChatMedia(method: string, payload: string): Promise<string>;
  PickDirectory(): Promise<string>;
  ReadClipboardFiles():Promise<string[]>;
  PickChatFiles(): Promise<string[]>;
  PickFile(): Promise<string>;
  PickDirectories():Promise<string[]>;
  DirectoryLinks(method:string,payload:string):Promise<string>;
  Interconnect(method:string,payload:string):Promise<string>;
  Stroke(method:string,payload:string):Promise<string>;
  ReadDroppedImage(path:string):Promise<string>;
  InspectDroppedPaths(paths:string[]):Promise<{path:string;kind:"file"|"directory"}[]>;
  SetFileDropTarget?(enabled:boolean):Promise<void>;
  Reveal(): Promise<void>;
  Shortcuts(): Promise<string>;
  Residency(): Promise<string>;
  Modules(method: string, payload: string): Promise<string>;
  ExportProfile(): Promise<string>;
  PickProfile(): Promise<string>;
  PickModulePackage(): Promise<string>;
  PickProfileLocation(kind: string): Promise<string>;
  Profiles(method: string, payload: string): Promise<string>;
  CaptureLaunch(): Promise<CaptureLaunchRequest|null>;
  AcknowledgeCaptureLaunch(id:string):Promise<boolean>;
  SetCaptureEditing(active:boolean):Promise<void>;
  CaptureClosePending():Promise<boolean>;
  CancelCaptureClose():Promise<void>;
}
declare global {
  interface Window {
    go?: { main?: { App?: NativeApp } };
    runtime?: {EventsOn:(name:string,listener:()=>void)=>()=>void};
  }
}

export interface UpdateBuildInfo {version:string;configured:boolean}
export interface UpdateCheckResult {state:string;currentVersion?:string;latestVersion?:string;errorCode?:string}
export const readClipboardFiles=()=>native().ReadClipboardFiles();
export const updateInfo=()=>native().UpdateInfo();
export const checkUpdates=(id:string)=>native().CheckForUpdates(id);
export const cancelUpdateCheck=(id:string)=>native().CancelUpdateCheck(id);
export const openUpdateRelease=()=>native().OpenUpdateRelease();

// A browser preview must not claim the Windows backend is connected.
function native(): NativeApp {
  const api = typeof window === "undefined" ? undefined : window.go?.main?.App;
  if (!api) throw new Error("请从 WinOne 桌面程序打开");
  return api;
}
export const startupProbePort={
  surface:async(nonce:string)=>{
    const expected=await native().BeginCaptureSurfaceProbe(nonce);
    // Hidden WebView2 resize delivery is asynchronous. This finite probe never
    // creates a background poller in a normal session.
    for(let attempt=0;attempt<40;attempt++){
      await new Promise(resolve=>setTimeout(resolve,25));
      if(Math.abs(window.innerWidth*window.devicePixelRatio-expected.width)<=1&&Math.abs(window.innerHeight*window.devicePixelRatio-expected.height)<=1)break;
    }
    await native().EndCaptureSurfaceProbe(nonce,window.innerWidth,window.innerHeight,window.devicePixelRatio);
  },
  nonce:()=>native().StartupProbeNonce?.()??Promise.resolve(""),
  complete:(nonce:string)=>{
    const complete=native().CompleteStartupProbe;
    if(!complete)throw Error("Startup probe bridge is unavailable");
    return complete(nonce);
  },
};
export const settingsClosePort={
  setActivity:(dirty:boolean,busy:boolean)=>native().SetSettingsActivity?.(dirty,busy)??Promise.resolve(),
  pending:()=>native().SettingsClosePending?.()??Promise.resolve(false),
  cancel:()=>native().CancelSettingsClose?.()??Promise.resolve(),
  approve:()=>{const approve=native().ApproveSettingsClose;if(!approve)throw Error("Window close bridge is unavailable");return approve();},
  subscribe:(listener:()=>void)=>window.runtime?.EventsOn("winone:settings-close",listener)??(()=>{}),
  subscribeDiscard:(listener:()=>void)=>window.runtime?.EventsOn("winone:settings-discarded",listener)??(()=>{}),
};
export const shortcutRecordingPort={begin:()=>native().BeginShortcutRecording(),end:(token:string)=>native().EndShortcutRecording(token)};
export const settingsNavigationPort={
  read:()=>native().SettingsNavigation(),
  subscribe:(listener:()=>void)=>window.runtime?.EventsOn("winone:open-settings",listener)??(()=>{}),
};
export const windowFramePort={initialize:()=>native().InitializeWindow(),read:()=>native().WindowFrameState(),command:(action:"minimize"|"toggle-maximize"|"close")=>native().WindowCommand(action)};
export const captureSurfacePort={release:(id:string)=>captureCall("handoff.release",{expectedId:id}).then(()=>{}),begin:(id:string)=>native().BeginCaptureSurface(id),show:(id:string)=>native().ShowCaptureSurface(id,window.innerWidth,window.innerHeight,window.devicePixelRatio),end:(id:string)=>native().EndCaptureSurface(id),monitor:(id:string)=>native().CaptureSurfaceMonitor(id),keepSettings:()=>native().KeepSettingsWindow(),recovery:(id:string)=>native().ShowCaptureRecovery(id)};
export const openCaptureWindow=(id:string)=>native().OpenCaptureWindow(id);
export const revealCaptureEditor=()=>native().RevealCaptureEditor();
export const captureOpeningPort={...captureSurfacePort,setEditing:(active:boolean)=>native().SetCaptureEditing(active),abortOpening:()=>native().AbortCaptureOpening(),revealEditor:revealCaptureEditor};
export const openRepository=()=>native().OpenRepository();
export const pickModulePackage=()=>native().PickModulePackage();
export const captureLaunchPort:CaptureLaunchPort={
  read:()=>native().CaptureLaunch(),
  acknowledge:id=>native().AcknowledgeCaptureLaunch(id),
  subscribe:listener=>{
    if(!window.runtime)throw new Error("WinOne window runtime is not ready");
    return window.runtime.EventsOn("winone:capture-launch",listener);
  },
};
export const setCaptureEditing=(active:boolean)=>native().SetCaptureEditing(active);
export const captureClosePending=()=>native().CaptureClosePending();
export const cancelCaptureClose=()=>native().CancelCaptureClose();
export function subscribeCaptureClose(listener:()=>void):()=>void {
 if(!window.runtime)throw new Error("WinOne window runtime is not ready");
 return window.runtime.EventsOn("winone:capture-close",listener);
}
export async function getSnapshot(): Promise<Snapshot> {
  return native().Snapshot();
}
export async function saveSettings(value: Settings): Promise<Settings> {
  return native().SaveSettings(value);
}

// Immediate controls change only the field the user just selected. Other
// pages share the Host revision, so read it at click time and retain CAS:
// an intervening write is reported, never retried as a blind overwrite.
export async function savePreferenceChange(patch:Partial<Pick<Settings,"theme"|"language"|"defaultPage">>&{startup?:Partial<NonNullable<Settings["startup"]>>}):Promise<Settings> {
  const {settings}=await getSnapshot();
  const {startup,...appearance}=patch;
  return saveSettings({...settings,...appearance,...(startup?{startup:{runAtLogon:false,startMinimized:true,...settings.startup,...startup}}:{})});
}

export async function wintrayCall<T>(
  method: string,
  params: unknown = null,
): Promise<T> {
  return JSON.parse(
    await native().WinTray(method, JSON.stringify(params)),
  ) as T;
}
export async function pickProgram(): Promise<string> {
  return native().PickProgram();
}
export async function pickPrograms(): Promise<string[]> { return await native().PickPrograms() ?? []; }
export async function programInfo(path:string):Promise<{name:string;icon?:string}> { return native().ProgramInfo(path); }
export interface ModuleLogs {folderAvailable:boolean;entries:{name:string;available:boolean;truncated:boolean;text:string}[]}
export const readModuleLogs=(id:string)=>native().ReadModuleLogs(id);
export const openModuleLogFolder=(id:string)=>native().OpenModuleLogFolder(id);
export async function pickWinTrayConfig(): Promise<string> { return native().PickWinTrayConfig(); }
export async function exportProfile(): Promise<string> { return native().ExportProfile(); }
export async function pickProfile(): Promise<string> { return native().PickProfile(); }
export async function pickProfileLocation(kind: string): Promise<string> { return native().PickProfileLocation(kind); }
export async function profileCall<T>(method:string,params:unknown):Promise<T> { return JSON.parse(await native().Profiles(method,JSON.stringify(params))) as T; }

export async function captureCall<T>(
  method: string,
  params: unknown = null,
): Promise<T> {
  return JSON.parse(
    await native().Capture(method, JSON.stringify(params)),
  ) as T;
}
export async function pickDirectory(): Promise<string> {
  return native().PickDirectory();
}
export async function pickChatFiles():Promise<string[]>{return await native().PickChatFiles()??[];}
export async function pickFile():Promise<string>{return native().PickFile();}
export async function pickFiles():Promise<string[]>{return native().PickChatFiles();}
export async function pickDirectories():Promise<string[]>{return native().PickDirectories();}
export async function directoryLinksCall<T>(method:string,params:unknown=null):Promise<T>{return JSON.parse(await native().DirectoryLinks(method,JSON.stringify(params))) as T;}
export async function interconnectCall<T>(method:string,params:unknown=null):Promise<T>{return JSON.parse(await native().Interconnect(method,JSON.stringify(params))) as T;}
export async function strokeCall<T>(method:string,params:unknown=null):Promise<T>{return JSON.parse(await native().Stroke(method,JSON.stringify(params))) as T;}
// Multiple path fields may coexist with Chat or the link workspace. Keep the
// native registration until the last consumer has unmounted.
let fileDropTargets=0;
export async function setFileDropTarget(enabled:boolean):Promise<void>{fileDropTargets=Math.max(0,fileDropTargets+(enabled?1:-1));await native().SetFileDropTarget?.(fileDropTargets>0);}
export const readDroppedImage=(path:string)=>native().ReadDroppedImage(path);
export const inspectDroppedPaths=(paths:string[])=>native().InspectDroppedPaths(paths);
export async function reveal(): Promise<void> {
  return native().Reveal();
}

export async function alwaysOnTopCall<T>(method: string, params: unknown = null): Promise<T> {
  return JSON.parse(await native().AlwaysOnTop(method, JSON.stringify(params))) as T;
}
export async function quickAssistantCall<T>(method:string,params:unknown=null):Promise<T>{return JSON.parse(await native().QuickAssistant(method,JSON.stringify(params))) as T;}
export async function chatCall<T>(method:string,params:unknown={}):Promise<T>{const payload=JSON.stringify(params);return scheduleChatCall(method,payload,async()=>JSON.parse(await native().Chat(method,payload)) as T);}
export async function chatBrowserCall<T>(method:string,params:unknown={}):Promise<T>{return JSON.parse(await native().ChatBrowser(method,JSON.stringify(params))) as T;}
export type ChatExportOptions={password?:string;excludeMedia?:boolean};
export type ChatExportOutcome=
 |{status:"canceled"}
 |{status:"oversize";totalBytes:number;mediaBytes:number;limitBytes:number;messagesBytes?:number;transcriptBytes?:number}
 |{status:"ok";filePath:string;fidelity:"full"|"partial"|"db-only";unpackedBytes:number;totalBytes:number;mediaBytes:number;fileBytes:number;mediaMissing:number;mediaDropped:number;mediaExcluded:number;missingTranscripts:string[];orcaWorkers:number;messageCount:number};
// Only the native save dialog supplies the destination. Session data and file
// bytes stay between the Host and its isolated exporter, never in this bridge.
export async function exportChat(id:string,options:ChatExportOptions={}):Promise<ChatExportOutcome>{return chatCall<ChatExportOutcome>("export",{id,...options});}
export async function selectionCall<T>(method:string,params:unknown=null):Promise<T>{return JSON.parse(await native().Selection(method,JSON.stringify(params))) as T;}
export interface ShortcutStatus {
  actionErrors?:Record<string,string>;
  bindings: { id: string; shortcut: string; registered: boolean; error?: string }[];
  lastError: string;
}
export async function getShortcuts(): Promise<ShortcutStatus> {
  return JSON.parse(await native().Shortcuts()) as ShortcutStatus;
}
export function setAdministratorPreference(enabled:boolean):Promise<Settings>{return native().SetAdministratorPreference(enabled);}
export function restartAsAdministrator():Promise<boolean>{return native().RestartAsAdministrator();}
export async function getResidency(): Promise<{requested:boolean;reasons:string[]}> {
  return JSON.parse(await native().Residency());
}
export async function moduleCall<T>(method: string, params: unknown = null): Promise<T> {
  return JSON.parse(await native().Modules(method, JSON.stringify(params))) as T;
}

/** Native pixel clipboard and save dialog; bytes come from a scoped Host file. */
export async function chatMediaCall<T>(method:string,payload:unknown):Promise<T>{return JSON.parse(await native().ChatMedia(method,JSON.stringify(payload)));}
