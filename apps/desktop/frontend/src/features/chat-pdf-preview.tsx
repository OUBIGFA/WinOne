import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import {getDocument,GlobalWorkerOptions} from 'pdfjs-dist';
import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url';
import {EventBus,PDFViewer,PDFLinkService,PDFFindController} from 'pdfjs-dist/web/pdf_viewer.mjs';
import {Dialog,Input,Button,InlineNotice,type DesktopMenuItem} from 'chatgpt-desktop-kit';
import {useTranslation} from '../i18n';
import viewerStyles from 'pdfjs-dist/web/pdf_viewer.css?inline';
import previewStyles from './chat-pdf-preview.css?inline';

GlobalWorkerOptions.workerSrc=workerUrl;

/** PDF.js supplies page layout, text selection, links, forms and lazy rendering.
 * The pane owns all visible controls; PDF documents never run embedded scripts. */
export function ChatPDFPreview({url,onActions}:{url:string;onActions:(items:DesktopMenuItem[])=>void}){
 const {t}=useTranslation(),container=useRef<HTMLDivElement>(null),pages=useRef<HTMLDivElement>(null),controller=useRef<{viewer:PDFViewer;bus:EventBus}|undefined>(undefined),callback=useRef(onActions);callback.current=onActions;
 const [state,setState]=useState({page:1,pages:0,scale:1}),[error,setError]=useState(''),[rendered,setRendered]=useState(false),[search,setSearch]=useState(false),[query,setQuery]=useState('');
 useLayoutEffect(()=>{
  // PDF.js checks computed positioning during construction. Install its styles
  // before that check without injecting <style> text rejected by desktop CSP.
  const document=container.current!.ownerDocument,sheet=new CSSStyleSheet();
  sheet.replaceSync(`${viewerStyles}\n${previewStyles}`);
  document.adoptedStyleSheets=[...document.adoptedStyleSheets,sheet];
  return()=>{document.adoptedStyleSheets=document.adoptedStyleSheets.filter(current=>current!==sheet);};
 },[]);
 useEffect(()=>{
  let alive=true;setError('');setRendered(false);
  const bus=new EventBus(),links=new PDFLinkService({eventBus:bus,externalLinkTarget:2}),find=new PDFFindController({eventBus:bus,linkService:links});
  const viewer=new PDFViewer({container:container.current!,viewer:pages.current!,eventBus:bus,linkService:links,findController:find,imageResourcesPath:new URL('/pdfjs/images/',location.href).href,maxCanvasPixels:8*1024*1024});
  links.setViewer(viewer);controller.current={viewer,bus};
  const update=()=>{if(alive)setState({page:viewer.currentPageNumber,pages:viewer.pagesCount,scale:viewer.currentScale||1});};
  bus.on('pagesinit',()=>{viewer.currentScaleValue='page-width';update();});bus.on('pagechanging',update);bus.on('scalechanging',update);
  bus.on('pagerendered',(event:{error?:unknown})=>{if(!alive)return;if(event.error)setError(String(event.error));else setRendered(true);});
  const loading=getDocument({url,cMapUrl:new URL('/pdfjs/cmaps/',location.href).href,cMapPacked:true,standardFontDataUrl:new URL('/pdfjs/standard_fonts/',location.href).href,wasmUrl:new URL('/pdfjs/wasm/',location.href).href,iccUrl:new URL('/pdfjs/iccs/',location.href).href});
  void loading.promise.then(doc=>{if(alive){links.setDocument(doc);viewer.setDocument(doc);}}).catch(error=>{if(alive)setError(String(error));});
  const observer=new ResizeObserver(()=>{if(container.current?.clientWidth&&viewer.pagesCount){if(viewer.currentScaleValue==='page-width')viewer.currentScaleValue='page-width';viewer.update();}});observer.observe(container.current!);
  return()=>{alive=false;observer.disconnect();controller.current=undefined;viewer.setDocument(null!);links.setDocument(null!);void loading.destroy();};
 },[url]);
 useEffect(()=>{
  const run=(action:(viewer:PDFViewer)=>void)=>{const viewer=controller.current?.viewer;if(viewer&&viewer.pagesCount)action(viewer);};
  callback.current([
   {id:'pdf.page',label:t('chat.pdfPage',{page:state.page,pages:state.pages}),heading:true,separator:true},
   {id:'pdf.previous',label:t('chat.pdfPrevious'),disabled:state.page<=1,onSelect:()=>run(viewer=>{viewer.currentPageNumber--;})},
   {id:'pdf.next',label:t('chat.pdfNext'),disabled:state.page>=state.pages,onSelect:()=>run(viewer=>{viewer.currentPageNumber++;})},
   {id:'pdf.find',label:t('chat.pdfFind'),disabled:!state.pages,onSelect:()=>setSearch(true)},
   {id:'pdf.zoomIn',label:t('chat.zoomIn'),disabled:!state.pages,onSelect:()=>run(viewer=>{viewer.currentScale=Math.min(10,viewer.currentScale*1.25);})},
   {id:'pdf.zoomOut',label:t('chat.zoomOut'),disabled:!state.pages,onSelect:()=>run(viewer=>{viewer.currentScale=Math.max(.1,viewer.currentScale/1.25);})},
   {id:'pdf.fit',label:t('chat.pdfFit'),disabled:!state.pages,onSelect:()=>run(viewer=>{viewer.currentScaleValue='page-width';})},
   {id:'pdf.rotate',label:t('chat.pdfRotate'),disabled:!state.pages,onSelect:()=>run(viewer=>{viewer.pagesRotation=(viewer.pagesRotation+90)%360;})},
  ]);
 },[state,t]);
 function find(){controller.current?.bus.dispatch('find',{source:controller.current.viewer,type:'',query,caseSensitive:false,entireWord:false,highlightAll:true,findPrevious:false});setSearch(false);}
 return <div className="chat-pdf-preview" data-pdf-rendered={rendered||undefined}>
  {error&&<InlineNotice tone="danger" title={error}/>}
  <div ref={container} className="chat-pdf-scroll" aria-label={t('chat.pdfDocument')}><div ref={pages} className="pdfViewer"/></div>
  <Dialog open={search} onClose={()=>setSearch(false)} title={t('chat.pdfFind')} footer={<Button disabled={!query.trim()} onClick={find}>{t('chat.pdfFind')}</Button>}><Input autoFocus value={query} aria-label={t('chat.pdfFind')} onChange={event=>setQuery(event.target.value)} onKeyDown={event=>{if(event.key==='Enter'&&!event.nativeEvent.isComposing)find();}}/></Dialog>
 </div>;
}
