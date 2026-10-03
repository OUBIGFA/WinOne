import {expect,it} from "vitest";
import {captureSurfaceLayout} from "./capture-surface-layout";

it("places controls beside the crop while retaining full-frame image coordinates",()=>{
 const surface={desktopPreviewId:"b".repeat(32),placement:{x:0,y:0,desktopX:0,desktopY:0,desktopWidth:1920,desktopHeight:1080},monitor:{left:0,top:0,right:1920,bottom:1040}};
 const layout=captureSurfaceLayout(surface,{width:1920,height:1080},1920,190,undefined,{x:100,y:100,width:400,height:300});
 expect(layout.left).toBe(0);expect(layout.width).toBe(1920);
 expect(layout.toolbar.left).toBe(100);expect(layout.toolbar.top).toBe(408);
});

it("keeps the image at its desktop location and the toolbar inside its monitor",()=>{
 const surface={desktopPreviewId:"b".repeat(32),placement:{x:-1600,y:100,desktopX:-1920,desktopY:0,desktopWidth:3840,desktopHeight:1080},monitor:{left:-1920,top:0,right:0,bottom:1040}};
 const layout=captureSurfaceLayout(surface,{width:600,height:400},1920,190);
 expect({left:layout.left,top:layout.top,width:layout.width,height:layout.height}).toEqual({left:160,top:50,width:300,height:200});
 expect(layout.toolbar.left).toBeGreaterThanOrEqual(8);
 expect(layout.toolbar.left+layout.toolbar.width).toBeLessThanOrEqual(952);
 expect(layout.toolbar.top+190).toBeLessThanOrEqual(512);
});

it("moves the toolbar between monitor edges without moving the screenshot",()=>{
 const surface={desktopPreviewId:"b".repeat(32),placement:{x:-1920,y:0,desktopX:-1920,desktopY:0,desktopWidth:3840,desktopHeight:1080},monitor:{left:-1920,top:0,right:0,bottom:1040}};
 const top=captureSurfaceLayout(surface,{width:1920,height:1080},1920,190,"top");
 const bottom=captureSurfaceLayout(surface,{width:1920,height:1080},1920,190,"bottom");
 expect(top.toolbar.top).toBe(8);
 expect(bottom.toolbar.top).toBe(322);
 expect([top.left,top.top,top.width,top.height]).toEqual([bottom.left,bottom.top,bottom.width,bottom.height]);
 // Expanded tools must remain reachable inside this display's work area.
 const expanded=captureSurfaceLayout(surface,{width:1920,height:1080},1920,900,"bottom");
 expect(expanded.toolbar.top).toBe(8);
 expect(expanded.toolbar.maxHeight).toBe(504);
});
