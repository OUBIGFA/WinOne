import type {CropRect} from "./capture-editor-model";

export type CropEdge="top"|"bottom"|"left"|"right"|"top-left"|"top-right"|"bottom-left"|"bottom-right";

// Move the frame without changing its size or translating the source pixels.
export function moveCrop(crop:CropRect,dx:number,dy:number,width:number,height:number):CropRect {
 return {...crop,x:Math.max(0,Math.min(width-crop.width,Math.round(crop.x+dx))),y:Math.max(0,Math.min(height-crop.height,Math.round(crop.y+dy)))};
}

// Deltas are source pixels, not CSS pixels. Keep the opposite edge fixed and
// stop at one pixel rather than flipping the selection under the pointer.
export function resizeCrop(crop:CropRect,edge:CropEdge,dx:number,dy:number,width:number,height:number):CropRect {
 let left=crop.x,top=crop.y,right=left+crop.width,bottom=top+crop.height;
 if(edge.includes("left"))left=Math.max(0,Math.min(right-1,Math.round(left+dx)));
 if(edge.includes("right"))right=Math.min(width,Math.max(left+1,Math.round(right+dx)));
 if(edge.includes("top"))top=Math.max(0,Math.min(bottom-1,Math.round(top+dy)));
 if(edge.includes("bottom"))bottom=Math.min(height,Math.max(top+1,Math.round(bottom+dy)));
 return {x:left,y:top,width:right-left,height:bottom-top};
}
