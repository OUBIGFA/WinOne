import {useEffect,useMemo,useRef,useState} from "react";
import type {AnnotationDocument,AnnotationMark,EditPreview} from "./capture-editor-model";

// Finished marks use the same renderer as saved output. Live gestures remain
// vectors while drawing; crop stays a mask over the immutable source frame.
export function rasterDocument(document:AnnotationDocument):AnnotationDocument|undefined {
  if(!document.marks.length)return;
  return {version:2,width:document.width,height:document.height,marks:document.marks};
}

export function useEffectPreview(document:AnnotationDocument,sourceId:string,render?: (document:AnnotationDocument)=>Promise<EditPreview>,sharedQueue?:{current:Promise<void>}) {
  const prefix=useMemo(()=>rasterDocument(document),[document]);
  const key=useMemo(()=>JSON.stringify(prefix)??"",[prefix]);
  const renderRef=useRef(render);renderRef.current=render;
  const localQueue=useRef(Promise.resolve());
  const queue=sharedQueue??localQueue;
  const [attempt,setAttempt]=useState(0);
  const [rendering,setRendering]=useState(false);
  const [state,setState]=useState<{key:string;sourceId:string;error?:string}>();
  const [raster,setRaster]=useState<{key:string;sourceId:string;url:string;marks:AnnotationMark[]}>();
  const urls=useRef(new Set<string>());
  const release=(url:string)=>{if(urls.current.delete(url))URL.revokeObjectURL(url);};
  const available=!!render;
  // Only an unchanged prefix can remain under newly appended vectors. Undo,
  // replacement, and source changes must never show obsolete effects.
  const compatible=useMemo(()=>available&&!!prefix&&raster?.sourceId===sourceId&&
    (raster.key===key||JSON.stringify(prefix.marks.slice(0,raster.marks.length))===JSON.stringify(raster.marks)),[available,prefix,raster,sourceId,key]);
  useEffect(()=>{if(!compatible)setRaster(undefined);},[key,sourceId,available]);
  useEffect(()=>()=>{if(raster)release(raster.url);},[raster]);
  // Also own decoded images awaiting React's commit, so an intervening unmount
  // cannot leak an allocation that never reached the displayed raster state.
  useEffect(()=>()=>{for(const url of urls.current)release(url);},[]);
  useEffect(()=>{
    if(!key||!available)return;
    let disposed=false,url:string|undefined;
    const controller=new AbortController();
    setState({key,sourceId});
    // Serialize render + image retrieval. Native storage retains only the most
    // recent preview, so a newer render must not retire an image still loading.
    queue.current=queue.current.then(async()=>{
      if(disposed)return;
      setRendering(true);
      try {
        const document:AnnotationDocument=JSON.parse(key);
        const preview=await renderRef.current!(document);
        if(disposed)return;
        if(!/^[a-f0-9]{32}$/.test(preview.id)||preview.sourceId!==sourceId||preview.width!==document.width||preview.height!==document.height)throw Error("Invalid effect preview");
        const response=await fetch(`/capture-edit-preview/${preview.id}.png`,{signal:controller.signal});
        if(!response.ok)throw Error(`Preview HTTP ${response.status}`);
        const blob=await response.blob();
        if(disposed)return;
        // Match the native asset limit; edit rasters retain original pixels.
        if(blob.size>512*1024*1024||blob.type!=="image/png")throw Error("Invalid preview image");
        url=URL.createObjectURL(blob);urls.current.add(url);
        const image=new Image();image.src=url;
        await image.decode();
        if(disposed)return;
        setRaster({key,sourceId,url,marks:document.marks});url=undefined;
        setState({key,sourceId});
      } catch(error) {
        if(url){release(url);url=undefined;}
        if(!disposed)setState({key,sourceId,error:String(error)});
      } finally {
        setRendering(false);
      }
    });
    return ()=>{
      disposed=true;controller.abort();
      if(url){release(url);url=undefined;}
    };
  },[key,sourceId,available,attempt]);
  const current=state?.key===key&&state.sourceId===sourceId?state:undefined;
  const displayed=compatible?raster:undefined;
  return {
    url:displayed?.url,
    count:displayed?.marks.length??0,
    pending:available&&!!key&&displayed?.key!==key&&!current?.error,
    rendering,
    error:available&&key?current?.error:undefined,
    retry:()=>setAttempt(value=>value+1),
  };
}
