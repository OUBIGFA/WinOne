/** Host contracts for chat resources. Keep these independent of global settings. */
import type {AISelection} from "./ai-configuration";
import type {ChatPermission} from "./chat-state";
import type {BrowserCommentTargetInfo,BrowserCommentStyleChange} from './cindy/browser/browserComment';
export interface ChatBrowserAnnotation{id:string;markerNumber:number;pageUrl:string;target:BrowserCommentTargetInfo;comment:string;screenshotPath:string;styleChanges?:BrowserCommentStyleChange[]}
export interface ChatAttachment{name:string;path:string;size:number;mimeType:string;annotation?:ChatBrowserAnnotation;imageAnnotation?:{sourcePath:string;strokes:{points:{x:number;y:number}[]}[]}}
export interface ChatFiles{directory:string;entries:{name:string;path:string;directory:boolean;size:number}[]}
export interface ChatResources{
 agents:{path:string}[];
 skills:{name:string;description:string;path:string}[];
 plugins:{name:string;path:string;enabled:boolean}[];
 mcpServers:{name:string;enabled:boolean;status?:string;error?:string}[];
 diagnostics:string[];
}
export interface ChatMCPServer{name:string;enabled:boolean;command?:string;args?:string[];cwd?:string;url?:string;env?:Record<string,string>;headers?:Record<string,string>;timeout?:number;exposure?:'direct'|'deferred'|'codemode'|'hidden';description?:string;toolExposure?:Record<string,string>}
export interface ChatAutomation{id:string;name:string;prompt:string;projectId?:string;directory?:string;model:AISelection;permission:ChatPermission;intervalMinutes:number;enabled:boolean;nextRunAt?:string;lastRunAt?:string;lastConversationId?:string;status?:string;error?:string}
export interface ChatSettingsData{workspaceDirectory:string;titleModel?:AISelection;skillDirectories:string[];pluginDirectories:string[];mcpServers:ChatMCPServer[];automations:ChatAutomation[];defaultThinkingLevel?:string;autoCompaction?:boolean;autoRetry?:boolean;backgroundCompaction?:boolean;codeMode?:boolean;planning?:boolean;toolPreset?:string;subagents?:boolean;maxSubagents?:number;subagentModel?:AISelection;subagentThinkingLevel?:string}
