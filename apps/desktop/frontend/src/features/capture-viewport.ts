export type CaptureZoom="fit"|"width"|"actual"|"200"|"400";
interface Size {width:number;height:number}
interface Rect extends Size {left:number;top:number}

export function captureZoomAnchor(image:Rect,viewport:Rect){
 return {x:Math.max(0,Math.min(1,(viewport.left+viewport.width/2-image.left)/image.width)),y:Math.max(0,Math.min(1,(viewport.top+viewport.height/2-image.top)/image.height))};
}

// Measure after layout, so centering margins and newly visible scrollbars are
// included. The browser clamps the resulting scroll at the image boundaries.
export function captureZoomDelta(anchor:{x:number;y:number},image:Rect,viewport:Rect){
 return {x:image.left+anchor.x*image.width-(viewport.left+viewport.width/2),y:image.top+anchor.y*image.height-(viewport.top+viewport.height/2)};
}

export function captureZoomScale(mode:CaptureZoom,image:Size,viewport:Size,pixelRatio:number):number {
 const actual=1/pixelRatio;
 if(mode==="actual")return actual;
 if(mode==="200")return 2*actual;
 if(mode==="400")return 4*actual;
 const width=Math.min(actual,viewport.width/image.width);
 return mode==="width"?width:Math.min(width,viewport.height/image.height);
}

// Offsets are CSS pixels within the image. Native annotation coordinates are
// never clipped or rewritten; only the canvas allocation follows the viewport.
export function visibleCaptureViewport(image:Rect,viewport:Rect){
 const x=Math.max(0,Math.min(image.width,Math.floor(viewport.left-image.left)));
 const y=Math.max(0,Math.min(image.height,Math.floor(viewport.top-image.top)));
 const right=Math.min(image.width,Math.ceil(viewport.left+viewport.width-image.left));
 const bottom=Math.min(image.height,Math.ceil(viewport.top+viewport.height-image.top));
 return {x,y,width:Math.max(0,right-x),height:Math.max(0,bottom-y)};
}
