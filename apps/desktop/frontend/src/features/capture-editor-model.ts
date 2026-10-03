export type DrawingTool = "rectangle" | "ellipse" | "arrow" | "pen" | "cover" | "text" | "mosaic" | "blur";
export interface Point { x:number; y:number }
export interface AnnotationMark {
  kind:DrawingTool;
  x?:number; y?:number; width?:number; height?:number;
  color:string; strokeWidth:number; points?:Point[];
  text?:string; fontSize?:number;
  effectSize?:number;
}
export interface CropRect {x:number;y:number;width:number;height:number}
export interface EditPreview {id:string;sourceId:string;width:number;height:number}
export interface AnnotationDocument { version:1|2; width:number; height:number; marks:AnnotationMark[]; crop?:CropRect }
export interface History { document:AnnotationDocument; past:AnnotationDocument[]; future:AnnotationDocument[] }

// Preview dimensions never become export dimensions. Pointer coordinates are
// converted once at the input boundary and constrained to the source image.
export function imagePoint(clientX:number,clientY:number,view:{left:number;top:number;width:number;height:number},width:number,height:number):Point {
  if (![clientX,clientY,view.left,view.top,view.width,view.height,width,height].every(Number.isFinite)||view.width<=0||view.height<=0||width<=0||height<=0) throw new Error("Invalid image viewport");
  // Snap to source pixels, avoiding different float rounding at right/bottom
  // edges between JavaScript doubles and the native renderer's float values.
  return {x:Math.round(Math.max(0,Math.min(width,(clientX-view.left)*width/view.width))),y:Math.round(Math.max(0,Math.min(height,(clientY-view.top)*height/view.height)))};
}

export function gestureMark(kind:DrawingTool,points:Point[],color:string,strokeWidth:number):AnnotationMark|null {
  if(points.length<2)return null;
  const first=points[0],last=points[points.length-1];
  if(kind==="arrow"||kind==="pen") {
    if(!points.some(p=>p.x!==first.x||p.y!==first.y))return null;
    return {kind,points:kind==="arrow"?[first,last]:points,color,strokeWidth};
  }
  const width=Math.abs(last.x-first.x),height=Math.abs(last.y-first.y);
  if(width<1||height<1)return null;
  return {kind,x:Math.min(first.x,last.x),y:Math.min(first.y,last.y),width,height,color,strokeWidth};
}

export function createHistory(width:number,height:number):History {
  if(!Number.isInteger(width)||!Number.isInteger(height)||width<=0||height<=0)throw new Error("Invalid image size");
  return {document:{version:1,width,height,marks:[]},past:[],future:[]};
}

export function restoreHistory(document:AnnotationDocument):History {
  if(![1,2].includes(document.version)||!Array.isArray(document.marks)||document.version===1&&document.crop)throw new Error("Invalid annotation document");
  const history=createHistory(document.width,document.height);
  for(const mark of document.marks)history.document.marks.push(validatedMark(history.document,mark));
  const restored=document.crop?commitCrop(history,document.crop):history;
  return {document:{...restored.document,version:document.version},past:[],future:[]};
}

export function commitMark(history:History,mark:AnnotationMark):History {
  const copy=validatedMark(history.document,mark);
  return commitDocument(history,{...history.document,marks:[...history.document.marks,copy]});
}

function validatedMark(document:AnnotationDocument,mark:AnnotationMark):AnnotationMark {
  const {width,height,marks}=document;
  if(!["rectangle","ellipse","arrow","pen","cover","text","mosaic","blur"].includes(mark.kind)||marks.length>=512||!/^#[0-9a-f]{6}$/i.test(mark.color)||!Number.isFinite(mark.strokeWidth)||mark.strokeWidth<1||mark.strokeWidth>64)throw new Error("Invalid annotation");
  if(mark.kind==="arrow"||mark.kind==="pen") {
    const points=mark.points;
    if(!points||points.length<2||points.length>4096||mark.kind==="arrow"&&(points.length!==2||points[0].x===points[1].x&&points[0].y===points[1].y))throw new Error("Invalid stroke");
    if(points.some(p=>!Number.isFinite(p.x)||!Number.isFinite(p.y)||p.x<0||p.y<0||p.x>width||p.y>height)||marks.reduce((n,m)=>n+(m.points?.length??0),points.length)>16384)throw new Error("Invalid stroke");
  } else {
    const {x,y,width:w,height:h}=mark;
    if(x===undefined||y===undefined||w===undefined||h===undefined||![x,y,w,h].every(Number.isFinite)||x<0||y<0||w<1||h<1||x+w>width||y+h>height)throw new Error("Invalid annotation bounds");
    if(mark.kind==="text"&&(typeof mark.text!=="string"||!mark.text.trim()||mark.text.length>4096||mark.fontSize===undefined||!Number.isFinite(mark.fontSize)||mark.fontSize<8||mark.fontSize>256))throw new Error("Invalid text annotation");
    if((mark.kind==="mosaic"||mark.kind==="blur")&&(mark.effectSize===undefined||!Number.isInteger(mark.effectSize)||mark.effectSize<2||mark.effectSize>64))throw new Error("Invalid effect strength");
  }
  // Marks are immutable after insertion. History shares their arrays rather
  // than copying image pixels; a bounded stack caps long-session overhead.
  return {...mark, ...(mark.points?{points:mark.points.map(p=>({...p}))}:{})};
}

// Replacement validates the whole remaining point budget but retains layer
// order; changing an early effect must not move it above later annotations.
export function replaceMark(history:History,index:number,mark:AnnotationMark):History {
  const marks=history.document.marks;
  if(!Number.isInteger(index)||index<0||index>=marks.length)throw new Error("Invalid annotation index");
  const copy=validatedMark({...history.document,marks:marks.filter((_,i)=>i!==index)},mark);
  if(JSON.stringify(copy)===JSON.stringify(marks[index]))return history;
  return commitDocument(history,{...history.document,marks:marks.map((current,i)=>i===index?copy:current)});
}

export function removeMark(history:History,index:number):History {
  if(!Number.isInteger(index)||index<0||index>=history.document.marks.length)throw new Error("Invalid annotation index");
  return commitDocument(history,{...history.document,marks:history.document.marks.filter((_,i)=>i!==index)});
}

function commitDocument(history:History,document:AnnotationDocument):History {
  return {document,past:[...history.past,history.document].slice(-100),future:[]};
}

// Crop remains a source-space rectangle until export. Existing marks keep
// their coordinates; undo restores both the crop and the annotation content.
export function commitCrop(history:History,crop:CropRect|undefined):History {
  const {width,height}=history.document;
  if(crop) {
    const {x,y,width:w,height:h}=crop;
    if(![x,y,w,h].every(Number.isInteger)||x<0||y<0||w<1||h<1||x+w>width||y+h>height)throw new Error("Invalid crop bounds");
    if(x===0&&y===0&&w===width&&h===height)crop=undefined;
  }
  if(JSON.stringify(crop)===JSON.stringify(history.document.crop))return history;
  const {crop:previous,...document}=history.document;
  // Version 2 prevents an older renderer silently ignoring a crop request.
  return commitDocument(history,crop?{...document,version:2,crop:{...crop}}:{...document,version:1});
}

export function undo(history:History):History {
  const document=history.past.at(-1);
  return document?{document,past:history.past.slice(0,-1),future:[history.document,...history.future]}:history;
}
export function redo(history:History):History {
  const document=history.future[0];
  return document?{document,past:[...history.past,history.document].slice(-100),future:history.future.slice(1)}:history;
}
