// ImageLightbox keeps Cindy's full UI, gestures and rasterization. The seams
// below adapt only WinOne's file capabilities, native actions and saved drafts.
import {createContext,useContext,useEffect,useState,type ReactNode} from 'react';
import {chatCall,chatMediaCall} from '../../../bridge';
import type {ChatAttachment} from '../../chat-types';
import type {ChatFilePreviewData} from '../../chat-file-preview';
import type {AttachedFile} from './fileTypes';
import type {AnnotationStroke} from './lightboxAnnotations';

export type MediaRecord={id:string;url:string;path?:string;name:string;mimeType:string;size:number;originalURL?:string;local:boolean;attachment?:ChatAttachment};
export type MediaOpen={id:string;src:string;alt?:string;gallery?:boolean;autoAnnotate?:boolean;attachment?:ChatAttachment;edit?:boolean;onClose?:()=>void};
export const MEDIA_OPEN_EVENT='winone:media-open';
export const MEDIA_ATTACH_EVENT='winone:media-attach';
export const MEDIA_OVERLAY_EVENT='winone:media-overlay';
const registry=new Map<string,MediaRecord>();
const drafts=new Map<string,{text:string;attachments:AttachedFile[];quotes:never[];browserComments:never[]}>();
const ScopeContext=createContext({sessionId:'',workingDir:''});
export function MediaScope({id,cwd,children}:{id:string;cwd?:string;children:ReactNode}){return <ScopeContext.Provider value={{sessionId:id,workingDir:cwd??''}}>{children}</ScopeContext.Provider>;}
export function useChatSessionFile(){return useContext(ScopeContext);}
export function useSidebarTargetSessionId(id?:string|null){const scope=useChatSessionFile();return id||scope.sessionId;}
export function createLogger(name:string){return {info:(...args:unknown[])=>console.info(name,...args),warn:(...args:unknown[])=>console.warn(name,...args),error:(...args:unknown[])=>console.error(name,...args)};}
const FEEDBACK='winone:media-feedback';
export const toast={success:(message:string)=>feedback(message,false),error:(message:string)=>feedback(message,true),warning:(message:string)=>feedback(message,true)};
function feedback(message:string,error:boolean){window.dispatchEvent(new CustomEvent(FEEDBACK,{detail:{message,error}}));}
export function MediaFeedback(){const [value,setValue]=useState<{message:string;error:boolean}>();useEffect(()=>{let timer:ReturnType<typeof setTimeout>;const receive=(event:Event)=>{setValue((event as CustomEvent).detail);clearTimeout(timer);timer=setTimeout(()=>setValue(undefined),6000);};window.addEventListener(FEEDBACK,receive);return()=>{clearTimeout(timer);window.removeEventListener(FEEDBACK,receive);};},[]);return value?<div role={value.error?'alert':'status'} className="cindy-panel-popup fixed bottom-5 left-1/2 z-[12000] max-w-[90vw] -translate-x-1/2 rounded-xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-2 text-13 text-[var(--text-primary)] shadow-lg">{value.message}</div>:null;}
export function registerMedia(record:MediaRecord){registry.delete(record.url);registry.set(record.url,record);while(registry.size>256)registry.delete(registry.keys().next().value!);return record;}
export function registerPreviewMedia(id:string,preview:ChatFilePreviewData,local=true,originalURL?:string,attachment?:ChatAttachment){return registerMedia({id,url:preview.url,path:preview.path,name:preview.name,mimeType:preview.mimeType,size:preview.size,local,originalURL,attachment});}
export function mediaRecord(src:string){return registry.get(src);}
export function isLocalMediaSource(src:string){return registry.get(src)?.local===true;}
export function releaseTransientMedia(src:string){if(/^(data:|blob:)/.test(src))registry.delete(src);}
export function openMedia(value:MediaOpen){window.dispatchEvent(new CustomEvent(MEDIA_OPEN_EVENT,{detail:value}));}

export function setMediaDraftSnapshot(id:string,text:string,files:ChatAttachment[]){
 if(!id)return;
 drafts.set(id,{text,quotes:[],browserComments:[],attachments:files.map(file=>({id:file.path,name:file.name,path:file.path,ext:'.'+(file.name.split('.').pop()??''),size:file.size,category:file.mimeType.startsWith('image/')?'image':'text',mimeType:file.mimeType}))});
}
export function getDraft(id:string){return drafts.get(id);}
export type MediaAttachDetail={id:string;files:ChatAttachment[];replace?:string;handled:boolean;resolve:()=>void;reject:(error:unknown)=>void};
export function attachMedia(id:string,files:ChatAttachment[],replace?:string):Promise<void>{return new Promise((resolve,reject)=>{const detail:MediaAttachDetail={id,files,replace,handled:false,resolve,reject};window.dispatchEvent(new CustomEvent(MEDIA_ATTACH_EVENT,{detail}));if(!detail.handled)void chatCall('media.attach',{id,files,replace}).then(()=>resolve(),reject);});}

const IMAGE_LIMIT=100*1024*1024;
function assertSize(size:number){if(size>IMAGE_LIMIT)throw Error('图片超过 100 MB');}
function dataImage(src:string):Blob{
 const match=/^data:(image\/[a-z0-9+.-]+)((?:;[^,]*)?),(.*)$/is.exec(src);if(!match)throw Error('图片数据格式无效');
 const [,mime,flags,body]=match;
 if(/;base64(?:;|$)/i.test(flags)){assertSize(body.length*3/4);const raw=atob(body);return new Blob([Uint8Array.from(raw,char=>char.charCodeAt(0))],{type:mime});}
 assertSize(src.length);const bytes:number[]=[];const encoder=new TextEncoder();for(let i=0;i<body.length;){if(body[i]==='%'&&/^[0-9a-f]{2}$/i.test(body.slice(i+1,i+3))){bytes.push(parseInt(body.slice(i+1,i+3),16));i+=3;}else{const point=body.codePointAt(i)!;const chunk=String.fromCodePoint(point);bytes.push(...encoder.encode(chunk));i+=chunk.length;}}
 assertSize(bytes.length);return new Blob([new Uint8Array(bytes)],{type:mime});
}
export function blobDataUrl(blob:Blob):Promise<string>{assertSize(blob.size);return new Promise((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(String(reader.result));reader.onerror=()=>reject(reader.error??Error('图片读取失败'));reader.readAsDataURL(blob);});}
async function blobBase64(blob:Blob){const url=await blobDataUrl(blob);return {base64:url.slice(url.indexOf(',')+1),mimeType:blob.type.split(';')[0]||'image/png'};}

export async function prepareMedia(src:string,id:string):Promise<MediaRecord>{
 const existing=registry.get(src);if(existing)return existing;
 if(/^data:image\//i.test(src)){const blob=dataImage(src);return registerMedia({id,url:src,name:'image.'+(blob.type==='image/jpeg'?'jpg':blob.type==='image/svg+xml'?'svg':'png'),mimeType:blob.type,size:blob.size,local:false});}
 if(!id)throw Error('请选择一个会话');
 if(/^https?:\/\//i.test(src)){
  const result=await chatCall<{attachment:ChatAttachment;preview:ChatFilePreviewData}>('media.source',{id,url:src});
  const record=registerPreviewMedia(id,result.preview,false,src,result.attachment);registry.set(src,record);return record;
 }
 const preview=await chatCall<ChatFilePreviewData>('file.preview',{id,path:src});if(preview.kind!=='image')throw Error('该文件不是图片');return registerPreviewMedia(id,preview);
}
async function readBlob(src:string,id?:string):Promise<Blob>{
 if(/^data:image\//i.test(src))return dataImage(src);
 let record=registry.get(src);if(!record){if(!id)throw Error('图片来源已过期，请重新打开');record=await prepareMedia(src,id);}
 let url=record.url;
 if(record.path){const fresh=await chatCall<ChatFilePreviewData>('file.preview',{id:record.id,path:record.path});url=fresh.url;registerPreviewMedia(record.id,fresh,record.local,record.originalURL,record.attachment);}
 const response=await fetch(url);if(!response.ok)throw Error((await response.text()).slice(0,200)||'图片读取失败');const blob=await response.blob();assertSize(blob.size);return blob;
}
async function upload(blob:Blob,id:string,name='image.png'):Promise<MediaRecord>{
 if(!id)throw Error('请选择一个会话');assertSize(blob.size);
 const grant=await chatCall<{url:string}>('media.upload',{id,name});const response=await fetch(grant.url,{method:'POST',headers:{'Content-Type':blob.type||'application/octet-stream'},body:blob});
 if(!response.ok)throw Error((await response.text()).slice(0,300)||'图片保存失败');const value=await response.json() as {attachment:ChatAttachment;preview:ChatFilePreviewData};return registerPreviewMedia(id,value.preview,true,undefined,value.attachment);
}
function sourceRecord(params:{url?:string;filePath?:string}){const record=params.url?registry.get(params.url):[...registry.values()].find(value=>value.path===params.filePath);if(!record)throw Error('图片来源已过期，请重新打开');return record;}
async function nativeSource(src:string,id?:string){const record=registry.get(src)??await prepareMedia(src,id??'');return record.path?record:upload(await readBlob(src,record.id),record.id,record.name);}
export async function copyImageBlob(blob:Blob,id:string,plainText?:string,originalPath?:string){const image=await upload(blob,id,'image.png');await chatMediaCall('copy',{id,path:image.path,plainText,originalPath});}

export const mediaHost={
 platform:'win32' as string,
 async readImageBytes({url}:{url:string}){const record=registry.get(url);return blobBase64(await readBlob(url,record?.id));},
 async readCachedImageAsBase64({url}:{url:string}){return mediaHost.readImageBytes({url});},
 async copyPngToClipboard({png,source,id}:{png:ArrayBuffer;source?:string;id?:string;plainText?:string}){const owner=id||(source?registry.get(source)?.id:'');await copyImageBlob(new Blob([png],{type:'image/png'}),owner??'');},
 async copyMediaToClipboard(params:{url?:string;filePath?:string}){const record=sourceRecord(params);const {burnInAnnotations}=await import('./annotationBurnIn');const png=await burnInAnnotations(await blobBase64(await readBlob(record.url)),[],'image/png');await copyImageBlob(png.blob,record.id,undefined,record.local?record.path:undefined);return {success:true,error:undefined as string|undefined};},
 async showItemInFolder(params:{url?:string;filePath?:string}){const record=sourceRecord(params);if(!record.path)throw Error('图片没有本地文件');await chatCall('file.reveal',{id:record.id,path:record.path});return {success:true,error:undefined as string|undefined};},
 async openExternal(url:string){await chatCall('link.open',{url:registry.get(url)?.originalURL||url});},
 async openMediaWithDefaultApp({url}:{url:string}){const record=await nativeSource(url);await chatCall('file.open',{id:record.id,path:record.path});},
 async saveMediaAs({url,sessionId}:{url:string;sessionId?:string}){const record=await nativeSource(url,sessionId);return chatMediaCall<{canceled:boolean;path?:string}>('save-as',{id:record.id,path:record.path});},
 async cacheImageFromBuffer({sessionId,buffer,mimeType,suggestedName}:{sessionId:string;buffer:Uint8Array|ArrayBuffer;mimeType:string;suggestedName?:string}){const copy=buffer instanceof Uint8Array?new Uint8Array(buffer):buffer;return upload(new Blob([copy],{type:mimeType}),sessionId,suggestedName);},
 async cacheMediaForSession({url,sessionId}:{url:string;sessionId:string}){const blob=await readBlob(url,sessionId);if(!/^image\/(png|jpeg|gif|webp)$/.test(blob.type.split(';')[0]))throw Error('[INVALID_PARAMS] 此图片需要先转为 PNG');const old=registry.get(url),record=await upload(blob,sessionId,old?.name||'image.png');return {...record,ext:'.'+record.name.split('.').pop()};},
};

export async function saveDraft(id:string,draft:{text?:string|null;attachments?:AttachedFile[];quotes?:unknown[];browserComments?:unknown[]},_options?:unknown){
 const old=new Set(drafts.get(id)?.attachments.map(file=>file.id)??[]),files:ChatAttachment[]=[];
 for(const attached of draft.attachments??[]){if(old.has(attached.id))continue;if(!attached.url)throw Error('图片来源缺失');const original=await nativeSource(attached.url,id);let file=original.attachment??{name:original.name,path:original.path!,size:original.size,mimeType:original.mimeType};
  if(attached.annotationStrokes?.length){const {burnInAnnotations}=await import('./annotationBurnIn');const image=await burnInAnnotations(await blobBase64(await readBlob(original.url)),attached.annotationStrokes,'image/png');const output=await upload(image.blob,id,'annotated.png');file={...output.attachment!,imageAnnotation:{sourcePath:original.path!,strokes:attached.annotationStrokes.map(stroke=>({points:[...stroke.points]}))}};}
  files.push(file);
 }
 if(files.length)await attachMedia(id,files);
}
export async function saveEditedImage(id:string,file:ChatAttachment,strokes:AnnotationStroke[]){
 const original=await prepareMedia(file.imageAnnotation?.sourcePath||file.path,id);let result:ChatAttachment;
 if(strokes.length){const {burnInAnnotations}=await import('./annotationBurnIn');const image=await burnInAnnotations(await blobBase64(await readBlob(original.url)),strokes,'image/png');const output=await upload(image.blob,id,file.name.replace(/\.[^.]+$/,'.png'));result={...output.attachment!,imageAnnotation:{sourcePath:original.path!,strokes}};}
 else{const copy=await upload(await readBlob(original.url),id,file.name);result=copy.attachment!;}
 await attachMedia(id,[result],file.path);
}
export async function openFileInSidebarFileBrowser(id:string,path:string){window.dispatchEvent(new CustomEvent('winone:media-open-file',{detail:{id,path}}));}
