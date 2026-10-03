import {expect,it} from "vitest";
import {captureZoomScale,visibleCaptureViewport,captureZoomAnchor,captureZoomDelta} from "./capture-viewport";

it("keeps the same source point at the viewport center after zooming",()=>{
 const view={left:10,top:20,width:800,height:500};
 const anchor=captureZoomAnchor({left:110,top:-20000,width:600,height:40000},view);
 const enlarged={left:26,top:-20000,width:1200,height:80000};
 const delta=captureZoomDelta(anchor,enlarged,view);
 expect(delta.x).toBeCloseTo(216);expect(delta.y).toBeCloseTo(20270);
 const restored={...enlarged,left:enlarged.left-delta.x,top:enlarged.top-delta.y};
 expect(captureZoomAnchor(restored,view)).toEqual(anchor);
});

it("keeps a long image readable by width and uses physical pixels for percentages",()=>{
 const image={width:600,height:40000},view={width:1000,height:600};
 expect(captureZoomScale("fit",image,view,1)).toBe(.015);
 expect(captureZoomScale("width",image,view,1)).toBe(1);
 expect(captureZoomScale("actual",image,view,2)).toBe(.5);
 expect(captureZoomScale("200",image,view,2)).toBe(1);
});
it("bounds the drawing allocation to the visible slice of a very long image",()=>{
 const slice=visibleCaptureViewport({left:16,top:-30000,width:600,height:40000},{left:0,top:0,width:1000,height:600});
 expect(slice).toEqual({x:0,y:30000,width:600,height:600});
 expect(visibleCaptureViewport({left:0,top:900,width:600,height:40000},{left:0,top:0,width:1000,height:600})).toEqual({x:0,y:0,width:600,height:0});
});
