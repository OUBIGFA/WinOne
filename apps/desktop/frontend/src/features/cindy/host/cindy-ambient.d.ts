// Ambient names that Cindy's renderer declares in vite-env.d.ts (b7e46bc).
type ProjectAutomationEvent = import('./electronAPI').ProjectAutomationReconciledEvent;

// Group composers use Tiptap's JSON document shape for saved plain text, but
// never instantiate a Tiptap editor in WinOne. Keep its structural wire type.
declare module '@tiptap/core' {
  export interface JSONContent {type?: string; attrs?: Record<string, any>; content?: JSONContent[]; marks?: {type: string; attrs?: Record<string, any>}[]; text?: string}
}
// Only the document text-extraction shape is used; OneBot itself uses a plain
// textarea and does not instantiate a ProseMirror editor.
declare module '@tiptap/pm/model' {
 export interface Node {content:{size:number};textBetween(from:number,to:number,blockSeparator?:string,leafText?:string):string}
}
interface CCAgentPermissionResult {behavior: 'allow' | 'deny'; message?: string; updatedPermissions?: unknown[]; decisionClassification?: string}

// Cindy's preload types `window.electronAPI` globally. In WinOne the vendored
// files read the seam module instead (the sync rewrites `window.electronAPI`
// to an import); only type positions such as Window['electronAPI'] use this.
interface Window {
  electronAPI: typeof import('./electronAPI').electronAPI;
}

// Personal WeChat state shapes declared in Cindy's renderer vite-env.d.ts.
type WechatBotPhase =
  | 'disconnected'
  | 'authorizing'
  | 'waiting_confirmation'
  | 'connected'
  | 'reconnecting'
  | 'needs_reauth'
  | 'disabled_by_policy'
  | 'error';
interface WechatBotState {
  phase: WechatBotPhase;
  bound: boolean;
  connectedAt?: number;
  lastInboundAt?: number;
  queuedTasks: number;
  errorCode?: string;
}
interface WechatChannelSettingsState {
  version: 1;
  workingDir: string | null;
  workingDirAvailable: boolean;
  companionBotId?: string;
}

// Cindy's shared modules run in Electron's main and renderer processes and use
// a Node global only after the browser API check fails (`typeof btoa`,
// `typeof process`). WebView2 always takes the browser branch; these
// declarations exist only so the type checker accepts the unused fallback.
declare const process: {readonly platform: string} | undefined;
declare const Buffer: {
  from(value: string, encoding: 'binary' | 'base64'): {toString(encoding: 'binary' | 'base64'): string};
};
// The routine run type reaches the renderer through a type-only import; the
// engine's receipt admission sizes keys with Node's Buffer in the worker only.
declare module 'node:buffer' {
  export const Buffer: {byteLength(value: string, encoding: 'utf8'): number};
}
