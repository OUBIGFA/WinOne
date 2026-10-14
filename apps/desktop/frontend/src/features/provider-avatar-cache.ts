import {captureCall} from "../bridge";

export const maximumAvatarBytes=1<<20;
const chunkBytes=256<<10;
const references=/^file:[0-9a-f]{16}\.(png|jpg|gif|webp|ico|svg)$/;
const mimeTypes=new Set(["image/png","image/jpeg","image/gif","image/webp","image/x-icon","image/svg+xml"]);
const cache=new Map<string,Promise<string>>();
let revision=0;
const listeners=new Set<()=>void>();
export const providerAvatarRevision=()=>revision;
export function subscribeProviderAvatars(listener:()=>void){listeners.add(listener);return()=>{listeners.delete(listener);};}
// Successful hash-addressed pictures remain reusable. Explicit refresh also
// wakes mounted icons whose earlier read failed before the service was ready.
export function refreshProviderAvatars(){revision++;for(const listener of listeners)listener();}
export const isStoredAvatar=(icon?:string)=>!!icon&&references.test(icon);

// Magpie's file picker sends base64 JSON because WebViews cannot reliably send
// File bodies. Chunk only at the Host boundary; Magpie receives the same bytes.
function base64(bytes:Uint8Array){let binary="";for(let i=0;i<bytes.length;i+=0x8000)binary+=String.fromCharCode(...bytes.subarray(i,i+0x8000));return btoa(binary);}
export async function storeProviderAvatar(bytes:Uint8Array,signal?:AbortSignal){
 if(!bytes.length||bytes.length>maximumAvatarBytes)throw Error("请选择不超过 1 MB 的图片");
 let upload="";
 try{
  for(let offset=0;offset<bytes.length;offset+=chunkBytes){
   signal?.throwIfAborted();
   const part=bytes.subarray(offset,offset+chunkBytes);
   const result=await captureCall<{upload?:string;offset?:number;icon?:string}>("ai.icons.store",{data:base64(part),size:bytes.length,offset,upload});
   upload=result.upload??"";
   if(offset+part.length===bytes.length){if(!isStoredAvatar(result.icon))throw Error("头像服务未返回有效图片");return result.icon!;}
   if(!upload||result.offset!==offset+part.length)throw Error("图片传输不完整，请重新选择");
  }
  throw Error("图片传输不完整，请重新选择");
 }finally{if(upload)await captureCall("ai.icons.cancel",{upload});}
}

async function readAvatar(icon:string){
 let binary="",size=0,mimeType="";
 do{
  const chunk=await captureCall<{data:string;size:number;mimeType:string}>("ai.icons.read",{icon,offset:binary.length});
  if(!Number.isInteger(chunk.size)||chunk.size<=0||chunk.size>maximumAvatarBytes||!mimeTypes.has(chunk.mimeType)||(size&&size!==chunk.size)||(mimeType&&mimeType!==chunk.mimeType))throw Error("头像数据无效");
  size=chunk.size;mimeType=chunk.mimeType;
  const part=atob(chunk.data);if(!part.length||part.length>chunkBytes||binary.length+part.length>size)throw Error("头像数据不完整");binary+=part;
 }while(binary.length<size);
 return `data:${mimeType};base64,${btoa(binary)}`;
}

// Immutable hashes share one read across menus, the overview and Chat. Failures
// are evicted so reopening the view can retry after an interrupted connection.
export function loadProviderAvatar(icon:string):Promise<string>{
 if(!isStoredAvatar(icon))return Promise.reject(Error("头像引用无效"));
 const existing=cache.get(icon);if(existing)return existing;
 if(cache.size>=64)cache.delete(cache.keys().next().value!);
 const promise=readAvatar(icon).catch(error=>{cache.delete(icon);throw error;});cache.set(icon,promise);return promise;
}
