import type {CapturePlacement} from "./capture-draft-api";
export interface CaptureSurface {placement:CapturePlacement;desktopPreviewId:string;monitor:{left:number;top:number;right:number;bottom:number}}
export function captureSurfaceLayout(surface:CaptureSurface,image:{width:number;height:number},viewportWidth:number,toolbarHeight:number,toolbarEdge?:"top"|"bottom",selection?:{x:number;y:number;width:number;height:number}){
 const p=surface.placement,s=viewportWidth/p.desktopWidth,m=surface.monitor;
 const left=(p.x-p.desktopX)*s,top=(p.y-p.desktopY)*s,width=image.width*s,height=image.height*s;
 const ml=Math.max(0,(m.left-p.desktopX)*s),mt=Math.max(0,(m.top-p.desktopY)*s),mr=Math.min(viewportWidth,(m.right-p.desktopX)*s),mb=Math.min(p.desktopHeight*s,(m.bottom-p.desktopY)*s);
 // Allow the complete default tool row with labels on desktop displays. The
 // monitor work area still limits controls before overflow menus are needed.
 const toolbarWidth=Math.max(1,Math.min(1440,mr-ml-16)),maxHeight=Math.max(1,mb-mt-16),h=Math.min(toolbarHeight,maxHeight);
 const selectedLeft=left+(selection?.x??0)*s,selectedTop=top+(selection?.y??0)*s,selectedHeight=selection?selection.height*s:height;
 const x=Math.max(ml+8,Math.min(selectedLeft,mr-toolbarWidth-8));
 let y=selectedTop+selectedHeight+8;if(y+h>mb-8)y=selectedTop-h-8;y=Math.max(mt+8,Math.min(y,mb-h-8));
 // Explicit positioning only moves controls; image coordinates stay unchanged.
 if(toolbarEdge)y=toolbarEdge==="top"?mt+8:mb-h-8;
 return {scale:s,left,top,width,height,boundary:{x:ml,y:mt,width:mr-ml,height:mb-mt},toolbar:{left:x,top:y,width:toolbarWidth,maxHeight},properties:{left:Math.max(ml+8,mr-336),top:mt+8,maxHeight}};
}
