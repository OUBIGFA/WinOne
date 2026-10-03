import {moveCrop} from "./capture-crop-resize";
import {useCaptureSelection} from "./use-capture-selection";
import type {CaptureSelectionPort} from "./capture-selection";
import {InlineError} from "../ui/notice";
import {captureSurfaceLayout,type CaptureSurface} from "./capture-surface-layout";
import { useEffect, useLayoutEffect, useRef, useState, type PointerEvent } from "react";
import { Dialog } from "@base-ui/react/dialog";
import { Stage, Layer, Rect, Ellipse, Arrow, Line, Group, Text } from "react-konva";
import { Check, Square, Circle, MoveUpRight, Pencil, PaintBucket, Type, Crop, Grid2X2, Blend, MousePointer2, Undo2, Redo2, Copy, Pin, Save, FolderOpen, Settings2, CircleAlert, X, GripVertical } from "lucide-react";
import {useCaptureToolbarDrag} from "./use-capture-toolbar-drag";
import {CaptureToolbarActions,type ToolbarAction} from "../ui/capture-toolbar-actions";
import {Popover,PopoverTrigger,PopoverContent,PopoverTitle} from "../components/ui/popover";
import {FloatingBoundary} from "../components/ui/floating-boundary";
import {CaptureInlineText,type InlineTextHandle} from "./capture-inline-text";
import { Button } from "../ui/button";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {CaptureCropHandles} from "./capture-crop-handles";
import {captureZoomScale,type CaptureZoom} from "./capture-viewport";
import {useCaptureViewport} from "./use-capture-viewport";
import {useCaptureZoomAnchor} from "./use-capture-zoom-anchor";
import type {CropRect} from "./capture-editor-model";
import {Input} from "../components/ui/input";
import {CaptureDrawingTools,type ToolbarPreferencesPort} from "../ui/capture-drawing-tools";
import { useTranslation } from "../i18n";
import { createHistory, restoreHistory, commitMark, commitCrop, replaceMark, removeMark, gestureMark, imagePoint, undo, redo, type AnnotationDocument, type AnnotationMark, type DrawingTool, type Point, type EditPreview } from "./capture-editor-model";
import {hitTest,moveMark,markBounds} from "./capture-editor-geometry";
import {AnnotationProperties} from "./capture-annotation-properties";
import {editorShortcut,hasEditorTextSelection} from "./capture-editor-shortcuts";
import {useEffectPreview} from "./capture-effect-preview";
import "./capture-editor.css";

const tools = [
  {kind:"select",icon:MousePointer2},
  {kind:"crop",icon:Crop},
  {kind:"rectangle",icon:Square}, {kind:"ellipse",icon:Circle},
  {kind:"arrow",icon:MoveUpRight}, {kind:"pen",icon:Pencil}, {kind:"cover",icon:PaintBucket}, {kind:"text",icon:Type},
  {kind:"mosaic",icon:Grid2X2}, {kind:"blur",icon:Blend},
] as const;

type EditorTool=DrawingTool|"crop"|"select";
interface Gesture {pointer:number;points:Point[];tool:Exclude<EditorTool,"select">;color:string;strokeWidth:number;text:string;fontSize:number;effectSize:number}
function currentMark(gesture:Gesture) {
  const mark=gestureMark(gesture.tool==="crop"?"rectangle":gesture.tool,gesture.points,gesture.color,gesture.strokeWidth);
  if(mark&&gesture.tool==="text")return {...mark,text:gesture.text,fontSize:gesture.fontSize};
  return mark&&(gesture.tool==="mosaic"||gesture.tool==="blur")?{...mark,effectSize:gesture.effectSize}:mark;
}

function Mark({mark}:{mark:AnnotationMark}) {
  const common={stroke:mark.color,strokeWidth:mark.strokeWidth,listening:false,lineJoin:"round" as const,lineCap:"round" as const};
  // Bounds are shown only for an in-progress gesture or unavailable raster.
  if(mark.kind==="mosaic"||mark.kind==="blur")return <Group listening={false}><Rect x={mark.x} y={mark.y} width={mark.width} height={mark.height} stroke="white" strokeWidth={4}/><Rect x={mark.x} y={mark.y} width={mark.width} height={mark.height} stroke="#222222" strokeWidth={2} dash={[8,6]}/></Group>;
  if(mark.kind==="text")return <Group x={mark.x} y={mark.y} clipWidth={mark.width} clipHeight={mark.height} listening={false}><Text text={mark.text} fontSize={mark.fontSize} fontFamily="Segoe UI" width={mark.width} height={mark.height} fill={mark.color} wrap="word"/></Group>;
  if(mark.kind==="pen"||mark.kind==="arrow") {
    const points=mark.points!.flatMap(p=>[p.x,p.y]);
    return mark.kind==="arrow"?<Arrow {...common} points={points} fill={mark.color} pointerLength={mark.strokeWidth*5} pointerWidth={mark.strokeWidth*4}/>:<Line {...common} points={points}/>;
  }
  if(mark.kind==="ellipse")return <Ellipse {...common} x={mark.x!+mark.width!/2} y={mark.y!+mark.height!/2} radiusX={mark.width!/2} radiusY={mark.height!/2}/>;
  if(mark.kind==="cover") {
    // Match the native renderer's outward integer rounding so edge pixels are
    // covered even when the drag starts between source pixels.
    const x=Math.floor(mark.x!),y=Math.floor(mark.y!);
    return <Rect x={x} y={y} width={Math.ceil(mark.x!+mark.width!)-x} height={Math.ceil(mark.y!+mark.height!)-y} fill={mark.color} listening={false}/>;
  }
  return <Rect {...common} x={mark.x} y={mark.y} width={mark.width} height={mark.height}/>;
}

export default function CaptureEditor({image,onSave,onClose,onPreview,onCopy,onPin,initialDocument,onDocumentChange,saveError,closeSignal=0,onKeepEditing,toolbarPort,onSaveAndCopy,onConfirm,selection,surface,onSurfaceReady,onSurfaceError,finalized=false,onSaveAs,selectionRequired=false,sourceImage=false,captureNotice,initialZoom="fit",onRetake,retakeLabel,onStopRetake,retakeStopping=false}:{image:{id:string;width:number;height:number};onSave:(document:AnnotationDocument)=>Promise<boolean>;onClose:()=>void|Promise<void>;onPreview?:(document:AnnotationDocument)=>Promise<EditPreview>;onCopy?:(document:AnnotationDocument)=>Promise<void>;onPin?:(document:AnnotationDocument)=>Promise<void>;initialDocument?:AnnotationDocument;onDocumentChange?:(document:AnnotationDocument)=>void;saveError?:string;closeSignal?:number;onKeepEditing?:()=>void;toolbarPort?:ToolbarPreferencesPort;onSaveAndCopy?:(document:AnnotationDocument)=>Promise<boolean>;onConfirm?:(document:AnnotationDocument)=>Promise<boolean>;selection?:CaptureSelectionPort;surface?:CaptureSurface;selectionRequired?:boolean;sourceImage?:boolean;captureNotice?:string;initialZoom?:CaptureZoom;onRetake?:(document:AnnotationDocument)=>Promise<void>;retakeLabel?:string;onStopRetake?:()=>Promise<void>;retakeStopping?:boolean;onSurfaceReady?:()=>Promise<void>;onSurfaceError?:(error:unknown)=>void;finalized?:boolean;onSaveAs?:(document:AnnotationDocument)=>Promise<boolean>}) {
  const {t}=useTranslation();
  const [history,setHistory]=useState(()=>initialDocument?restoreHistory(initialDocument):createHistory(image.width,image.height));
  const confirmAfterSelection=useRef(false);
  const [selectionOutput,setSelectionOutput]=useState<"confirm"|"save">();
  const selectionState=useCaptureSelection(selection,image.width,image.height,(crop,immediate)=>{setHistory(current=>restoreHistory({...current.document,version:2,crop}));setTool("select");if(confirmAfterSelection.current&&onConfirm){confirmAfterSelection.current=false;setSelectionOutput("confirm");}else if(immediate)setSelectionOutput("save");});
  useEffect(()=>onDocumentChange?.(history.document),[history.document]);
  useEffect(()=>{if(!selection&&!selectionState.busy&&selectionOutput){setSelectionOutput(undefined);void save(selectionOutput==="confirm"?onConfirm:undefined);}},[!!selection,selectionState.busy,selectionOutput]);
  const [tool,setTool]=useState<EditorTool>("rectangle");
  const [color,setColor]=useState("#e23b30");
  const [strokeWidth,setStrokeWidth]=useState(3);
  const [inlineText,setInlineText]=useState<{id:number;mark:AnnotationMark;index?:number}>();
  const inlineInput=useRef<InlineTextHandle>(null),inlineSequence=useRef(0);
  const [fontSize,setFontSize]=useState(24);
  const [effectSize,setEffectSize]=useState(24);
  const [loaded,setLoaded]=useState(false);
  const [failed,setFailed]=useState(false);
  const [actionBusy,setBusy]=useState(false);
  const [outputRequest,setOutputRequest]=useState<{run:(document:AnnotationDocument)=>Promise<void>;started:boolean}>();
  const outputPending=useRef(false);
  const [desktopReady,setDesktopReady]=useState(false);
  const [surfaceChanging,setSurfaceChanging]=useState(false);
  const [screenSize,setScreenSize]=useState(()=>({width:window.innerWidth,height:window.innerHeight}));
  const [toolbarHeight,setToolbarHeight]=useState(190);
  const toolbarRef=useRef<HTMLDivElement>(null),surfaceSent=useRef(false);
  const surfaceActions=useRef({onSurfaceReady,onSurfaceError});surfaceActions.current={onSurfaceReady,onSurfaceError};
  useEffect(()=>{
    if(!surface){setSurfaceChanging(false);return;}
    let failed=false;
    const resized=()=>{
      const p=surface.placement;
      if(surfaceSent.current&&(Math.abs(window.innerWidth*window.devicePixelRatio-p.desktopWidth)>1||Math.abs(window.innerHeight*window.devicePixelRatio-p.desktopHeight)>1)){
        if(!failed){failed=true;setSurfaceChanging(true);surfaceActions.current.onSurfaceError?.(Error(t("capture.displayChanged")));}
        return;
      }
      setScreenSize({width:window.innerWidth,height:window.innerHeight});
    };
    resized();window.addEventListener("resize",resized);return()=>window.removeEventListener("resize",resized);
  },[surface,t]);
  useLayoutEffect(()=>{if(!surface||!toolbarRef.current)return;const measure=()=>setToolbarHeight(toolbarRef.current?.getBoundingClientRect().height||190);measure();const observer=new ResizeObserver(measure);observer.observe(toolbarRef.current);return()=>observer.disconnect();},[surface]);
  useEffect(()=>{
    if(!surface||surfaceSent.current)return;
    if(loaded&&desktopReady&&screenSize.width===window.innerWidth&&screenSize.height===window.innerHeight){surfaceSent.current=true;void surfaceActions.current.onSurfaceReady?.().catch(error=>surfaceActions.current.onSurfaceError?.(error));return;}
    const timer=setTimeout(()=>surfaceActions.current.onSurfaceError?.(Error(t("capture.previewFailed"))),10000);
    return()=>clearTimeout(timer);
  },[surface,loaded,desktopReady,screenSize,t]);
  const [cropPreview,setCropPreview]=useState<CropRect>();
  const surfaceLayout=surface?captureSurfaceLayout(surface,image,screenSize.width,toolbarHeight,undefined,cropPreview??history.document.crop):undefined;
  const {position:toolbarPosition,...toolbarDragHandlers}=useCaptureToolbarDrag(surfaceLayout,toolbarHeight);
  // Both views share the worker's single render slot and preview file lifetime.
  const rasterQueue=useRef(Promise.resolve());
  const effectPreview=useEffectPreview(history.document,image.id,onPreview,rasterQueue);
  const textBackground=useEffectPreview({...history.document,marks:inlineText?.index===undefined?[]:history.document.marks.filter((_,index)=>index!==inlineText.index)},image.id,inlineText?.index===undefined?undefined:onPreview,rasterQueue);
  // Preview rendering must not block another stroke. Exports wait for the
  // render queue so they cannot race the native preview image store.
  const [editingProperties,setEditingProperties]=useState(false);
  const busy=selectionState.busy||actionBusy||!!outputRequest||editingProperties||!!cropPreview||(surfaceChanging&&!finalized);
  const outputBusy=busy||finalized||!!inlineText||effectPreview.pending||effectPreview.rendering;
  const handledClose=useRef(0);
  const [error,setError]=useState("");
  useEffect(()=>{
    if(!outputRequest||outputRequest.started)return;
    if(failed||effectPreview.error||textBackground.error){setOutputRequest(undefined);outputPending.current=false;return;}
    if(inlineText||effectPreview.pending||effectPreview.rendering||textBackground.pending||textBackground.rendering)return;
    // Commit typing first, then wait for both raster queues. The click remains
    // a single intent even if blur starts rendering before its click event.
    outputRequest.started=true;setOutputRequest(undefined);setBusy(true);
    void outputRequest.run(history.document).catch(error=>setError(String(error))).finally(()=>{outputPending.current=false;setBusy(false);});
  },[outputRequest,inlineText,history.document,failed,effectPreview.error,effectPreview.pending,effectPreview.rendering,textBackground.error,textBackground.pending,textBackground.rendering]);
  useEffect(()=>{
    // Keep the saved mark if its editable background cannot be prepared.
    // Showing an incomplete background would hide unrelated annotations.
    if(inlineText&&textBackground.error){setInlineText(undefined);setError(t("capture.previewFailed")+" "+textBackground.error);}
  },[inlineText,textBackground.error,t]);
  const [confirmClose,setConfirmClose]=useState(false);
  const continueEditing=useRef<HTMLButtonElement|null>(null);
  useEffect(()=>{if(confirmClose)continueEditing.current?.focus();},[confirmClose]);
  const [zoom,setZoom]=useState<CaptureZoom>(initialZoom);
  const [pending,setPending]=useState<AnnotationMark|null>(null);
  const [selected,setSelected]=useState<AnnotationMark>();
  const selectedIndex=selected?history.document.marks.indexOf(selected):-1;
  const selectedMark=selectedIndex>=0?selected:undefined;

  useEffect(()=>{if(closeSignal>handledClose.current&&!busy&&!editingProperties){handledClose.current=closeSignal;close();}},[closeSignal,busy,editingProperties]);
  const popup=useRef<HTMLDivElement|null>(null);
  useEffect(()=>{
    if(editingProperties)return;
    const active=document.activeElement;
    // A disabled Delete/Undo control can release focus to the document body.
    // Keep keyboard history commands inside the editor after that transition.
    if(active===document.body||(active instanceof HTMLElement&&active.matches(":disabled")))popup.current?.focus({preventScroll:true});
  },[history.document,selectedIndex,editingProperties,confirmClose,busy]);
  const [pendingMove,setPendingMove]=useState<AnnotationMark>();
  const movingCrop=useRef<{pointer:number;start:Point;crop:CropRect}|null>(null);
  const moving=useRef<{pointer:number;start:Point;index:number;mark:AnnotationMark}|null>(null);
  const [notice,setNotice]=useState("");
  const [toolOptions,setToolOptions]=useState(false),[statusOpen,setStatusOpen]=useState(false);
  useEffect(()=>{if(failed||error||saveError||effectPreview.error||notice||captureNotice)setStatusOpen(true);},[failed,error,saveError,effectPreview.error,notice,captureNotice]);
  useEffect(()=>setNotice(""),[history.document]);
  const [viewport,setViewport]=useState({width:800,height:500});
  const [scrollNode,setScrollNode]=useState<HTMLDivElement|null>(null);
  const [canvasNode,setCanvasNode]=useState<HTMLDivElement|null>(null);
  const gesture=useRef<Gesture|null>(null);
  useEffect(()=>{
    const node=scrollNode;
    if(!node||typeof ResizeObserver==="undefined")return;
    const observer=new ResizeObserver(()=>{
      cancelGesture();
      setViewport({width:Math.max(1,node.clientWidth-32),height:Math.max(1,node.clientHeight-32)});
    });
    observer.observe(node);
    return ()=>observer.disconnect();
  },[scrollNode]);
  // Surface rasters retain physical pixels. Marks above the last effect stay
  // vector-only while drawing; regular previews fit within the available area.
  const scale=surfaceLayout?.scale??captureZoomScale(zoom,image,viewport,window.devicePixelRatio);
  const width=image.width*scale,height=image.height*scale;
  // Apply the new scroll position before measuring the visible vector slice.
  const preserveZoomPosition=useCaptureZoomAnchor(scrollNode,canvasNode,zoom,scale);
  const visible=useCaptureViewport(canvasNode,scrollNode,width,height);
  const crop=history.document.crop;
  const effectTool=tool==="mosaic"||tool==="blur";
  const visibleCrop=selection||selectionState.busy?selectionState.crop:cropPreview??(tool==="crop"&&pending?{x:pending.x!,y:pending.y!,width:pending.width!,height:pending.height!}:crop);
  const selectedBounds=selectedMark?markBounds(pendingMove??selectedMark):undefined;
  const effectRaster=effectPreview.url&&!(pendingMove&&selectedIndex<effectPreview.count)?effectPreview.url:undefined;
  function close() {
    if(busy)return;
    if(onConfirm){void closeEditor();return;}
    if(!finalized&&(initialDocument||history.document.marks.length||crop||inlineText)){setConfirmClose(true);return;}
    void closeEditor();
  }
  async function closeEditor(cancelInteraction=false) {
    if(cancelInteraction?(actionBusy||selectionState.busy||outputPending.current):busy)return;
    setBusy(true);setError("");
    try {await onClose();}catch(error){setError(String(error));}finally{setBusy(false);}
  }
  function cancelGesture() {if(movingCrop.current){movingCrop.current=null;setCropPreview(undefined);}gesture.current=null;setPending(null);moving.current=null;setPendingMove(undefined);}
  function deleteSelected() {
    if(selectedIndex<0)return;
    setHistory(removeMark(history,selectedIndex));setSelected(undefined);
  }
  function point(event:PointerEvent<HTMLDivElement>) {
    return imagePoint(event.clientX,event.clientY,event.currentTarget.getBoundingClientRect(),image.width,image.height);
  }
  function beginText(p:Point,index?:number){
    if(index!==undefined&&(effectPreview.pending||effectPreview.rendering))return;
    if(p.x>=image.width||p.y>=image.height)return;
    const existing=index===undefined?undefined:history.document.marks[index];
    const mark:AnnotationMark=existing??{kind:"text",x:p.x,y:p.y,width:Math.min(360/scale,image.width-p.x),height:Math.min(fontSize*1.5,image.height-p.y),text:"",fontSize,color,strokeWidth:1};
    setInlineText({id:++inlineSequence.current,mark,index});setToolOptions(false);
  }
  function finishText(mark?:AnnotationMark){
    if(!inlineText)return;
    const index=inlineText.index;
    const next=mark?(index===undefined?commitMark(history,mark):replaceMark(history,index,mark)):(index===undefined?history:removeMark(history,index));
    setHistory(next);setInlineText(undefined);setSelected(mark?next.document.marks[index??next.document.marks.length-1]:undefined);
  }
  function move(event:PointerEvent<HTMLDivElement>) {
    const frame=movingCrop.current;
    if(frame&&frame.pointer===event.pointerId){const next=point(event);setCropPreview(moveCrop(frame.crop,next.x-frame.start.x,next.y-frame.start.y,image.width,image.height));return;}
    const movement=moving.current;
    if(movement&&movement.pointer===event.pointerId) {
      const next=point(event);
      setPendingMove(moveMark(movement.mark,next.x-movement.start.x,next.y-movement.start.y,image.width,image.height));
      return;
    }
    const current=gesture.current;
    if(!current||current.pointer!==event.pointerId)return;
    const next=point(event);
    if(current.tool==="pen") {
      const last=current.points.at(-1)!;
      if(Math.hypot(next.x-last.x,next.y-last.y)<0.5)return;
      if(current.points.length>=4096){cancelGesture();setError(t("capture.editLimit"));return;}
      current.points.push(next);
    } else current.points=[current.points[0],next];
    setPending(currentMark(current));
  }
  function requestOutput(run:(document:AnnotationDocument)=>Promise<void>){
    if(outputPending.current||busy||finalized||!loaded||failed||effectPreview.error||textBackground.error||gesture.current||moving.current||movingCrop.current)return;
    outputPending.current=true;setError("");setNotice("");
    try{inlineInput.current?.commit();setOutputRequest({run,started:false});}
    catch(error){outputPending.current=false;setError(String(error));}
  }
  function cancelOutput(){
    if(outputRequest&&!outputRequest.started){setOutputRequest(undefined);outputPending.current=false;}
  }
  function save(action=onSave) {
    requestOutput(async document=>{if(await action(document))await onClose();else setNotice(t("capture.editNotSaved"));});
  }
  function imageAction(action:((document:AnnotationDocument)=>Promise<void>)|undefined,message:string) {
    if(action)requestOutput(async document=>{await action(document);setNotice(t(message));});
  }
  const outputDisabled=busy||finalized||!loaded||failed||!!pending||!!effectPreview.error||!!textBackground.error;
  const toolbarActions:ToolbarAction[]=[
    {id:"save",label:t("capture.saveImage"),icon:Save,primary:true,disabled:outputDisabled,onClick:()=>void save()},
    ...(onSaveAs?[{id:"save-as",label:t("capture.saveAs"),icon:FolderOpen,disabled:outputDisabled,onClick:()=>void save(onSaveAs)}]:[]),
    ...(onCopy?[{id:"copy",label:t("capture.copy"),icon:Copy,disabled:outputDisabled,onClick:()=>void imageAction(onCopy,"capture.copiedResult")}]:[]),
    ...(onPin?[{id:"pin",label:t("capture.pin"),icon:Pin,disabled:outputDisabled,onClick:()=>void imageAction(onPin,"capture.pinnedResult")}]:[]),
    {id:"undo",label:t("capture.undo"),icon:Undo2,disabled:busy||finalized||!history.past.length,onClick:()=>{cancelGesture();setHistory(undo);}},
    {id:"redo",label:t("capture.redo"),icon:Redo2,disabled:busy||finalized||!history.future.length,onClick:()=>{cancelGesture();setHistory(redo);}},
  ];
  if(onConfirm)toolbarActions.unshift({id:"confirm",label:t("capture.confirmCopy"),icon:Check,disabled:outputDisabled,onClick:()=>void save(onConfirm)});
  if(onRetake)toolbarActions.push({id:"retake",label:retakeLabel||t("capture.retake"),disabled:busy||(!finalized&&(outputBusy||!loaded||failed||!!pending)),onClick:()=>{setBusy(true);setError("");void onRetake(history.document).catch(error=>setError(String(error))).finally(()=>setBusy(false));}});
  if(onStopRetake)toolbarActions.unshift({id:"stop",label:t("capture.stopScroll"),busy:retakeStopping,onClick:()=>void onStopRetake().catch(error=>setError(String(error)))});
  return <FloatingBoundary.Provider value={surfaceLayout?.boundary}><Dialog.Root open disablePointerDismissal onOpenChange={(open,event)=>{
    if(open||editingProperties)return;
    if(event.reason==="escape-key"&&(gesture.current||moving.current||movingCrop.current)){cancelGesture();return;}
    if(event.reason==="escape-key"&&confirmClose&&!finalized&&!busy){setConfirmClose(false);onKeepEditing?.();return;}
    close();
  }}>
    <Dialog.Portal>
      <Dialog.Backdrop className="capture-editor-backdrop">{surface&&<img className="capture-surface-background" src={`/capture-preview/${surface.desktopPreviewId}.png`} alt="" onLoad={()=>setDesktopReady(true)} onError={()=>onSurfaceError?.(Error(t("capture.previewFailed")))}/>}</Dialog.Backdrop>
        <Dialog.Popup ref={popup} className={surface?"capture-editor capture-surface":"capture-editor"} onContextMenu={event=>{if(onConfirm){event.preventDefault();if((event.target as HTMLElement).closest('[role="menu"],[data-slot="popover-content"]'))return;event.stopPropagation();cancelGesture();selectionState.cancel();void closeEditor(true);}}} onKeyDownCapture={event=>{
          // Saving belongs to the document, including while inline text has
          // focus. Capture it before the text editor keeps its native commands.
          if(event.key==="Escape"&&outputRequest){event.preventDefault();event.stopPropagation();cancelOutput();return;}
          const command=editorShortcut(event,false);
          if(command!=="save"&&command!=="saveAs")return;
          event.preventDefault();event.stopPropagation();
          if(editingProperties||busy||finalized||gesture.current||moving.current||movingCrop.current)return;
          if(event.nativeEvent.isComposing&&!inlineText)return;
          if(command==="saveAs"){if(onSaveAs)void save(onSaveAs);}
          else void save();
        }} onKeyDown={event=>{
        if(editingProperties)return;
        if(event.key==="Enter"&&onConfirm&&!selection&&!inlineText&&!(event.target as HTMLElement).closest("input,textarea,button,[role=menu]")){event.preventDefault();void save(onConfirm);return;}
        if(finalized){const command=editorShortcut(event,false);if(command&&command!=="copy")event.preventDefault();return;}
        if(event.key==="Escape"&&(gesture.current||moving.current||movingCrop.current)){event.stopPropagation();cancelGesture();return;}
        const target=event.target as HTMLElement;
        if(busy||gesture.current||moving.current||movingCrop.current)return;
        const action=editorShortcut(event,target.matches("input,textarea,select")||!!target.closest('[data-slot="select-trigger"],[data-slot="select-content"]')||target.isContentEditable);
        if(!action||action==="copy"&&!onCopy||action==="delete"&&selectedIndex<0)return;
        if(action==="copy"&&hasEditorTextSelection(popup.current,event.target))return;
        event.preventDefault();
        if(action==="copy")void imageAction(onCopy,"capture.copiedResult");
        else if(action==="save")void save();
        else if(action==="saveAs"){if(onSaveAs)void save(onSaveAs);}
        else if(action==="delete")deleteSelected();
        else setHistory(action==="undo"?undo:redo);
      }}>
        <div ref={toolbarRef} {...toolbarDragHandlers} tabIndex={surface?0:undefined} className={surface?"capture-editor-controls capture-surface-toolbar":"capture-editor-controls"} style={{...(surfaceLayout?{...surfaceLayout.toolbar,...toolbarPosition,cursor:"grab",touchAction:"none"}:{}),...(selection||selectionState.busy?{visibility:"hidden" as const,pointerEvents:"none" as const}:{})}}>
          <Dialog.Title className="sr-only">{t("capture.annotate")}</Dialog.Title>
          <Dialog.Description className="sr-only">{t(tool==="select"?"capture.selectHint":effectTool?"capture.effectHint":tool==="text"?"capture.textHint":tool==="crop"?"capture.cropHint":"capture.drawHint")}{(onConfirm||onSaveAndCopy)&&!finalized&&<> · {t(onConfirm?"capture.doubleConfirmCopy":"capture.doubleSaveCopy")}</>}</Dialog.Description>
          <div className="capture-toolbar-row" aria-label={t("capture.actions")}>
            {surface&&<Button data-toolbar-drag-handle variant="ghost" aria-label={t("capture.dragToolbar")} title={t("capture.dragToolbar")} className="cursor-grab px-1 active:cursor-grabbing" style={{translate:"none"}}><GripVertical size={18}/></Button>}
            <CaptureToolbarActions items={toolbarActions} menuLabel={t("capture.moreActions")}/>
                      {!surface&&<CaptureOptionSelect<CaptureZoom> label={t("capture.zoom")} value={zoom} disabled={busy} onChange={value=>{if(value===zoom)return;cancelGesture();preserveZoomPosition();setZoom(value);}} items={[{value:"fit",label:t("capture.fit")},{value:"width",label:t("capture.fitWidth")},{value:"actual",label:"100%"},{value:"200",label:"200%"},{value:"400",label:"400%"}]}/> }
            {(failed||error||saveError||effectPreview.error||notice||captureNotice)&&<Popover open={statusOpen} onOpenChange={setStatusOpen}>
              <PopoverTrigger render={<Button variant="ghost" aria-label={t("capture.feedback")}/>}><CircleAlert size={18}/></PopoverTrigger>
              <PopoverContent className="max-h-[var(--available-height)] max-w-(--available-width) overflow-y-auto">
                <PopoverTitle>{t("capture.feedback")}</PopoverTitle>
                {(failed||error)&&<InlineError>{failed?t("capture.previewFailed"):error}</InlineError>}
                {saveError&&<InlineError>{saveError}</InlineError>}
                {captureNotice&&<p role="status">{captureNotice}</p>}
                {notice&&<p role="status">{notice}</p>}
                {effectPreview.error&&<><InlineError>{t("capture.previewFailed")}</InlineError><Button disabled={actionBusy} onClick={effectPreview.retry}>{t("capture.refreshPreview")}</Button></>}
              </PopoverContent>
            </Popover>}
            <Popover open={confirmClose&&!finalized} onOpenChange={open=>{if(!open){setConfirmClose(false);onKeepEditing?.();}}}>
              <PopoverTrigger render={<Button disabled={busy} onClick={close} aria-label={t("capture.closeEditor")}/> }><X size={18}/></PopoverTrigger>
              <PopoverContent className="max-w-(--available-width)">
                <PopoverTitle>{t(initialDocument?"capture.closeDraftPrompt":"capture.unsavedAnnotations")}</PopoverTitle>
                <div className="flex flex-wrap gap-2"><Button ref={continueEditing} disabled={busy} onClick={()=>{setConfirmClose(false);onKeepEditing?.();}}>{t("capture.keepEditing")}</Button><Button disabled={busy} variant="danger" onClick={()=>void closeEditor()}>{t(initialDocument?"capture.discardDraft":"capture.discardAnnotations")}</Button></div>
              </PopoverContent>
            </Popover>
          </div>
          <div className="capture-toolbar-row">
            <CaptureDrawingTools items={tools.filter(item=>onPreview||(item.kind!=="mosaic"&&item.kind!=="blur"))} selected={tool} disabled={busy||finalized} port={toolbarPort} onSelect={kind=>{cancelGesture();setSelected(undefined);setTool(kind as EditorTool);setToolOptions(false);}}/>
            <Popover open={toolOptions} onOpenChange={setToolOptions}>
              <PopoverTrigger render={<Button disabled={busy||finalized}/> }><Settings2 size={18}/>{t("capture.toolOptions")}</PopoverTrigger>
              <PopoverContent className="w-80 max-h-[var(--available-height)] max-w-(--available-width) overflow-y-auto">
                <PopoverTitle>{t("capture.toolOptions")}</PopoverTitle>
                <fieldset className="contents" disabled={finalized}>
                        <div className="capture-editor-options">
          {tool==="select"&&<><Button disabled={busy||selectedIndex<0} onClick={()=>{setToolOptions(false);if(selectedMark?.kind==="text")beginText({x:selectedMark.x!,y:selectedMark.y!},selectedIndex);else setEditingProperties(true);}}>{t("capture.editAnnotation")}</Button><Button disabled={busy||selectedIndex<0} onClick={deleteSelected}>{t("capture.deleteAnnotation")}</Button><Button disabled={busy||selectedIndex<0} onClick={()=>setSelected(undefined)}>{t("capture.deselect")}</Button></>}
          {tool!=="crop"&&tool!=="select"&&!effectTool&&<label>{t("capture.color")}<Input type="color" className="w-10 px-1" value={color} disabled={busy} onChange={e=>setColor(e.target.value)}/></label>}
          {tool!=="cover"&&tool!=="text"&&tool!=="crop"&&tool!=="select"&&!effectTool&&<CaptureOptionSelect label={t("capture.strokeWidth")} value={strokeWidth} disabled={busy} onChange={setStrokeWidth} items={[1,3,6,12,24].map(value=>({value,label:`${value} px`}))}/>}
          {effectTool&&<CaptureOptionSelect label={t("capture.effectStrength")} value={effectSize} disabled={busy} onChange={setEffectSize} items={[4,8,12,24,48,64].map(value=>({value,label:String(value)}))}/>}
          {tool==="text"&&<CaptureOptionSelect label={t("capture.fontSize")} value={fontSize} disabled={busy} onChange={setFontSize} items={[12,16,24,32,48,64,96,128,256].map(value=>({value,label:`${value} px`}))}/>}
          {crop&&!selectionRequired&&<Button disabled={busy} onClick={()=>{cancelGesture();setHistory(current=>commitCrop(current,undefined));}}>{t("capture.resetCrop")}</Button>}
        </div>

                </fieldset>
              </PopoverContent>
            </Popover>
          </div>
        </div>
        <div className={surface?"capture-surface-body":"flex min-h-0 flex-1 gap-4"}>
        {selection&&<p role="status" className="pointer-events-none fixed left-4 top-4 z-20 max-w-[calc(100vw-2rem)] rounded-md border bg-popover px-3 py-2 text-sm text-popover-foreground shadow-sm">{t(selection.mode==="window"?"capture.windowSelectionHint":"capture.regionSelectionHint")}</p>}
        {selectionState.error&&<InlineError>{selectionState.error}</InlineError>}
        <div className="capture-editor-scroll" ref={setScrollNode} inert={editingProperties||finalized}>
          <div ref={setCanvasNode} className="capture-editor-canvas" role="group" aria-label={t("capture.drawingArea")} tabIndex={0} style={{width,height,...(surfaceLayout?{position:"absolute" as const,left:surfaceLayout.left,top:surfaceLayout.top,margin:0}:{}),cursor:tool==="select"?"default":"crosshair"}} onContextMenu={e=>e.preventDefault()}
            onDoubleClick={event=>{if(selection||selectionState.busy){confirmAfterSelection.current=true;return;}if(inlineText)return;if(!onConfirm&&(tool==="select"||tool==="text")){const p=imagePoint(event.clientX,event.clientY,event.currentTarget.getBoundingClientRect(),image.width,image.height);const index=hitTest(history.document.marks,p,6/scale);if(index>=0&&history.document.marks[index].kind==="text"){beginText(p,index);return;}}if(!selection&&(onConfirm||onSaveAndCopy))void save(onConfirm||onSaveAndCopy!);}}
            onPointerDown={event=>{
              if(busy||finalized||!loaded||failed||event.button!==0||gesture.current||moving.current||movingCrop.current)return;
              if(inlineText){event.preventDefault();inlineInput.current?.commit();return;}
              if(tool==="text"){event.preventDefault();const p=point(event);const index=hitTest(history.document.marks,p,6/scale);beginText(p,index>=0&&history.document.marks[index].kind==="text"?index:undefined);return;}
              event.preventDefault();setConfirmClose(false);setError("");
              // Drawing takes keyboard ownership from the text field so that
              // Ctrl+Z undoes the mark the user just placed, not their typing.
              event.currentTarget.focus({preventScroll:true});
              event.currentTarget.setPointerCapture(event.pointerId);
              if(selection){selectionState.down(point(event));return;}
              if(tool==="select") {
                const start=point(event),index=hitTest(history.document.marks,start,6/scale);
                setSelected(index>=0?history.document.marks[index]:undefined);
                if(index>=0)moving.current={pointer:event.pointerId,start,index,mark:history.document.marks[index]};
                else if(crop&&start.x>=crop.x&&start.y>=crop.y&&start.x<crop.x+crop.width&&start.y<crop.y+crop.height&&(crop.width<image.width||crop.height<image.height)){movingCrop.current={pointer:event.pointerId,start,crop};}
                else event.currentTarget.releasePointerCapture(event.pointerId);
                return;
              }
              gesture.current={pointer:event.pointerId,points:[point(event)],tool,color,strokeWidth,text:"",fontSize,effectSize};
            }} onPointerMove={event=>{if(selection)selectionState.move(point(event));else move(event);}} onPointerCancel={()=>{selectionState.cancel();cancelGesture();}} onLostPointerCapture={cancelGesture}
            onPointerUp={event=>{
              if(selection){void selectionState.up(point(event));event.currentTarget.releasePointerCapture(event.pointerId);return;}
              const frame=movingCrop.current;
              if(frame&&frame.pointer===event.pointerId){
                const end=point(event),next=moveCrop(frame.crop,end.x-frame.start.x,end.y-frame.start.y,image.width,image.height);
                cancelGesture();event.currentTarget.releasePointerCapture(event.pointerId);
                if(next.x!==frame.crop.x||next.y!==frame.crop.y)setHistory(current=>commitCrop(current,next));
                return;
              }
              const movement=moving.current;
              if(movement&&movement.pointer===event.pointerId) {
                const end=point(event);
                const next=replaceMark(history,movement.index,moveMark(movement.mark,end.x-movement.start.x,end.y-movement.start.y,image.width,image.height));
                setHistory(next);setSelected(next.document.marks[movement.index]);
                cancelGesture();event.currentTarget.releasePointerCapture(event.pointerId);
                return;
              }
              if(gesture.current?.pointer!==event.pointerId)return;
              move(event);
              const current=gesture.current;
              if(current) {
                const mark=currentMark(current);
                if(mark)try {
                  const next=current.tool==="crop"?commitCrop(history,{x:mark.x!,y:mark.y!,width:mark.width!,height:mark.height!}):commitMark(history,mark);
                  setHistory(next);
                }catch {setError(t("capture.editLimit"));}
              }
              cancelGesture();
              event.currentTarget.releasePointerCapture(event.pointerId);
            }}>
            <img src={`/${surface||sourceImage?"capture-source":"capture-preview"}/${encodeURIComponent(image.id)}.png`} alt={t("capture.preview")} draggable={false} onLoad={()=>{setLoaded(true);setFailed(false);}} onError={()=>{setFailed(true);setLoaded(false);if(surface)onSurfaceError?.(Error(t("capture.previewFailed")));}}/>
            {loaded&&!failed&&effectRaster&&<img className="capture-effect-raster" src={effectRaster} alt="" draggable={false}/>}
            {loaded&&!failed&&visible.width>0&&visible.height>0&&<div className="capture-editor-vectors" aria-hidden="true"><div className="capture-editor-vector-slice" style={{position:"absolute",left:visible.x,top:visible.y,width:visible.width,height:visible.height}}><Stage width={visible.width} height={visible.height}><Layer listening={false} x={-visible.x} y={-visible.y} scaleX={scale} scaleY={scale}>{history.document.marks.map((mark,index)=>(inlineText?.index===index&&!effectRaster)||effectRaster&&index<effectPreview.count?null:<Mark key={index} mark={mark===selectedMark&&pendingMove?pendingMove:mark}/>)}{pending&&tool!=="crop"&&<Mark mark={pending}/>}</Layer></Stage></div></div>}
            {visibleCrop&&<div className="capture-crop-mask" aria-hidden="true">
              <div style={{left:0,top:0,width,height:visibleCrop.y*scale}}/>
              <div style={{left:0,top:(visibleCrop.y+visibleCrop.height)*scale,width,bottom:0}}/>
              <div style={{left:0,top:visibleCrop.y*scale,width:visibleCrop.x*scale,height:visibleCrop.height*scale}}/>
              <div style={{left:(visibleCrop.x+visibleCrop.width)*scale,top:visibleCrop.y*scale,right:0,height:visibleCrop.height*scale}}/>
              <span style={{left:visibleCrop.x*scale,top:visibleCrop.y*scale,width:visibleCrop.width*scale,height:visibleCrop.height*scale}}/>
            </div>}
            {!selection&&visibleCrop&&loaded&&!failed&&(tool==="crop"||surface||selectionRequired)&&!(selectionRequired&&visibleCrop.x===0&&visibleCrop.y===0&&visibleCrop.width===image.width&&visibleCrop.height===image.height)&&<CaptureCropHandles crop={visibleCrop} scale={scale} width={image.width} height={image.height}
              disabled={actionBusy||editingProperties||surfaceChanging||finalized} onPreview={setCropPreview}
              onCommit={next=>{setCropPreview(undefined);setHistory(current=>commitCrop(current,next));}}/>}
            {selectedBounds&&<div className="capture-annotation-selection" aria-hidden="true" style={{left:selectedBounds.x*scale,top:selectedBounds.y*scale,width:Math.max(1,selectedBounds.width*scale),height:Math.max(1,selectedBounds.height*scale)}}/>}
            {effectPreview.pending&&<div className="capture-effect-status" role="status">{t("capture.working")}</div>}
            {inlineText&&<CaptureInlineText key={inlineText.id} ref={inlineInput} mark={inlineText.mark} scale={scale} image={image} label={t("capture.inlineText")} hint={t("capture.inlineTextHint")}
              preparing={inlineText.index!==undefined&&!!onPreview&&textBackground.pending}
              background={inlineText.index===undefined||!onPreview?undefined:textBackground.url??`/${surface||sourceImage?"capture-source":"capture-preview"}/${image.id}.png`}
              onCommit={finishText} onCancel={()=>{cancelOutput();setInlineText(undefined);}}/>}
          </div>
        </div>
        {editingProperties&&selectedMark&&<AnnotationProperties floating={surfaceLayout?.properties} mark={selectedMark} onClose={()=>setEditingProperties(false)} onApply={mark=>{
          const next=replaceMark(history,selectedIndex,mark);
          setHistory(next);setSelected(next.document.marks[selectedIndex]);
        }}/>}
        </div>
      </Dialog.Popup>
    </Dialog.Portal>
  </Dialog.Root></FloatingBoundary.Provider>;
}
