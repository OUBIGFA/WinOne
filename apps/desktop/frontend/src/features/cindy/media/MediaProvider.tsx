import {lazy,Suspense,useCallback,useEffect,useMemo,useRef,useState,type ReactNode} from 'react';
import {useTranslation} from 'react-i18next';
import {ConfirmDialogProvider} from '../panel-ui/confirm-dialog-provider';
import {Spinner} from '../panel-ui/spinner';
import {MarkdownMediaContext} from '../markdown/useCopyAsImage';
import {MEDIA_OPEN_EVENT,MEDIA_OVERLAY_EVENT,MediaFeedback,MediaScope,blobDataUrl,copyImageBlob,mediaRecord,prepareMedia,releaseTransientMedia,saveEditedImage,toast,type MediaOpen} from './host';
import type {AnnotationStroke} from './lightboxAnnotations';
const ImageLightbox=lazy(()=>import('./ImageLightbox').then(module=>({default:module.ImageLightbox})));
type OpenImage=MediaOpen&{edit?:boolean;resolved:string;fallback?:string;initialStrokes?:AnnotationStroke[]};

// One host serves message images, CodeMirror image widgets and rasterized
// tables/formulae/diagrams. Opening a viewer never sends a model request.
export function MediaProvider({id,cwd,children}:{id:string;cwd?:string;children:ReactNode}){
 const {t}=useTranslation(),[image,setImage]=useState<OpenImage>(),[loading,setLoading]=useState(false),sequence=useRef(0),onClosed=useRef<(()=>void)|undefined>(undefined);
 const close=useCallback(()=>{sequence.current++;setLoading(false);setImage(current=>{if(current)releaseTransientMedia(current.resolved);return undefined;});const callback=onClosed.current;onClosed.current=undefined;callback?.();},[]);
 const open=useCallback(async(detail:MediaOpen&{edit?:boolean})=>{
  const token=++sequence.current;onClosed.current=detail.onClose;setLoading(true);
  try{
   const visible=await prepareMedia(detail.src,detail.id||id);let resolved=visible.url,attachment=detail.attachment,initialStrokes=attachment?.imageAnnotation?.strokes;
   if(attachment?.imageAnnotation){try{resolved=(await prepareMedia(attachment.imageAnnotation.sourcePath,detail.id||id)).url;}catch{initialStrokes=[];attachment={...attachment,imageAnnotation:undefined};toast.warning(t('chat.media.originalMissing'));}}
   if(sequence.current===token)setImage({...detail,attachment,id:detail.id||id,resolved,fallback:resolved===visible.url?undefined:visible.url,initialStrokes});
  }catch(error){if(sequence.current===token){toast.error(String(error));close();}}
  finally{if(sequence.current===token)setLoading(false);}
 },[id,close,t]);
 useEffect(()=>{
  const explicit=(event:Event)=>{const detail=(event as CustomEvent<MediaOpen>).detail;if(detail?.src)void open(detail);};
  const editor=(event:Event)=>{const detail=(event as CustomEvent<{src?:string;url?:string;alt?:string}>).detail;const src=detail?.src||detail?.url;if(src)void open({id:mediaRecord(src)?.id||id,src,alt:detail.alt});};
  window.addEventListener(MEDIA_OPEN_EVENT,explicit);window.addEventListener('xdt-open-md-image-lightbox',editor);
  return()=>{window.removeEventListener(MEDIA_OPEN_EVENT,explicit);window.removeEventListener('xdt-open-md-image-lightbox',editor);};
 },[id,open]);
 useEffect(()=>{window.dispatchEvent(new CustomEvent(MEDIA_OVERLAY_EVENT,{detail:!!image||loading}));return()=>{window.dispatchEvent(new CustomEvent(MEDIA_OVERLAY_EVENT,{detail:false}));};},[image,loading]);
 useEffect(()=>{if(!loading)return;const key=(event:KeyboardEvent)=>{if(event.key==='Escape'){event.preventDefault();close();}};window.addEventListener('keydown',key,true);return()=>window.removeEventListener('keydown',key,true);},[loading,close]);
 const media=useMemo(()=>({
  annotate:id?async({blob}:{blob:Blob})=>open({id,src:await blobDataUrl(blob),autoAnnotate:true}):undefined,
  copy:id?async({blob,plainText}:{blob:Blob;plainText?:string})=>copyImageBlob(blob,id,plainText):undefined,
 }),[id,open]);
 return <MediaScope id={id} cwd={cwd}><MarkdownMediaContext.Provider value={media}>{children}<MediaFeedback/>
  {loading&&<div role="dialog" aria-modal="true" aria-label={t('chat.media.imagePreview')} className="cindy-panel-popup fixed inset-0 z-[9999] flex items-center justify-center bg-[var(--overlay-lightbox)]" onClick={close}><Spinner size={24}/></div>}
  {image&&<MediaScope id={image.id} cwd={cwd}><ConfirmDialogProvider><Suspense fallback={<div className="cindy-panel-popup fixed inset-0 z-[9999] flex items-center justify-center bg-[var(--overlay-lightbox)]"><Spinner size={24}/></div>}>
   <ImageLightbox src={image.resolved} sessionId={image.id} enableGallery={image.gallery} autoAnnotate={image.autoAnnotate} initialStrokes={image.edit?undefined:image.initialStrokes} annotationFallbackSrc={image.fallback} annotationEdit={image.edit&&image.attachment?{initialStrokes:image.initialStrokes,onSave:async({strokes})=>{await saveEditedImage(image.id,image.attachment!,strokes);}}:undefined} onClose={close}/>
  </Suspense></ConfirmDialogProvider></MediaScope>}
 </MarkdownMediaContext.Provider></MediaScope>;
}
