// Cindy block-copy lifecycle with the WebView clipboard and a host annotation
// callback. File previews have no message attachment destination.
import {createContext,useContext,useEffect,useRef,useState,type ReactNode} from 'react';
import {copyPngBlobToClipboard} from './rasterizeToImage';
export interface CopyAsImagePayload{blob:Blob;plainText?:string}
export const MarkdownMediaContext=createContext<{annotate?:(payload:CopyAsImagePayload)=>Promise<void>;copy?:(payload:CopyAsImagePayload)=>Promise<void>}>({});
export function useCopyAsImage(getPayload:()=>Promise<CopyAsImagePayload>):{copiedImage:boolean;copyAsImage:()=>Promise<void>;canAnnotate:boolean;openAnnotate:()=>Promise<void>;annotateNode:ReactNode}{
 const media=useContext(MarkdownMediaContext),[copiedImage,setCopiedImage]=useState(false),[error,setError]=useState('');
 const timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined),alive=useRef(true);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;if(timer.current)clearTimeout(timer.current);};},[]);
 async function copyAsImage(){try{const payload=await getPayload();if(media.copy)await media.copy(payload);else await copyPngBlobToClipboard(payload.blob,payload.plainText);if(!alive.current)return;setError('');setCopiedImage(true);if(timer.current)clearTimeout(timer.current);timer.current=setTimeout(()=>setCopiedImage(false),1500);}catch(error){if(alive.current)setError(String(error));}}
 async function openAnnotate(){try{if(media.annotate)await media.annotate(await getPayload());}catch(error){if(alive.current)setError(String(error));}}
 return {copiedImage,copyAsImage,canAnnotate:!!media.annotate,openAnnotate,annotateNode:error?<p role="alert" className="text-sm text-destructive">{error}</p>:null};
}
