// Cindy's attachment/file preload over WinOne's bounded local bridge. Browser
// files are copied in chunks; the Host resolves every later native action.
import {chatCall, chatMediaCall} from '../../../bridge';
import {hasManagedMedia, registerManagedMedia} from './onebotMedia';
import type {BotGroupGetResult} from '../../../../../../../modules/chat-pi/cindy/tasks/shared/botGroupChat';
import type {OpenPathResult} from '../shared/openPathResult';

const call = <T>(method: string, params: unknown = {}) => chatCall<T>('onebot.groups.files.' + method, params);
const paths = new WeakMap<File, string>();
let activeGroup = '';
const CHUNK = 256 << 10;
export function encodeBytes(bytes: Uint8Array): string {let value='';for(let i=0;i<bytes.length;i+=0x4000)value+=String.fromCharCode(...bytes.subarray(i,i+0x4000));return btoa(value);}
export function decodeBytes(value: string): Uint8Array {return Uint8Array.from(atob(value),c=>c.charCodeAt(0));}
interface FileInfo {path: string; resolvedPath: string; size: number; modified: number; name: string; mimeType: string; kind: string; downloadOnly?: boolean; url?: string}
interface FileChunk {base64: string; next: number; done: boolean; size: number}
interface ResolvedPath {status:'unique'|'multiple'|'none';candidates:string[];kind?:'file'|'directory'}
type RemoteFetch = {ok:true;cachePath:string;size:number;stale:boolean}|{ok:false;code:'BAD_ARGS'|'OUTSIDE_WORKDIR'|'NOT_FOUND'|'FETCH_FAILED';message?:string};
type RemoteDownload = {ok:true;path:string;stale:boolean;skipped:number}|{ok:false;code:'BAD_ARGS'|'OUTSIDE_WORKDIR'|'NOT_FOUND'|'FETCH_FAILED'|'REMOTE_UNSUPPORTED'|'NO_SPACE';message?:string};
export async function readGroupBytes(path: string): Promise<Uint8Array> {
  const chunks: Uint8Array[]=[];let offset=0;
  for (;;) {const value=await call<FileChunk>('read',{path,offset});chunks.push(decodeBytes(value.base64));offset=value.next;if(value.done)break;}
  const result=new Uint8Array(offset);let at=0;for(const bytes of chunks){result.set(bytes,at);at+=bytes.length;}return result;
}
async function stage(blob: Blob, name: string, scope?: string, image=false): Promise<FileInfo> {
  const groupId=scope?.startsWith('bot-group:')?scope.slice(10):activeGroup;
  if(!groupId)throw Error('请先打开群聊');
  const {token}=await call<{token:string}>('begin',{groupId,name,size:blob.size});
  for(let offset=0;offset<blob.size;offset+=CHUNK){const bytes=new Uint8Array(await blob.slice(offset,offset+CHUNK).arrayBuffer());await call('chunk',{token,offset,base64:encodeBytes(bytes)});}
  const file=await call<FileInfo>('finish',{token,image});
  if(file.url)registerManagedMedia(file.url,file.mimeType,encodeBytes(new Uint8Array(await blob.arrayBuffer())));
  return file;
}
/** Prime the stable media addresses before the original group view renders. */
export async function prepareGroupDetail(result: BotGroupGetResult, groupId: string): Promise<BotGroupGetResult> {
  activeGroup=groupId;
  if(result.ok){
    const urls=new Set(result.group.messages.flatMap(row=>(row.attachments??[]).filter(item=>item.category==='image'&&item.url).map(item=>item.url!)));
    await Promise.all([...urls].filter(url=>!hasManagedMedia(url)).map(async url=>{try {const info=await call<FileInfo>('describe',{path:url});registerManagedMedia(url,info.mimeType,encodeBytes(await readGroupBytes(url)));}catch(error){console.warn('OneBot attachment preview unavailable',error);}}));
  }
  return result;
}
async function materialize(url: string, name='image.png'): Promise<string> {
  if(url.startsWith('data:')||url.startsWith('blob:')){const response=await fetch(url);return (await stage(await response.blob(),name,undefined,true)).path;}
  if(url.startsWith('http:')||url.startsWith('https:')){const result=await call<{url:string}>('remoteImage',{url});return (await call<FileInfo>('describe',{path:result.url})).path;}
  return (await call<FileInfo>('describe',{path:url})).path;
}
async function nativeMedia<T>(method: string, path: string, extra: Record<string,unknown> = {}): Promise<T> {
  return chatMediaCall<T>(method,{id:'bot-group:'+activeGroup,path,...extra});
}
export const oneBotGroupFilePreload = {
  resolvePath:(input:{href:string;workingDir:string})=>call<ResolvedPath>('resolvePath',input),
  resolvePathBatch:(input:{hrefs:string[];workingDir:string})=>call<Record<string,ResolvedPath>>('resolvePathBatch',input),
  cindyMediaStorage:{reportDraftUrls:(urls:string[])=>{void call('draftUrls',{urls}).catch(error=>console.warn('OneBot draft image references were not saved',error));}},
  async readImageBytes(input:{url:string}) {const file=await call<FileInfo>('describe',{path:await materialize(input.url)});return {base64:encodeBytes(await readGroupBytes(file.path)),mimeType:file.mimeType};},
  // Shared file controls also know Cindy remote origins. This product has no
  // remote file service; report that fact rather than reading their paths here.
  fileBrowser: {
    chatFetch:async(_input:unknown):Promise<RemoteFetch>=>({ok:false,code:'FETCH_FAILED',message:'未启用远程设备文件服务'}),
    chatDownload:async(_input:unknown):Promise<RemoteDownload>=>({ok:false,code:'REMOTE_UNSUPPORTED',message:'未启用远程设备文件服务'}),
    chatStat:async(_input:unknown):Promise<{verdict:'file'|'directory'|'nonfile'|'unknown'}>=>({verdict:'unknown'}),
    chatCancel:async(_requestId:string)=>({cancelled:false}),
    onTransferProgress:(_callback:(event:{requestId:string;received:number;total:number;phase?:'pack'|'upload'|'download'|'extract'})=>void)=>()=>{},
    readCached:async(_input:unknown):Promise<{ok:true;kind:'text';content:string}|{ok:false;message:string}>=>({ok:false,message:'未启用远程设备文件服务'}),
    previewHtml:async(_input:unknown):Promise<{url:string}>=>{throw Error('未启用远程设备文件服务');},
    cancelHtmlPreview:async(_requestId:string)=>({cancelled:false}),
  },
  getFilePath: (file: File): string => paths.get(file)??'',
  async resolveAttachmentFile(file: File, scope?: string): Promise<string> {const existing=paths.get(file);if(existing)return existing;const saved=await stage(file,file.name,scope);paths.set(file,saved.path);return saved.path;},
  async cacheImageFromBuffer(input: {sessionId?:string;buffer:Uint8Array;mimeType?:string;suggestedName?:string}) {const file=await stage(new Blob([new Uint8Array(input.buffer).buffer],{type:input.mimeType}),input.suggestedName??'image.png',input.sessionId,true);return {url:file.url!};},
  async cacheImageFromPath(input: {sessionId?:string;sourcePath:string;originalName?:string}) {const value=await call<{url:string;mimeType:string}>('image',{path:input.sourcePath});registerManagedMedia(value.url,value.mimeType,encodeBytes(await readGroupBytes(value.url)));return {url:value.url};},
  async cacheMediaForSession(input: {sessionId?:string;url:string;originalName?:string}) {const source=await materialize(input.url,input.originalName),info=await call<FileInfo>('describe',{path:source});return {...await oneBotGroupFilePreload.cacheImageFromPath({sourcePath:source,sessionId:input.sessionId}),name:input.originalName??info.name,ext:'.'+info.name.split('.').at(-1),size:info.size,mimeType:info.mimeType};},
  async peekFileHeader(input: {filePath:string;bytes?:number}) {const result=await call<FileChunk>('read',{path:input.filePath,offset:0,limit:Math.min(input.bytes??8192,CHUNK)});return {success:true,data:result.base64,actualBytes:decodeBytes(result.base64).length};},
  async readFileForAttachment(input: {filePath:string;encoding?:string}):Promise<{success:boolean;data:string;size:number;error?:string}> {const bytes=await readGroupBytes(input.filePath);return {success:true,data:input.encoding==='base64'?encodeBytes(bytes):new TextDecoder().decode(bytes),size:bytes.length};},
  async readTextFilePreview(input: {filePath:string}):Promise<{success:boolean;data?:string;size:number;reason?:string;limitMb?:number;error?:string}> {
    const info=await call<FileInfo>('describe',{path:input.filePath});
    // Reuse WinOne's bounded text-preview behavior; the complete file stays on
    // disk and can still be saved or opened through its native action.
    if(info.size>(2<<20))return {success:false,reason:'oversize',size:info.size,limitMb:2};
    return {success:true,data:new TextDecoder().decode(await readGroupBytes(input.filePath)),size:info.size};
  },
  async stageChatAttachment(input: {sourcePath:string;suggestedName:string}):Promise<{success:true;path:string}|{success:false;code:string}> {return {success:true,path:(await call<FileInfo>('describe',{path:input.sourcePath})).path};},
  cleanupStagedChatAttachments: (files:readonly string[])=>call<void>('discard',{paths:files}),
  // Managed images are content addressed and may also be an avatar or a sent
  // message. Only staged, unreferenced files are eligible for tray cleanup.
  cleanupCachedImages: (urls:readonly string[])=>call<void>('discard',{paths:urls}),
  async getFileThumbnail(input:{path:string;size?:number;revalidate?:boolean}) {const info=await call<FileInfo>('describe',{path:input.path});return {byteSize:info.size,dataUrl:info.kind==='image'?'data:'+info.mimeType+';base64,'+encodeBytes(await readGroupBytes(input.path)):null};},
  async showItemInFolder(input:string|{url?:string;filePath?:string}):Promise<OpenPathResult> {const path=typeof input==='string'?input:input.url??input.filePath;await chatCall('onebot.groups.files.reveal',{path});return {success:true};},
  async openMediaWithDefaultApp(input:{url:string}):Promise<OpenPathResult> {await chatCall('onebot.groups.files.open',{path:await materialize(input.url)});return {success:true};},
  async openFileInBrowser(path:string) {await chatCall('onebot.groups.files.open',{path});return {success:true};},
  async copyMediaToClipboard(input:{url?:string;filePath?:string;plainText?:string;originalPath?:string}):Promise<OpenPathResult> {const file=await call<FileInfo>('describe',{path:await materialize(input.url??input.filePath??'')});return nativeMedia<OpenPathResult>(file.kind==='image'?'copy':'copy-file',file.path,{plainText:input.plainText,originalPath:input.originalPath});},
  async copyPngToClipboard(input:{png?:ArrayBuffer;base64?:string;buffer?:Uint8Array;plainText?:string;originalPath?:string}) {const bytes=input.png?new Uint8Array(input.png):input.buffer??decodeBytes(input.base64??'');const file=await stage(new Blob([new Uint8Array(bytes).buffer],{type:'image/png'}),'clipboard.png',undefined,true);return nativeMedia<{success:boolean}>('copy',file.path,{plainText:input.plainText});},
  async openGroupPath(path:string) {await chatCall('onebot.groups.files.open',{path});return {success:true};},
  async saveMediaAs(input:{url:string;suggestedName?:string;sessionId?:string}) {return nativeMedia<{canceled:boolean;path?:string}>('save-as',await materialize(input.url,input.suggestedName),{suggestedName:input.suggestedName});},
  async saveChatAttachmentAs(input:{sourcePath:string;suggestedName:string}):Promise<{status:'saved'|'canceled'|'failed';code?:string}> {const result=await nativeMedia<{canceled:boolean}>('save-file',input.sourcePath,{suggestedName:input.suggestedName});return {status:result.canceled?'canceled':'saved'};},
};
